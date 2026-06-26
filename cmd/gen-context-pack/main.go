// Command gen-context-pack assembles CONTEXT-PACK.md — a single, self-describing
// AI-onboarding bundle for Prata. It is the *compiled* form of the spine docs plus
// a pinned-facts table extracted straight from the code, so a future model (or a
// new owner's AI) gets the read order, the identity facts, the load-bearing
// constants, and the negative-knowledge index from one file.
//
// Design constraint: the output is a PURE FUNCTION of the repository sources — no
// git calls, no timestamps, no randomness. That determinism is what lets CI run
//
//	go run ./cmd/gen-context-pack > CONTEXT-PACK.md && git diff --exit-code CONTEXT-PACK.md
//
// and fail on drift: if a constant changes in code but the committed pack was not
// regenerated, the diff is non-empty and CI goes red. The generator is stdlib-only
// and OS-independent (it only reads files + runs regexes), so it builds on the
// Linux CI runner even though the daemon does not.
//
// Run from the repository root.
package main

import (
	"fmt"
	"os"
	"regexp"
	"strings"
)

// fact is one pinned value extracted from a source file: re's first capture group
// is the value. Keeping the table here (not in a config file) preserves Prata's
// stdlib-only, no-extra-tooling posture.
type fact struct {
	label string
	file  string
	note  string
	re    *regexp.Regexp
}

// facts are the rebuild-load-bearing constants the reconstruction audit flagged as
// "must guess" because they lived only in code, plus the canonical module path.
// Each is mirrored (with full rationale) in CONSTANTS.md; here they are extracted
// live so the pack cannot disagree with the code.
var facts = []fact{
	{"Go module path", "go.mod", "import root — differs from the GitHub slug carlosriverosiri", regexp.MustCompile(`(?m)^module\s+(\S+)`)},
	{"silencePeakFloor", "cmd/prata/main.go", "peak below this ⇒ treat capture as silence (dead mic)", reConst("silencePeakFloor")},
	{"transcribeQueueDepth", "cmd/prata/main.go", "FIFO transcribe queue + jobs/results/dictAdds channel sizes", reConst("transcribeQueueDepth")},
	{"failoverFailureThreshold", "cmd/prata/main.go", "consecutive failures before the once-per-streak tray hint", reConst("failoverFailureThreshold")},
	{"maxInjectAge", "cmd/prata/main.go", "drop (not inject) a transcription older than this after F1 release", reDur("maxInjectAge")},
	{"pasteSettleDelay", "internal/inject/inject.go", "wait after Ctrl+V before restoring the prior clipboard", reDur("pasteSettleDelay")},
	{"copySettleTimeout", "internal/inject/inject.go", "F8 wait for the synthesized Ctrl+C to populate the clipboard", reDur("copySettleTimeout")},
	{"focusSettle", "internal/inject/inject.go", "wait before re-reading the foreground to confirm a restore", reDur("focusSettle")},
	{"interEventDelay", "internal/inject/inject.go", "inter-event delay on the SendInput path", reDur("interEventDelay")},
	{"httpTimeout (transcribe)", "internal/transcribe/client.go", "whole-request timeout for a transcription POST", reDur("httpTimeout")},
	{"maxRatio", "internal/sanity/sanity.go", "gzip degenerate-output threshold — must NOT be lowered", reFloat("maxRatio")},
	{"minLength", "internal/sanity/sanity.go", "byte floor below which the gzip ratio is not trusted", reConst("minLength")},
	{"minPhraseReps", "internal/sanity/sanity.go", "phrase-loop: repeats before a back-to-back phrase is flagged", reConst("minPhraseReps")},
	{"f1RetryInterval", "internal/hotkey/listener.go", "F1 self-heal re-probe cadence", reDur("f1RetryInterval")},
	{"retentionDays", "internal/daemonlog/daemonlog.go", "per-day daemon logs older than this are pruned on startup", reConst("retentionDays")},
	{"copyRetryAttempts", "internal/installer/installer.go", "bounded retry for a transiently locked target binary", reConst("copyRetryAttempts")},
	{"httpTimeout (update)", "internal/update/update.go", "notify-only update-check request timeout", reDur("httpTimeout")},
}

// reConst matches `name = 123` (an integer constant).
func reConst(name string) *regexp.Regexp {
	return regexp.MustCompile(name + `\s*=\s*([0-9]+)\b`)
}

// reFloat matches `name = 1.23` (a float constant).
func reFloat(name string) *regexp.Regexp {
	return regexp.MustCompile(name + `\s*=\s*([0-9]+\.[0-9]+)`)
}

// reDur matches `name = 8 * time.Second` (a time.Duration constant).
func reDur(name string) *regexp.Regexp {
	return regexp.MustCompile(name + `\s*=\s*([0-9]+\s*\*\s*time\.\w+)`)
}

// extractValue returns the trimmed first capture group of re in content.
func extractValue(content string, re *regexp.Regexp) (string, bool) {
	m := re.FindStringSubmatch(content)
	if m == nil || len(m) < 2 {
		return "", false
	}
	return strings.TrimSpace(m[1]), true
}

// extractSection returns the slice of content from the first occurrence of start
// up to (but not including) the next occurrence of end after it. If end is not
// found, it returns everything from start onward.
func extractSection(content, start, end string) (string, bool) {
	i := strings.Index(content, start)
	if i < 0 {
		return "", false
	}
	rest := content[i:]
	if j := strings.Index(rest, end); j >= 0 {
		return strings.TrimRight(rest[:j], "\n"), true
	}
	return strings.TrimRight(rest, "\n"), true
}

// countMatches counts non-overlapping matches of re in content.
func countMatches(content string, re *regexp.Regexp) int {
	return len(re.FindAllString(content, -1))
}

