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

	procGetCurrentProcess        = kernel32.NewProc("GetCurrentProcess")
	procCloseHandle              = kernel32.NewProc("CloseHandle")
	procOpenProcessToken         = advapi32.NewProc("OpenProcessToken")
	procGetTokenInformation      = advapi32.NewProc("GetTokenInformation")
	procShellExecuteW            = shell32.NewProc("ShellExecuteW")
	procCreateToolhelp32Snapshot = kernel32.NewProc("CreateToolhelp32Snapshot")
	procProcess32FirstW          = kernel32.NewProc("Process32FirstW")
	procProcess32NextW           = kernel32.NewProc("Process32NextW")
	procOpenProcess              = kernel32.NewProc("OpenProcess")
	procTerminateProcess         = kernel32.NewProc("TerminateProcess")
)

const (
	tokenQuery           = 0x0008 // TOKEN_QUERY
	tokenElevationClass  = 20     // TOKEN_INFORMATION_CLASS.TokenElevation
	swShowNormal         = 1      // SW_SHOWNORMAL
	shellExecuteMinOK    = 32     // ShellExecuteW returns > 32 on success
	createNoWindowFlag   = 0x08000000
	installDirPermission = 0o755

	th32csSnapProcess = 0x00000002  // TH32CS_SNAPPROCESS
	processTerminate  = 0x0001      // PROCESS_TERMINATE
	maxPathW          = 260         // MAX_PATH (PROCESSENTRY32W.szExeFile)
	invalidHandle     = ^uintptr(0) // INVALID_HANDLE_VALUE ((HANDLE)-1)
	daemonImageName   = "prata.exe" // image name matched when clearing stragglers
	copyRetryAttempts = 10          // bounded retry for a transiently locked target
	copyRetryDelay    = 200 * time.Millisecond
)

// legacyBinaries are the only files cleanupLegacyUserBinaries removes from each
// per-user %LOCALAPPDATA%\Prata. User data (apikey.dat, backend.txt,
// dictionary-corrections.txt override) is preserved by never touching anything
// else in that folder.
var legacyBinaries = []string{"prata.exe", "prata-setkey.exe"}

