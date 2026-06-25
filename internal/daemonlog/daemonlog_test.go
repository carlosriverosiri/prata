package daemonlog

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestOpenPrintfClose exercises the happy path: Open creates the file, Printf
// appends a timestamped line, and a second Open after Close appends rather than
// truncating. PRATA_DAEMON_LOG isolates the test from the real per-user log.
func TestOpenPrintfClose(t *testing.T) {
	path := filepath.Join(t.TempDir(), "prata-test.log")
	t.Setenv("PRATA_DAEMON_LOG", path)

	closer, err := Open()
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	Printf("hello backend=%s n=%d", "Jobb", 7)
	if err := closer.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	// Reopen and append a second line to confirm append mode (O_APPEND), not
	// truncate.
	closer2, err := Open()
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	Printf("second line")
	if err := closer2.Close(); err != nil {
		t.Fatalf("Close (2): %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read log: %v", err)
	}
	got := string(data)
	if !strings.Contains(got, "hello backend=Jobb n=7") {
		t.Errorf("first line missing; log=%q", got)
	}
	if !strings.Contains(got, "second line") {
		t.Errorf("append did not preserve first write; log=%q", got)
	}
	// Each line is "YYYY-MM-DD HH:MM:SS  <message>\n": expect exactly two lines.
	if n := strings.Count(got, "\n"); n != 2 {
		t.Errorf("expected 2 log lines, got %d; log=%q", n, got)
	}
	if !strings.HasPrefix(got, "20") { // timestamp prefix (year 20xx)
		t.Errorf("line not timestamped; log=%q", got)
	}
}

// TestPruneOldLogs verifies retention deletes only dated log files past the
// window, keeps recent ones, and never touches a name that does not match the
// prata-YYYY-MM-DD.log pattern (an unparseable date or an unrelated file).
func TestPruneOldLogs(t *testing.T) {
	dir := t.TempDir()
	now := time.Date(2026, 6, 25, 12, 0, 0, 0, time.UTC)

	// name -> should still exist after a 30-day prune at `now`.
	files := map[string]bool{
		"prata-2026-06-25.log": true,  // today
		"prata-2026-06-01.log": true,  // 24 days old, inside the window
		"prata-2026-05-25.log": false, // 31 days old, outside the window
		"prata-2020-01-01.log": false, // ancient
		"prata-notadate.log":   true,  // unparseable date -> left alone
		"notes.txt":            true,  // unrelated file -> left alone
	}
	for name := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("x"), 0o600); err != nil {
			t.Fatalf("seed %s: %v", name, err)
		}
	}

	pruneOldLogs(dir, now, retentionDays)

	for name, wantSurvive := range files {
		_, err := os.Stat(filepath.Join(dir, name))
		survived := err == nil
		if survived != wantSurvive {
			t.Errorf("%s: survived=%v, want %v", name, survived, wantSurvive)
		}
	}
}

// TestPruneOldLogsMissingDir confirms prune is a best-effort no-op (no panic)
// when the logs directory cannot be read.
func TestPruneOldLogsMissingDir(t *testing.T) {
	pruneOldLogs(filepath.Join(t.TempDir(), "does-not-exist"), time.Now(), retentionDays)
}

// TestPrintfBeforeOpenIsNoop verifies Printf is a safe no-op when no file is
// open, so a stray log call during startup or after Close never panics.
func TestPrintfBeforeOpenIsNoop(t *testing.T) {
	t.Setenv("PRATA_DAEMON_LOG", filepath.Join(t.TempDir(), "unused.log"))
	// Ensure no handle is held from a prior test.
	if err := Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	Printf("must not panic and must not create a file")

	// Close again is also a no-op.
	if err := Close(); err != nil {
		t.Fatalf("second Close: %v", err)
	}
}
