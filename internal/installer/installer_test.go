package installer

import (
	"strings"
	"testing"
	"unicode/utf16"
)

func TestTaskXMLEnforcesInvariants(t *testing.T) {
	xml := taskXML(`C:\Program Files\Prata\prata.exe`)

	mustContain := []string{
		`<RunLevel>LeastPrivilege</RunLevel>`,           // never Highest (UIPI)
		`<LogonTrigger>`,                                // logon-triggered
		`<GroupId>S-1-5-32-545</GroupId>`,               // BUILTIN\Users (locale-safe SID)
		`<LogonType>InteractiveToken</LogonType>`,       // in-session user token
		`<ExecutionTimeLimit>PT0S</ExecutionTimeLimit>`, // no time limit
		`<Command>C:\Program Files\Prata\prata.exe</Command>`,
	}
	for _, s := range mustContain {
		if !strings.Contains(xml, s) {
			t.Errorf("task XML missing %q\n---\n%s", s, xml)
		}
	}

	mustNotContain := []string{
		`<UserId>`, // logon trigger must apply to all users
		`Highest`,  // RunLevel Highest would break SendInput via UIPI
	}
	for _, s := range mustNotContain {
		if strings.Contains(xml, s) {
			t.Errorf("task XML must not contain %q\n---\n%s", s, xml)
		}
	}
}

func TestTaskXMLEscapesCommand(t *testing.T) {
	xml := taskXML(`C:\Tools\a&b\prata.exe`)
	if !strings.Contains(xml, `<Command>C:\Tools\a&amp;b\prata.exe</Command>`) {
		t.Errorf("command path not XML-escaped:\n%s", xml)
	}
}

func TestInstallDir(t *testing.T) {
	t.Setenv("ProgramFiles", `C:\Program Files`)
	t.Setenv("ProgramW6432", `C:\Program Files`)
	if got, want := installDir(), `C:\Program Files\Prata`; got != want {
		t.Errorf("installDir() = %q, want %q", got, want)
	}

	// Empty ProgramFiles falls back to ProgramW6432.
	t.Setenv("ProgramFiles", "")
	t.Setenv("ProgramW6432", `C:\PF64`)
	if got, want := installDir(), `C:\PF64\Prata`; got != want {
		t.Errorf("installDir() fallback = %q, want %q", got, want)
	}
}

func TestSamePath(t *testing.T) {
	if !samePath(`C:\Program Files\Prata\prata.exe`, `C:\Program Files\Prata\prata.exe`) {
		t.Error("identical paths reported as different")
	}
	// Case-insensitive (Windows file systems fold case).
	if !samePath(`C:\Program Files\Prata\PRATA.EXE`, `C:\Program Files\Prata\prata.exe`) {
		t.Error("case-different paths to the same file reported as different")
	}
	if samePath(`C:\Temp\prata.exe`, `C:\Program Files\Prata\prata.exe`) {
		t.Error("distinct paths reported as the same")
	}
}

func TestIsElevatedSmoke(t *testing.T) {
	// We cannot exercise both branches in a test, but the call must succeed
	// and return a value. (In a normal test context the process is not
	// elevated, so this is typically false.)
	elevated, err := isElevated()
	if err != nil {
		t.Fatalf("isElevated returned error: %v", err)
	}
	t.Logf("isElevated() = %v", elevated)
}

func TestUTF16LEWithBOM(t *testing.T) {
	b := utf16LEWithBOM("AB")
	if len(b) < 2 || b[0] != 0xFF || b[1] != 0xFE {
		t.Fatalf("missing UTF-16LE BOM: % x", b[:min(2, len(b))])
	}
	// Decode the payload back and compare.
	units := make([]uint16, 0, (len(b)-2)/2)
	for i := 2; i+1 < len(b); i += 2 {
		units = append(units, uint16(b[i])|uint16(b[i+1])<<8)
	}
	if got := string(utf16.Decode(units)); got != "AB" {
		t.Errorf("round-trip = %q, want %q", got, "AB")
	}
}