// processEntry32 mirrors the Win32 PROCESSENTRY32W layout. DefaultHeapID is
// ULONG_PTR (uintptr): on 64-bit it is 8 bytes and, with the 4-byte padding the
// three leading DWORDs force, sits at offset 16 — exactly as the C ABI lays it
// out. Declaring it as a 4-byte type would shift every following field
// (including ExeFile) and the decoded process names would be garbage.
type processEntry32 struct {
	Size            uint32
	Usage           uint32
	ProcessID       uint32
	DefaultHeapID   uintptr
	ModuleID        uint32
	Threads         uint32
	ParentProcessID uint32
	PriClassBase    int32
	Flags           uint32
	ExeFile         [maxPathW]uint16
}

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

	// Clear blockers from a previous install (legacy per-user or an earlier
	// machine-wide one) before copying: a running daemon holds the
	// session-scoped single-instance mutex (a freshly started daemon would exit
	// as "already running") and may lock the target binary. Self-excluded.
	terminateOtherInstances()

	// source==dest: someone ran the already-installed binary with --install.
	// Skip the copy but still re-register the task — an idempotent repair.
	if samePath(src, dst) {
		logf("src==dst, skipping copy (idempotent repair)")
	} else {
		if err := copyFileWithRetry(src, dst); err != nil {
			logf("copy failed after %d attempts: %v", copyRetryAttempts, err)
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

	runErr := runTask()

	// Migration cleanup runs after the start attempt, in both outcomes: once the
	// machine-wide binary is installed and registered, stale per-user binaries
	// are redundant. Best-effort; it never changes the install result.
	cleanupLegacyUserBinaries()

	if runErr != nil {
		// Non-fatal: no interactive session (e.g. SYSTEM/IT-driven install) or
		// on-demand start refused. The logon trigger still starts Prata at the
		// next sign-in.
		logf("runTask failed (non-fatal): %v", runErr)
		ui.MessageBox("Prata", "Prata installerad. Startar vid nästa inloggning.", ui.IconInfo)
		return
	}
	logf("install complete; daemon started via task")
	ui.MessageBox("Prata", "Prata installerad och startad.", ui.IconInfo)
}

// terminateOtherInstances force-terminates every running prata.exe except the
// current process. A previously running daemon holds the session-scoped
// single-instance mutex and may lock the target binary in %ProgramFiles%; both
// block a clean install, so they are cleared before copy/register/start. It is
// reached only from the already-elevated install path, so it can terminate a
// higher-integrity straggler. Best-effort and fully logged — a failure here
// never aborts the install.
func terminateOtherInstances() (killed int) {
	snap, _, err := procCreateToolhelp32Snapshot.Call(uintptr(th32csSnapProcess), 0)
	if snap == invalidHandle {
		logf("process snapshot failed: %v", err)
		return 0
	}
	defer procCloseHandle.Call(snap)

	self := uint32(os.Getpid())
	var entry processEntry32
	entry.Size = uint32(unsafe.Sizeof(entry))

	ret, _, _ := procProcess32FirstW.Call(snap, uintptr(unsafe.Pointer(&entry)))
	for ret != 0 {
		name := syscall.UTF16ToString(entry.ExeFile[:])
		if shouldTerminate(name, entry.ProcessID, self) {
			if err := terminatePID(entry.ProcessID); err != nil {
				logf("terminate pid %d (%s) failed: %v", entry.ProcessID, name, err)
			} else {
				logf("terminated stale instance pid %d (%s)", entry.ProcessID, name)
				killed++
			}
		}
		ret, _, _ = procProcess32NextW.Call(snap, uintptr(unsafe.Pointer(&entry)))
	}
	logf("terminateOtherInstances: %d stale instance(s) terminated", killed)
	return killed
}

// shouldTerminate reports whether a snapshot entry is a prata.exe instance
// other than the current process. The image name is matched case-insensitively;
// the current PID is always excluded so --install can never terminate itself.
func shouldTerminate(name string, pid, self uint32) bool {
	if pid == self {
		return false
	}
	return strings.EqualFold(name, daemonImageName)
}

// terminatePID opens the process for termination and force-terminates it. A
// process that exited between the snapshot and OpenProcess surfaces as an error
// and is treated as already gone by the best-effort caller.
func terminatePID(pid uint32) error {
	h, _, err := procOpenProcess.Call(uintptr(processTerminate), 0, uintptr(pid))
	if h == 0 {
		return fmt.Errorf("OpenProcess: %w", err)
	}
	defer procCloseHandle.Call(h)
	if ret, _, err := procTerminateProcess.Call(h, 1); ret == 0 {
		return fmt.Errorf("TerminateProcess: %w", err)
	}
	return nil
}

// copyFileWithRetry wraps copyFile in a bounded retry loop. After stale
// instances are terminated the OS can take a moment to release the image
// section lock on the target, so a sharing violation immediately afterwards is
// expected and transient. On persistent failure the final error is returned so
// the caller aborts the install rather than continuing silently.
func copyFileWithRetry(src, dst string) error {
	var err error
	for attempt := 1; attempt <= copyRetryAttempts; attempt++ {
		if err = copyFile(src, dst); err == nil {
			if attempt > 1 {
				logf("copy succeeded on attempt %d/%d", attempt, copyRetryAttempts)
			}
			return nil
		}
		logf("copy attempt %d/%d failed: %v", attempt, copyRetryAttempts, err)
		if attempt < copyRetryAttempts {
			time.Sleep(copyRetryDelay)
		}
	}
	return err
}

// cleanupLegacyUserBinaries removes stale per-user prata.exe / prata-setkey.exe
// left by the legacy install.ps1 path across every user profile. It runs after
// the machine-wide binary is in place, so a leftover per-user binary is already
// redundant. Best-effort: a missing file or an inaccessible profile is logged
// and skipped, never fatal. Only the two known binaries are removed, so user
// data in the same folder is preserved.
func cleanupLegacyUserBinaries() {
	usersDir := filepath.Join(systemDrive(), "Users")
	profiles, err := os.ReadDir(usersDir)
	if err != nil {
		logf("cleanup: cannot read %s: %v", usersDir, err)
		return
	}
	removed := 0
	for _, path := range legacyBinaryPaths(usersDir, profiles) {
		switch err := os.Remove(path); {
		case err == nil:
			logf("cleanup: removed legacy binary %s", path)
			removed++
		case os.IsNotExist(err):
			// expected for profiles without a legacy install
		default:
			logf("cleanup: could not remove %s: %v", path, err)
		}
	}
	logf("cleanup: %d legacy binary file(s) removed", removed)
}

// legacyBinaryPaths returns the candidate per-user binary paths to remove for
// the given Users directory and its profile entries. It is pure (no I/O) so it
// can be unit-tested: for each subdirectory it joins
// <profile>\AppData\Local\Prata\<binary> for each legacy binary; non-directory
// entries are skipped.
func legacyBinaryPaths(usersDir string, profiles []os.DirEntry) []string {
	var paths []string
	for _, p := range profiles {
		if !p.IsDir() {
			continue
		}
		base := filepath.Join(usersDir, p.Name(), "AppData", "Local", "Prata")
		for _, bin := range legacyBinaries {
			paths = append(paths, filepath.Join(base, bin))
		}
	}
	return paths
}

// systemDrive returns the OS drive root (e.g. "C:\"), falling back to C:\.
// SystemDrive is reported without a trailing separator ("C:"), which would
// otherwise be treated as a drive-relative path by filepath.Join.
func systemDrive() string {
	if d := os.Getenv("SystemDrive"); d != "" {
		return d + `\`
	}
	return `C:\`
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
