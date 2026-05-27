// Package dict applies word-boundary text replacements from a
// dictionary file to correct common Whisper transcription errors
// (e.g. "gangliocid" → "gangliosid", "adoption" → "abduktion").
//
// Format of the dictionary file (one rule per line):
//
//	misspelling = correction
//
// Lines starting with '#' and blank lines are ignored. Matching is
// case-sensitive with ASCII word-boundary semantics. Rules apply in
// file order; later rules see the output of earlier ones.
package dict

import (
	"bufio"
	"fmt"
	"io"
	"regexp"
	"strings"
)

type rule struct {
	pattern     *regexp.Regexp
	replacement string
}

// Dict is an ordered list of word-boundary replacement rules.
type Dict struct {
	rules []rule
}

// Load parses a dictionary file from r. Returns a Dict ready for
// Apply, or an error if any line is malformed.
func Load(r io.Reader) (*Dict, error) {
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

		pattern := `\b` + regexp.QuoteMeta(key) + `\b`
		re, err := regexp.Compile(pattern)
		if err != nil {
			return nil, fmt.Errorf("line %d: compile pattern: %w", lineNum, err)
		}

		// Pre-escape any $ in the replacement so that "$1" etc.
		// aren't interpreted as backreferences by ReplaceAllString.
		safeRepl := strings.ReplaceAll(val, `$`, `$$`)
		rules = append(rules, rule{pattern: re, replacement: safeRepl})
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read: %w", err)
	}

	return &Dict{rules: rules}, nil
}

// Apply runs every rule in order on text and returns the corrected
// string.
func (d *Dict) Apply(text string) string {
	for _, r := range d.rules {
		text = r.pattern.ReplaceAllString(text, r.replacement)
	}
	return text
}
