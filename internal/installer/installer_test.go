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
		`<ExecutionTimeLimit>PT0S</ExecutionTimeLimit>`, // no time limit
		`<Command>C:\Program Files\Prata\prata.exe</Command>`,
	}
	for _, s := range mustContain {
		if !strings.Contains(xml, s) {
			t.Errorf("task XML missing %q\n---\n%s", s, xml)
		}
	}

	mustNotContain := []string{
		`<UserId>`,    // logon trigger must apply to all users
		`Highest`,     // RunLevel Highest would break SendInput via UIPI
		`<LogonType>`, // explicit LogonType with a GroupId breaks schtasks /XML
	}
	for _, s := range mustNotContain {
		if strings.Contains(xml, s) {
			t.Errorf("task XML must not contain %q\n---\n%s", s, xml)
		}
	}
}

// TestTaskXMLElementOrder guards against the schtasks "unexpected node" error,
// which is caused by elements appearing out of the schema-defined sequence.
// It checks the relative position of order-sensitive elements rather than the
// exact layout.
func TestTaskXMLElementOrder(t *testing.T) {
	xml := taskXML(`C:\Program Files\Prata\prata.exe`)

	// Within Principal: GroupId must precede RunLevel.
	assertOrder(t, xml, "<GroupId>", "<RunLevel>")

	// Within Settings, the schema sequence (subset we emit). Scope to the
	// Settings block because some element names (e.g. <Enabled>) also appear
	// in other blocks such as the trigger.
	settings := between(t, xml, "<Settings>", "</Settings>")
	settingsOrder := []string{
		"<AllowStartOnDemand>",
		"<MultipleInstancesPolicy>",
		"<DisallowStartIfOnBatteries>",
		"<StopIfGoingOnBatteries>",
		"<AllowHardTerminate>",
		"<StartWhenAvailable>",
		"<RunOnlyIfNetworkAvailable>",
		"<WakeToRun>",
		"<Enabled>",
		"<Hidden>",
		"<ExecutionTimeLimit>",
		"<Priority>",
		"<RunOnlyIfIdle>",
	}
	for i := 0; i+1 < len(settingsOrder); i++ {
		assertOrder(t, settings, settingsOrder[i], settingsOrder[i+1])
	}

	// Top-level: Triggers, Principals, Settings, Actions in that order.
	assertOrder(t, xml, "<Triggers>", "<Principals>")
	assertOrder(t, xml, "<Principals>", "<Settings>")
	assertOrder(t, xml, "<Settings>", "<Actions")
}

// between returns the substring of s strictly between the first occurrences of
// open and close. It fails the test if either marker is missing.
func between(t *testing.T, s, open, close string) string {
	t.Helper()
	i := strings.Index(s, open)
	if i < 0 {
		t.Fatalf("marker %q missing", open)
	}
	rest := s[i+len(open):]
	j := strings.Index(rest, close)
	if j < 0 {
		t.Fatalf("marker %q missing", close)
	}
	return rest[:j]
}

func assertOrder(t *testing.T, s, first, second string) {
	t.Helper()
	i := strings.Index(s, first)
	j := strings.Index(s, second)
	if i < 0 {
		t.Errorf("element %q missing", first)
		return
	}
	if j < 0 {
		t.Errorf("element %q missing", second)
		return
	}
	if i >= j {
		t.Errorf("element order wrong: %q (at %d) must precede %q (at %d)", first, i, second, j)
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
