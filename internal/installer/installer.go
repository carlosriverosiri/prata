// Package installer implements Prata's machine-wide `--install` subcommand:
// self-elevation via UAC, copying the binary into %ProgramFiles%\Prata, and
// registering a machine-wide Task Scheduler logon task that launches the
// daemon at Limited (medium) integrity.
//
// This is maintenance-path code, strictly separate from the dictation hot
// path. It is only reached through dispatchSubcommand in cmd/prata.
//
// Hard invariant (patient safety): the daemon must run at MEDIUM integrity.
// A process started directly from the elevated installer would inherit HIGH
// integrity, and UIPI would then silently block SendInput into a non-elevated
// Webdoc — broken injection with no error. So the daemon is never exec'd from
// the installer; it is started via `schtasks /Run`, which runs the task at its
// own RunLevel (LeastPrivilege/medium IL) in the user's session.
package installer

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"
	"unicode/utf16"
	"unsafe"

	"github.com/carlosriveros/prata/internal/ui"
)

// taskName is the Task Scheduler task name (and on-demand /Run target).
const taskName = "Prata"

// logf appends a timestamped diagnostic line to %TEMP%\prata-install.log. The
// install path runs without a console (windowsgui) and reports outcomes
// through modal message boxes, which are awkward to capture; the log gives a
// durable record of each step and the exact error on failure. Best-effort:
// logging failures are ignored, since there is no better channel and the
// install must not abort because it could not write a log line. The elevated
// child and the non-elevated parent share the same per-user %TEMP%, so the log
// is readable afterwards without elevation.
func logf(format string, args ...any) {
	f, err := os.OpenFile(
		filepath.Join(os.TempDir(), "prata-install.log"),
		os.O_CREATE|os.O_WRONLY|os.O_APPEND,
		0o600,
	)
	if err != nil {
		return
	}
	defer f.Close()
	fmt.Fprintf(f, "%s  %s\n", time.Now().Format("2006-01-02 15:04:05"), fmt.Sprintf(format, args...))
}

var (
	kernel32 = syscall.NewLazyDLL("kernel32.dll")
	advapi32 = syscall.NewLazyDLL("advapi32.dll")
	shell32  = syscall.NewLazyDLL("shell32.dll")

	procGetCurrentProcess   = kernel32.NewProc("GetCurrentProcess")
	procCloseHandle         = kernel32.NewProc("CloseHandle")
	procOpenProcessToken    = advapi32.NewProc("OpenProcessToken")
	procGetTokenInformation = advapi32.NewProc("GetTokenInformation")
	procShellExecuteW       = shell32.NewProc("ShellExecuteW")
)

const (
	tokenQuery           = 0x0008 // TOKEN_QUERY
	tokenElevationClass  = 20     // TOKEN_INFORMATION_CLASS.TokenElevation
	swShowNormal         = 1      // SW_SHOWNORMAL
	shellExecuteMinOK    = 32     // ShellExecuteW returns > 32 on success
	createNoWindowFlag   = 0x08000000
	installDirPermission = 0o755
)

// Run performs a clean machine-wide install. It self-elevates via UAC when
// needed, copies the running binary into %ProgramFiles%\Prata, registers the
// machine-wide logon task, and starts the daemon for the current session.
// All outcomes are reported through a message box.
func Run() {
	elevated, err := isElevated()
	if err != nil {
		logf("isElevated failed: %v", err)
		ui.MessageBox("Prata", fmt.Sprintf("Installationen misslyckades: kunde inte läsa behörighet: %v.", err), ui.IconError)
		return
	}
	logf("--install invoked (elevated=%v)", elevated)
	if !elevated {
		launched, err := relaunchElevated()
		if err != nil {
			logf("relaunchElevated failed: %v", err)
			ui.MessageBox("Prata", fmt.Sprintf("Installationen misslyckades: %v.", err), ui.IconError)
			return
		}
		if !launched {
			logf("elevation declined by user (UAC cancelled)")
			ui.MessageBox("Prata", "Installationen avbröts (förhöjning nekades).", ui.IconError)
		} else {
			logf("relaunched elevated; parent exiting")
		}
		// When the elevated child was launched it performs the install and
		// reports its own result; this non-elevated parent just exits.
		return
	}
	installElevated()
}

