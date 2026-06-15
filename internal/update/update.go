// Package update checks GitHub Releases for a newer Prata version. It only
// reports whether a newer release exists and where to get it — it never
// downloads or installs anything. The actual upgrade goes through
// install.ps1, the single tested install/upgrade path. This keeps the
// running binary from having to overwrite itself (impossible while running
// without the rename dance) and avoids the download-and-execute behaviour
// that behavioural AV/EDR products flag (see PRATA-DESIGN-LOG.md, the
// unsigned-binary Webroot ADR).
package update

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// latestReleaseURL is GitHub's "latest release" endpoint for the Prata repo
// (the same owner/name install.ps1 downloads from). It returns the most
// recent non-draft, non-prerelease release.
const latestReleaseURL = "https://api.github.com/repos/carlosriverosiri/prata/releases/latest"

// httpTimeout bounds the whole request so a "check for update" click can
// never hang the goroutine waiting on an unresponsive network.
const httpTimeout = 10 * time.Second

// Result describes the outcome of a version check.
type Result struct {
	Current    string // version embedded in the running binary ("dev" for local builds)
	Latest     string // tag_name of the latest GitHub release, e.g. "v0.2.0"
	URL        string // html_url of that release, for the user to open
	Newer      bool   // true when Latest is strictly newer than Current
	Comparable bool   // true when both versions parsed as numeric vX.Y.Z
}

// Check queries GitHub for the latest release and compares it to current.
// A "dev"/non-numeric current can't be ordered against a real tag, so
// Comparable is false and Newer is false in that case — local builds never
// nag — while Latest is still returned so the caller can surface it.
func Check(current string) (Result, error) {
	req, err := http.NewRequest(http.MethodGet, latestReleaseURL, nil)
	if err != nil {
		return Result{}, fmt.Errorf("build request: %w", err)
	}
	// GitHub rejects requests without a User-Agent; the documented media
	// type pins the response shape.
	req.Header.Set("User-Agent", "prata-update-check")
	req.Header.Set("Accept", "application/vnd.github+json")

	client := &http.Client{Timeout: httpTimeout}
	resp, err := client.Do(req)
	if err != nil {
		return Result{}, fmt.Errorf("contact GitHub: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return Result{}, fmt.Errorf("GitHub returned %s", resp.Status)
	}

	var payload struct {
		TagName string `json:"tag_name"`
		HTMLURL string `json:"html_url"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return Result{}, fmt.Errorf("decode release: %w", err)
	}
	if payload.TagName == "" {
		return Result{}, fmt.Errorf("release has no tag_name")
	}

	res := Result{
		Current: current,
		Latest:  payload.TagName,
		URL:     payload.HTMLURL,
	}
	cur, curOK := parseVersion(current)
	lat, latOK := parseVersion(payload.TagName)
	res.Comparable = curOK && latOK
	if res.Comparable {
		res.Newer = greater(lat, cur)
	}
	return res, nil
}

// parseVersion turns "v1.2.3" into [3]int{1,2,3}. A leading "v" and any
// pre-release ("-rc1") or build ("+meta") suffix are ignored; a missing
// minor/patch defaults to 0. ok is false when the string isn't a numeric
// dotted version (e.g. "dev", "", or a bare git short-hash).
func parseVersion(s string) ([3]int, bool) {
	s = strings.TrimPrefix(strings.TrimSpace(s), "v")
	if i := strings.IndexAny(s, "-+"); i >= 0 {
		s = s[:i]
	}
	if s == "" {
		return [3]int{}, false
	}
	var out [3]int
	for i, part := range strings.SplitN(s, ".", 3) {
		n, err := strconv.Atoi(part)
		if err != nil || n < 0 {
			return [3]int{}, false
		}
		out[i] = n
	}
	return out, true
}

// greater reports whether version a is strictly higher than b, comparing
// major, then minor, then patch.
func greater(a, b [3]int) bool {
	for i := 0; i < 3; i++ {
		if a[i] != b[i] {
			return a[i] > b[i]
		}
	}
	return false
}
