// Command dict-foldin folds valuable per-user override entries into the
// embedded baseline dictionary ahead of a release, so clinic corrections
// (domain knowledge, not personal preference) ship to every user. Run it
// manually before a build — never in the daemon hot path and never
// automatically in CI.
//
// Usage:
//
//	dict-foldin --override <path> [--baseline internal/dict/dictionary-corrections.txt] [--dry-run]
//
// Per key it adds a new rule or replaces an existing one in place; the
// baseline's comments, blank lines, and order are preserved; empty/identity
// rules are skipped; baseline rules are never removed. It edits only the
// baseline file and never touches the override. With --dry-run it prints what
// would change and writes nothing. It exits non-zero on a parse or I/O error.
package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/carlosriveros/prata/internal/dict"
)

// defaultBaseline is the single embedded baseline source — the only file the
// build embeds, so it is the only sensible fold-in target.
const defaultBaseline = "internal/dict/dictionary-corrections.txt"

func main() {
	override := flag.String("override", "", "path to the override file to fold in (required)")
	baseline := flag.String("baseline", defaultBaseline, "path to the baseline file to update")
	dryRun := flag.Bool("dry-run", false, "print what would change and write nothing")
	flag.Parse()

	if err := run(*override, *baseline, *dryRun); err != nil {
		fmt.Fprintln(os.Stderr, "dict-foldin:", err)
		os.Exit(1)
	}
}

// run folds overridePath into baselinePath and reports the result. On dryRun
// it writes nothing. It returns an error (the caller exits non-zero) on a
// missing override, an I/O failure, or a parse error in either file.
func run(overridePath, baselinePath string, dryRun bool) error {
	if overridePath == "" {
		return fmt.Errorf("--override is required")
	}
	overrideText, err := os.ReadFile(overridePath)
	if err != nil {
		return fmt.Errorf("read override: %w", err)
	}
	baselineText, err := os.ReadFile(baselinePath)
	if err != nil {
		return fmt.Errorf("read baseline: %w", err)
	}

	res, err := dict.FoldIn(string(baselineText), string(overrideText))
	if err != nil {
		return err
	}

	report(res, dryRun)

	if dryRun || (len(res.Added) == 0 && len(res.Replaced) == 0) {
		// Nothing to persist (or asked not to) — leave the file and its
		// modification time untouched so the build input is stable.
		return nil
	}

	// Preserve the baseline's existing permissions where they can be read.
	perm := os.FileMode(0o644)
	if fi, statErr := os.Stat(baselinePath); statErr == nil {
		perm = fi.Mode().Perm()
	}
	if err := os.WriteFile(baselinePath, []byte(res.Output), perm); err != nil {
		return fmt.Errorf("write baseline: %w", err)
	}
	return nil
}

// report prints a short, human-readable summary of the fold-in to stdout.
func report(res dict.FoldInResult, dryRun bool) {
	verb := "folded in"
	if dryRun {
		verb = "would fold in"
	}
	fmt.Printf("%s: %d added, %d replaced, %d skipped\n",
		verb, len(res.Added), len(res.Replaced), len(res.Skipped))
	for _, k := range res.Added {
		fmt.Printf("  + %s\n", k)
	}
	for _, k := range res.Replaced {
		fmt.Printf("  ~ %s\n", k)
	}
	for _, k := range res.Skipped {
		fmt.Printf("  - %s (skipped: empty or identity)\n", k)
	}
}