// installElevated runs the actual install steps; the caller guarantees the
// process is already elevated.
func installElevated() {
	dir := installDir()
	logf("install dir: %s", dir)
	if err := os.MkdirAll(dir, installDirPermission); err != nil {
		logf("MkdirAll(%s) failed: %v", dir, err)
		ui.MessageBox("Prata", fmt.Sprintf("Kunde inte skapa installationsmappen %s: %v.", dir, err), ui.IconError)
		return
	}

	src, err := os.Executable()
	if err != nil {
		logf("os.Executable failed: %v", err)
		ui.MessageBox("Prata", fmt.Sprintf("Installationen misslyckades: kunde inte hitta programfilen: %v.", err), ui.IconError)
		return
	}
	dst := filepath.Join(dir, "prata.exe")
	logf("copy src=%s dst=%s", src, dst)

	// source==dest: someone ran the already-installed binary with --install.
	// Skip the copy but still re-register the task — an idempotent repair.
	if samePath(src, dst) {
		logf("src==dst, skipping copy (idempotent repair)")
	} else {
		if err := copyFile(src, dst); err != nil {
			logf("copyFile failed: %v", err)
			ui.MessageBox("Prata", fmt.Sprintf("Kunde inte kopiera Prata till %s: %v.", dst, err), ui.IconError)
			return
		}
		logf("copy ok")
	}

	if err := registerTask(dst); err != nil {
		logf("registerTask failed: %v", err)
		ui.MessageBox("Prata", fmt.Sprintf("Kunde inte registrera autostart: %v.", err), ui.IconError)
		return
	}
	logf("task registered")

	if err := runTask(); err != nil {
		// Non-fatal: no interactive session (e.g. SYSTEM/IT-driven install) or
		// on-demand start refused. The logon trigger still starts Prata at the
		// next sign-in.
		logf("runTask failed (non-fatal): %v", err)
		ui.MessageBox("Prata", "Prata installerad. Startar vid nästa inloggning.", ui.IconInfo)
		return
	}
	logf("install complete; daemon started via task")
	ui.MessageBox("Prata", "Prata installerad och startad.", ui.IconInfo)
}

// isElevated reports whether the current process runs with an elevated
// (administrator) token.
func isElevated() (bool, error) {
	process, _, _ := procGetCurrentProcess.Call() // pseudo-handle, never fails
	var token syscall.Handle
	ret, _, err := procOpenProcessToken.Call(process, uintptr(tokenQuery), uintptr(unsafe.Pointer(&token)))
	if ret == 0 {
		return false, fmt.Errorf("OpenProcessToken: %w", err)
	}
	defer procCloseHandle.Call(uintptr(token))

	var elevation uint32 // TOKEN_ELEVATION.TokenIsElevated (DWORD)
	var retLen uint32
	ret, _, err = procGetTokenInformation.Call(
		uintptr(token),
		uintptr(tokenElevationClass),
		uintptr(unsafe.Pointer(&elevation)),
		unsafe.Sizeof(elevation),
		uintptr(unsafe.Pointer(&retLen)),
	)
	if ret == 0 {
		return false, fmt.Errorf("GetTokenInformation: %w", err)
	}
	return elevation != 0, nil
}

// relaunchElevated re-runs this executable with "--install" behind a UAC
// elevation prompt (ShellExecuteW verb "runas"). It returns launched=false
// when the user declined the prompt (ShellExecuteW result <= 32). ShellExecuteW
// does not wait for the child; the elevated child carries out the install.
func relaunchElevated() (launched bool, err error) {
	exe, err := os.Executable()
	if err != nil {
		return false, fmt.Errorf("kunde inte hitta programfilen: %w", err)
	}
	verb, err := syscall.UTF16PtrFromString("runas")
	if err != nil {
		return false, err
	}
	file, err := syscall.UTF16PtrFromString(exe)
	if err != nil {
		return false, err
	}
	params, err := syscall.UTF16PtrFromString("--install")
	if err != nil {
		return false, err
	}

	ret, _, _ := procShellExecuteW.Call(
		0, // hwnd — no owner window
		uintptr(unsafe.Pointer(verb)),
		uintptr(unsafe.Pointer(file)),
		uintptr(unsafe.Pointer(params)),
		0, // working directory — inherit
		uintptr(swShowNormal),
	)
	// A success HINSTANCE is > 32; <= 32 is an error code (a declined UAC
	// prompt reports ERROR_CANCELLED via SE_ERR_*). We treat any <= 32 as
	// "elevation did not happen".
	return ret > shellExecuteMinOK, nil
}

// installDir returns %ProgramFiles%\Prata. On a 64-bit binary ProgramFiles is
// the real "C:\Program Files" (not the (x86) tree); ProgramW6432 is a fallback
// if the variable is somehow empty.
func installDir() string {
	pf := os.Getenv("ProgramFiles")
	if pf == "" {
		pf = os.Getenv("ProgramW6432")
	}
	return filepath.Join(pf, "Prata")
}

// samePath reports whether two paths point at the same file, comparing cleaned
// absolute paths case-insensitively (Windows file systems are case-folding).
func samePath(a, b string) bool {
	absA, err1 := filepath.Abs(a)
	absB, err2 := filepath.Abs(b)
	if err1 != nil || err2 != nil {
		return false
	}
	return strings.EqualFold(filepath.Clean(absA), filepath.Clean(absB))
}

// copyFile copies src to dst, truncating an existing dst. A locked or
// unwritable dst surfaces as an error (no silent continuation); overwriting a
// running target is out of scope (handled in a later phase).
func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("open source: %w", err)
	}
	defer in.Close()

	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, installDirPermission)
	if err != nil {
		return fmt.Errorf("open destination: %w", err)
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return fmt.Errorf("copy: %w", err)
	}
	if err := out.Close(); err != nil {
		return fmt.Errorf("finalize destination: %w", err)
	}
	return nil
}

