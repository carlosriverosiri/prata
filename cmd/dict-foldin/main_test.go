package main

import (
	"os"
	"path/filepath"
	"testing"
)

func write(t *testing.T, dir, name, content string) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
	return p
}

// TestRunFoldsIn checks the real run: the override is folded into the baseline
// file in place (existing key replaced, new key appended).
func TestRunFoldsIn(t *testing.T) {
	dir := t.TempDir()
	base := write(t, dir, "baseline.txt", "adoption = abduktion\n")
	over := write(t, dir, "override.txt", "adoption = adduktion\nfraktur = fraktyr\n")

	if err := run(over, base, false); err != nil {
		t.Fatalf("run: %v", err)
	}
	got, err := os.ReadFile(base)
	if err != nil {
		t.Fatalf("read baseline: %v", err)
	}
	want := "adoption = adduktion\nfraktur = fraktyr\n"
	if string(got) != want {
		t.Errorf("baseline = %q, want %q", got, want)
	}
}

// TestRunDryRunWritesNothing confirms --dry-run leaves the baseline byte-identical.
func TestRunDryRunWritesNothing(t *testing.T) {
	dir := t.TempDir()
	const original = "adoption = abduktion\n"
	base := write(t, dir, "baseline.txt", original)
	over := write(t, dir, "override.txt", "fraktur = fraktyr\n")

	if err := run(over, base, true); err != nil {
		t.Fatalf("run: %v", err)
	}
	got, err := os.ReadFile(base)
	if err != nil {
		t.Fatalf("read baseline: %v", err)
	}
	if string(got) != original {
		t.Errorf("dry-run modified baseline: %q", got)
	}
}

// TestRunRequiresOverride checks the required-flag guard.
func TestRunRequiresOverride(t *testing.T) {
	if err := run("", "baseline.txt", false); err == nil {
		t.Error("expected an error when --override is empty")
	}
}

// TestRunParseErrorExits ensures a malformed override surfaces as an error
// (the caller exits non-zero), matching the contract.
func TestRunParseErrorExits(t *testing.T) {
	dir := t.TempDir()
	base := write(t, dir, "baseline.txt", "adoption = abduktion\n")
	over := write(t, dir, "override.txt", "no separator here\n")

	if err := run(over, base, false); err == nil {
		t.Error("expected a parse error for a malformed override")
	}
}