var rejRowRe = regexp.MustCompile(`(?m)^\| REJ-[0-9]+ `)

// readFile reads a repo-relative file, returning "" on error so the generator
// degrades to a clear inline marker rather than aborting.
func readFile(path string) string {
	b, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return string(b)
}

func main() {
	var b strings.Builder
	w := func(format string, a ...any) { fmt.Fprintf(&b, format, a...) }

	w("# CONTEXT-PACK — Prata\n\n")
	w("> **GENERATED — do not edit by hand.** Run `go run ./cmd/gen-context-pack > CONTEXT-PACK.md`.\n")
	w("> CI regenerates this and fails on any diff, so it cannot silently drift from the code.\n")
	w("> Deterministic: a pure function of the repository sources (no timestamps, no git calls).\n\n")

	// --- 0. Provenance (generated, but deterministic) ---
	w("## 0. Provenance\n\n")
	w("- Generator: `cmd/gen-context-pack` (stdlib-only, OS-independent).\n")
	w("- Source of truth: **this repository**. For the exact commit, see `git log`.\n")
	w("- This file is the *compiled* form of the spine docs + facts extracted from code.\n")
	w("- To rebuild Prata from docs alone, read in the order in §1; this pack embeds the\n")
	w("  highest-value pieces so you need not chase links for the essentials.\n\n")

	// --- 1. Read order (authored) ---
	w("## 1. Read order (what answers what)\n\n")
	w("| Read | For | Role |\n| --- | --- | --- |\n")
	w("| `PROJECT-IDENTITY.md` | module path, build cmd, absent secrets, traps | SOURCE |\n")
	w("| `PRATA-MASTER.md` | what it is + how it was reasoned, at a glance | SOURCE |\n")
	w("| `CONSTANTS.md` | every load-bearing constant + why | SOURCE |\n")
	w("| `DECISIONS-REJECTED.md` | dead ends + Status + Re-try trigger | SOURCE |\n")
	w("| `PRATA-DESIGN-LOG.md` | the full reasoning dialogue (dated) | SOURCE |\n")
	w("| `PRATA-GPU-SERVER.md` | backend/server/network setup | SOURCE |\n")
	w("| `PRATA-REVIEW.md` §15 | open vs resolved questions | DERIVED |\n")
	w("| `CHANGELOG.md` | release/work history | SOURCE |\n")
	w("\n")

	// --- 2. Identity (embedded) ---
	w("## 2. Identity (embedded: PROJECT-IDENTITY.md)\n\n")
	if id := readFile("PROJECT-IDENTITY.md"); id != "" {
		w("%s\n\n", strings.TrimSpace(stripFirstHeading(id)))
	} else {
		w("_(PROJECT-IDENTITY.md not found)_\n\n")
	}

	// --- 3. Pinned facts (extracted from code) ---
	w("## 3. Pinned facts — auto-extracted from the code\n\n")
	w("Every value below is read live from the source file named. If one changes in code, this\n")
	w("table changes, and the CI drift gate forces this pack to be regenerated.\n\n")
	w("| Fact | Value | Source | Why it matters |\n| --- | --- | --- | --- |\n")
	missing := 0
	for _, f := range facts {
		content := readFile(f.file)
		val, ok := extractValue(content, f.re)
		if !ok {
			val = "⚠️ NOT FOUND"
			missing++
		}
		w("| `%s` | `%s` | `%s` | %s |\n", f.label, val, f.file, f.note)
	}
	w("\n")

	// --- 4. Negative knowledge (embedded index) ---
	w("## 4. Negative knowledge — rejected paths (index from DECISIONS-REJECTED.md)\n\n")
	rej := readFile("DECISIONS-REJECTED.md")
	if rej != "" {
		n := countMatches(rej, rejRowRe)
		w("**%d rejected/abandoned paths recorded.** A rebuild must not re-tread these.\n", n)
		w("`Status: LOCKED` = never revisit; `DEFERRED` = parked pending the `Re-try trigger`.\n\n")
		if table, ok := extractSection(rej, "| ID | Rejected", "Class legend"); ok {
			w("%s\n\n", strings.TrimSpace(table))
		}
		w("Full detail (Status + Re-try trigger per item): `DECISIONS-REJECTED.md`.\n")
		w("Dated narratives: `PRATA-DESIGN-LOG.md`. Open threads: `PRATA-REVIEW.md` §15.\n\n")
	} else {
		w("_(DECISIONS-REJECTED.md not found)_\n\n")
	}

	// --- 5. Deeper ---
	w("## 5. Where to go deeper\n\n")
	w("- Full \"what + why\" synthesis: `PRATA-MASTER.md`.\n")
	w("- The reasoning dialogue behind each decision: `PRATA-DESIGN-LOG.md` (dated).\n")
	w("- Backend / server / network runbook: `PRATA-GPU-SERVER.md`.\n")
	w("- How to work on the project + doc-freshness rules: `AGENTS.md`.\n")

	if missing > 0 {
		fmt.Fprintf(os.Stderr, "gen-context-pack: warning: %d pinned fact(s) NOT FOUND — a regex or a source moved\n", missing)
	}

	if _, err := os.Stdout.WriteString(b.String()); err != nil {
		fmt.Fprintln(os.Stderr, "gen-context-pack:", err)
		os.Exit(1)
	}
}

// stripFirstHeading drops a leading "# ..." line so an embedded doc does not inject
// a second H1 into the pack.
func stripFirstHeading(doc string) string {
	doc = strings.TrimLeft(doc, "\n")
	if strings.HasPrefix(doc, "# ") {
		if nl := strings.IndexByte(doc, '\n'); nl >= 0 {
			return doc[nl+1:]
		}
	}
	return doc
}
