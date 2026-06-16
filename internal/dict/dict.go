// Package dict applies word-boundary text replacements from a
// dictionary file to correct common Whisper transcription errors
// (e.g. "gangliocid" → "gangliosid", "adoption" → "abduktion").
//
// Format of the dictionary file (one rule per line):
//
//	misspelling = correction
//
// Lines starting with '#' and blank lines are ignored. Matching is
// case-sensitive and uses Unicode-aware word boundaries: a word
// character is [\p{L}\p{N}_], so keys that touch å/ä/ö match correctly
// (Go's regexp \b is ASCII-only and would mis-handle them).
//
// Rules apply in file order and chain across distinct keys: with
// "a = b" before "b = c", Apply turns "a" into "c". For one key the
// first matching rule wins — once it rewrites the text, a later rule
// with the same key has nothing left to match, so a duplicate key
// further down the file is dead. Save deduplicates on write by
// replacing the existing line in place rather than appending a second.
//
// The active dictionary is built in two layers (see LoadDefault):
//
//	baseline  embedded in the binary at build time (immutable, ships to
//	          every user); the single source folded-in valuable terms.
//	override  a per-user file (PRATA_DICT_PATH or
//	          %LOCALAPPDATA%\Prata\dictionary-corrections.txt) layered on
//	          top — an override rule adds to, or replaces by key, a baseline
//	          rule. F9/Save writes only the override; the baseline is never
//	          touched and nothing is ever written next to the executable
//	          (its directory is read-only once installed under ProgramFiles).
package dict

import (
	"bufio"
	_ "embed"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"unicode"
	"unicode/utf8"
)

// baselineData is the immutable baseline dictionary embedded at build time.
// It always loads, even when no override file exists, so corrections are
// never silently disabled (and `go run` works without a file next to the
// build-cache executable). Valuable override entries are folded into this
// file before a release build (see PRATA-DESIGN-LOG, fold-in tool).
//
//go:embed dictionary-corrections.txt
var baselineData string

type rule struct {
	key         string
	replacement string
}

// Dict is an ordered list of word-boundary replacement rules.
type Dict struct {
	rules []rule
	// path is the file the rules were last read from, used by Reload.
	// It is empty when the Dict was built from an arbitrary io.Reader;
	// Reload then falls back to the default location (see resolvePath).
	path string
}

// Load parses a dictionary file from r. Returns a Dict ready for
// Apply, or an error if any line is malformed.
func Load(r io.Reader) (*Dict, error) {
	rules, err := parse(r)
	if err != nil {
		return nil, err
	}
	return &Dict{rules: rules}, nil
}

// LoadDefault builds the active dictionary: the embedded baseline with the
// per-user override file (resolvePath) layered on top. A missing override
// file is fine — the baseline alone applies. The returned Dict remembers the
// override path, so Reload re-layers from the same place.
func LoadDefault() (*Dict, error) {
	path, err := resolvePath()
	if err != nil {
		return nil, err
	}
	rules, err := loadLayered(baselineData, path)
	if err != nil {
		return nil, err
	}
	return &Dict{rules: rules, path: path}, nil
}

// loadLayered parses baseline, then merges the override file at overridePath
// on top (mergeRules). A non-existent override file is not an error: the
// baseline rules are returned unchanged. Shared by LoadDefault and Reload.
func loadLayered(baseline, overridePath string) ([]rule, error) {
	base, err := parse(strings.NewReader(baseline))
	if err != nil {
		return nil, fmt.Errorf("parse baseline: %w", err)
	}
	data, err := os.ReadFile(overridePath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return base, nil
		}
		return nil, fmt.Errorf("read override %s: %w", overridePath, err)
	}
	over, err := parse(strings.NewReader(string(data)))
	if err != nil {
		return nil, err
	}
	return mergeRules(base, over), nil
}

// mergeRules layers override rules on top of base. An override rule whose key
// already exists in base replaces that rule in place — at the first (and
// only firing) occurrence, so override wins under first-match-wins Apply. A
// new key is appended after the baseline. Within override, a repeated key
// follows the same rule, so the last occurrence wins (matching Save).
func mergeRules(base, override []rule) []rule {
	merged := make([]rule, len(base))
	copy(merged, base)

	firstIdx := make(map[string]int, len(merged))
	for i, r := range merged {
		if _, ok := firstIdx[r.key]; !ok {
			firstIdx[r.key] = i
		}
	}
	for _, r := range override {
		if i, ok := firstIdx[r.key]; ok {
			merged[i] = r
		} else {
			firstIdx[r.key] = len(merged)
			merged = append(merged, r)
		}
	}
	return merged
}

// parse reads dictionary rules from r in file order. It is shared by
// Load and Reload.
func parse(r io.Reader) ([]rule, error) {
	var rules []rule
	scanner := bufio.NewScanner(r)

	lineNum := 0
	for scanner.Scan() {
		lineNum++
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		idx := strings.Index(line, "=")
		if idx < 0 {
			return nil, fmt.Errorf("line %d: missing '=' separator", lineNum)
		}

		key := strings.TrimSpace(line[:idx])
		val := strings.TrimSpace(line[idx+1:])
		if key == "" {
			return nil, fmt.Errorf("line %d: empty key", lineNum)
		}

		// The key is matched literally (see replaceWholeWord) and the
		// replacement is inserted verbatim — no regexp, hence no
		// QuoteMeta on the key and no "$" escaping on the value.
		rules = append(rules, rule{key: key, replacement: val})
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read: %w", err)
	}

	return rules, nil
}

// Apply runs every rule in order on text and returns the corrected
// string. Rules chain: each one sees the previous rule's output.
func (d *Dict) Apply(text string) string {
	for _, r := range d.rules {
		text = replaceWholeWord(text, r.key, r.replacement)
	}
	return text
}