// registerTask writes the task XML to a temp file (UTF-16LE with BOM, required
// by schtasks /XML) and registers it machine-wide, replacing any existing
// "Prata" task (/F).
func registerTask(exePath string) error {
	tmp, err := os.CreateTemp("", "prata-task-*.xml")
	if err != nil {
		return fmt.Errorf("create temp xml: %w", err)
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)

	if _, err := tmp.Write(utf16LEWithBOM(taskXML(exePath))); err != nil {
		tmp.Close()
		return fmt.Errorf("write temp xml: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close temp xml: %w", err)
	}
	return runSchtasks("/Create", "/TN", taskName, "/XML", tmpName, "/F")
}

// runTask starts the task on demand in the current session (medium IL).
func runTask() error {
	return runSchtasks("/Run", "/TN", taskName)
}

// runSchtasks invokes schtasks.exe with no visible console window and returns
// a descriptive error including captured output on failure.
func runSchtasks(args ...string) error {
	cmd := exec.Command("schtasks", args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: createNoWindowFlag}
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("schtasks %s: %v: %s", strings.Join(args, " "), err, strings.TrimSpace(string(out)))
	}
	return nil
}

// taskXML builds the Task Scheduler definition for the machine-wide daemon.
//
// Element order matters: the Task Scheduler v1.2 schema declares principalType
// and settingsType as ordered sequences, and schtasks /XML rejects an
// out-of-order document with "the task XML contains an unexpected node". The
// order below follows the schema exactly.
//
//   - LogonTrigger without a UserId — fires for every user at logon.
//   - Principal GroupId S-1-5-32-545 (BUILTIN\Users; the well-known SID is
//     locale-safe — the display name is localized, e.g. "Användare" on Swedish
//     Windows). No explicit LogonType: for a group principal interactive logon
//     is implicit, and the schema requires LogonType before GroupId, so an
//     explicit value would only invite ordering bugs. RunLevel LeastPrivilege
//     is what enforces medium integrity and must never be Highest — that would
//     run the daemon elevated and break SendInput into non-elevated apps (UIPI
//     invariant). The integrity guarantee lives in RunLevel, not LogonType.
//   - Exec is the installed binary with no arguments (= daemon).
//   - MultipleInstancesPolicy Parallel so each session gets its own instance
//     (the session-scoped mutex prevents duplicates within a session).
func taskXML(exePath string) string {
	return `<?xml version="1.0" encoding="UTF-16"?>
<Task version="1.2" xmlns="http://schemas.microsoft.com/windows/2004/02/mit/task">
  <RegistrationInfo>
    <Description>Prata push-to-talk dictation daemon (machine-wide autostart).</Description>
  </RegistrationInfo>
  <Triggers>
    <LogonTrigger>
      <Enabled>true</Enabled>
    </LogonTrigger>
  </Triggers>
  <Principals>
    <Principal id="Author">
      <GroupId>S-1-5-32-545</GroupId>
      <RunLevel>LeastPrivilege</RunLevel>
    </Principal>
  </Principals>
  <Settings>
    <AllowStartOnDemand>true</AllowStartOnDemand>
    <MultipleInstancesPolicy>Parallel</MultipleInstancesPolicy>
    <DisallowStartIfOnBatteries>false</DisallowStartIfOnBatteries>
    <StopIfGoingOnBatteries>false</StopIfGoingOnBatteries>
    <AllowHardTerminate>true</AllowHardTerminate>
    <StartWhenAvailable>true</StartWhenAvailable>
    <RunOnlyIfNetworkAvailable>false</RunOnlyIfNetworkAvailable>
    <WakeToRun>false</WakeToRun>
    <Enabled>true</Enabled>
    <Hidden>false</Hidden>
    <ExecutionTimeLimit>PT0S</ExecutionTimeLimit>
    <Priority>7</Priority>
    <RunOnlyIfIdle>false</RunOnlyIfIdle>
  </Settings>
  <Actions Context="Author">
    <Exec>
      <Command>` + escapeXML(exePath) + `</Command>
    </Exec>
  </Actions>
</Task>`
}

var xmlEscaper = strings.NewReplacer(
	"&", "&amp;",
	"<", "&lt;",
	">", "&gt;",
	`"`, "&quot;",
	"'", "&apos;",
)

// escapeXML escapes the five XML predefined entities so a path containing
// them cannot corrupt the task definition.
func escapeXML(s string) string { return xmlEscaper.Replace(s) }

// utf16LEWithBOM encodes s as UTF-16 little-endian with a byte-order mark,
// the encoding schtasks /XML expects.
func utf16LEWithBOM(s string) []byte {
	units := utf16.Encode([]rune(s))
	buf := make([]byte, 0, 2+len(units)*2)
	buf = append(buf, 0xFF, 0xFE) // UTF-16LE BOM
	for _, u := range units {
		buf = append(buf, byte(u), byte(u>>8))
	}
	return buf
}