// replaceWholeWord replaces every whole-word occurrence of key in text
// with repl. A word is delimited by the string edges or by a non-word
// rune; a word character is [\p{L}\p{N}_].
//
// The boundary check is rune-aware on purpose: byte indexing would
// split å/ä/ö and reintroduce the very bug this fixes. We also cannot
// express it as a single regexp — RE2 has no Unicode \b and no
// lookaround, and a naive "(^|non-word)key(non-word|$)" would consume
// the boundary rune and miss adjacent matches. So we scan for the
// literal key and rebuild the string forward, skipping occurrences
// that are not whole words.
func replaceWholeWord(text, key, repl string) string {
	if key == "" {
		return text
	}

	var b strings.Builder
	lastEnd := 0
	from := 0
	for {
		i := strings.Index(text[from:], key)
		if i < 0 {
			break
		}
		start := from + i
		end := start + len(key)

		if isWordBoundary(text, start, end) {
			b.WriteString(text[lastEnd:start])
			b.WriteString(repl)
			lastEnd = end
			from = end
		} else {
			// Not a whole word: step one byte past this occurrence's
			// start so overlapping/adjacent occurrences are still found.
			from = start + 1
		}
	}

	if lastEnd == 0 {
		return text // nothing replaced; avoid copying
	}
	b.WriteString(text[lastEnd:])
	return b.String()
}

// isWordBoundary reports whether the span [start,end) in text is
// delimited on both sides by a word boundary: a string edge or a
// non-word rune.
func isWordBoundary(text string, start, end int) bool {
	if start > 0 {
		r, _ := utf8.DecodeLastRuneInString(text[:start])
		if isWordChar(r) {
			return false
		}
	}
	if end < len(text) {
		r, _ := utf8.DecodeRuneInString(text[end:])
		if isWordChar(r) {
			return false
		}
	}
	return true
}

// isWordChar reports whether r is a word character ([\p{L}\p{N}_]).
func isWordChar(r rune) bool {
	return r == '_' || unicode.IsLetter(r) || unicode.IsNumber(r)
}

// Reload rebuilds this Dict's rules in place from the embedded baseline plus
// the override file, so a long-running instance picks up rules added by Save.
// The override path is the one this Dict was loaded with when known,
// otherwise the default location (resolvePath). On error the existing rules
// are kept.
func (d *Dict) Reload() error {
	path := d.path
	if path == "" {
		p, err := resolvePath()
		if err != nil {
			return err
		}
		path = p
	}

	rules, err := loadLayered(baselineData, path)
	if err != nil {
		return err
	}

	d.rules = rules
	d.path = path
	return nil
}

// Save adds or updates the rule "wrong = correct" in the per-user override
// file (resolvePath) and reports whether a write happened. The embedded
// baseline is never modified, and nothing is ever written next to the
// executable.
//
// Both fields are trimmed. Nothing is written — (false, nil) — when
// either field is empty or the rule is an identity (wrong == correct).
//
// Deduplication happens on write: if a rule with the same key already
// exists, its line is replaced in place; otherwise the new rule is
// appended. This is required for correctness, not tidiness — matching
// is first-match-wins, so a duplicate appended at the end would never
// fire. Comments, blank lines, and unrelated rules are preserved
// verbatim. A missing override file (and its parent directory) is created.
func Save(wrong, correct string) (bool, error) {
	wrong = strings.TrimSpace(wrong)
	correct = strings.TrimSpace(correct)
	if wrong == "" || correct == "" || wrong == correct {
		return false, nil
	}

	path, err := resolvePath()
	if err != nil {
		return false, err
	}

	data, err := os.ReadFile(path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return false, fmt.Errorf("read dictionary: %w", err)
	}
	content := string(data)

	// Preserve the file's existing newline style; default to "\n".
	newline := "\n"
	if strings.Contains(content, "\r\n") {
		newline = "\r\n"
	}

	// Split into logical lines without carriage returns or the phantom
	// empty element a trailing newline produces.
	var lines []string
	if content != "" {
		norm := strings.ReplaceAll(content, "\r\n", "\n")
		norm = strings.TrimSuffix(norm, "\n")
		lines = strings.Split(norm, "\n")
	}

	newLine := wrong + " = " + correct
	replaced := false
	for i, line := range lines {
		t := strings.TrimSpace(line)
		if t == "" || strings.HasPrefix(t, "#") {
			continue
		}
		eq := strings.Index(t, "=")
		if eq < 0 {
			continue
		}
		if strings.TrimSpace(t[:eq]) == wrong {
			lines[i] = newLine
			replaced = true
			break
		}
	}
	if !replaced {
		lines = append(lines, newLine)
	}

	out := strings.Join(lines, newline) + newline
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return false, fmt.Errorf("create override directory: %w", err)
	}
	if err := os.WriteFile(path, []byte(out), 0o644); err != nil {
		return false, fmt.Errorf("write dictionary: %w", err)
	}
	return true, nil
}

// resolvePath returns the per-user override file location, in priority order:
//
//  1. PRATA_DICT_PATH (env) — highest priority, for development.
//  2. %LOCALAPPDATA%\Prata\dictionary-corrections.txt — the normal location.
//
// The baseline always comes from the embedded copy; this returns only the
// override path. cmd/prata's loadDict resolves the same way (it delegates to
// LoadDefault), so the two never disagree. The path is deliberately not next
// to the executable: ProgramFiles is read-only for non-admins once installed.
func resolvePath() (string, error) {
	if p := os.Getenv("PRATA_DICT_PATH"); p != "" {
		return p, nil
	}
	return filepath.Join(os.Getenv("LOCALAPPDATA"), "Prata", "dictionary-corrections.txt"), nil
}
