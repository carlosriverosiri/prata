// Package daemonlog provides a minimal, append-mode file log for the Prata
// daemon. In production the daemon is built with -H windowsgui and has no
// console, so everything written to stderr is discarded — there is no durable
// record of what happened on each dictation. This package gives one: a
// per-day log file under %LOCALAPPDATA%\Prata\logs.
//
// It is deliberately tiny — stdlib only, no levels, no structured fields — and
// best-effort throughout: any failure to open or write the log is swallowed,
// never fatal, because losing a diagnostic line must never disrupt dictation.
// The log lines carry only metadata (backend, timings, char counts, errors),
// never the transcribed text itself, so the file is safe by construction even
// though it may sit beside patient work. The pattern mirrors internal/installer's
// logf; this is its daemon-side counterpart.
package daemonlog

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const (
	// envOverride replaces the full log path entirely (test isolation,
	// mirroring PRATA_INSTALL_LOG in the installer).
	envOverride = "PRATA_DAEMON_LOG"

	logPrefix  = "prata-"
	logSuffix  = ".log"
	dateLayout = "2006-01-02"

	// retentionDays bounds how long per-day logs are kept. Prata is a
	// "see and forget" tool that can run for years; without pruning the
	// logs/ directory would grow by one small file per active day forever.
	// Logs older than this are deleted on Open. The window is generous —
	// long enough to investigate an issue reported days later, short enough
	// to stay bounded.
	retentionDays = 30
)

// mu guards the package-level file handle. A single daemon process calls Open
// once at startup and Close once at shutdown (both on main), but Printf is
// reached from the processor goroutine, so the handle needs a mutex.
var (
	mu sync.Mutex
	f  *os.File
)

// closerFunc adapts the package-level Close function to io.Closer so Open can
// hand the caller a value to defer.
type closerFunc func() error

func (c closerFunc) Close() error { return c() }

// noopCloser is returned when Open fails, so the caller can defer Close
// unconditionally without a nil check.
type noopCloser struct{}

func (noopCloser) Close() error { return nil }

// Open creates (if needed) and opens today's log file for appending, storing
// the handle as package state. The returned io.Closer must be deferred by the
// caller. Best-effort: if the directory or file cannot be created, Open returns
// a no-op Closer and a non-nil error — the caller logs to stderr and continues,
// because the daemon must never be fatal over a missing log.
func Open() (io.Closer, error) {
	path := logPath()
	if path == "" {
		return noopCloser{}, fmt.Errorf("daemonlog: LOCALAPPDATA not set")
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return noopCloser{}, fmt.Errorf("daemonlog: create dir: %w", err)
	}
	// Drop logs past the retention window so a years-running daemon never
	// accumulates one file per active day forever. Skipped when the path is
	// overridden (tests): retention targets the real dated logs only.
	if os.Getenv(envOverride) == "" {
		pruneOldLogs(dir, time.Now(), retentionDays)
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return noopCloser{}, fmt.Errorf("daemonlog: open file: %w", err)
	}
	mu.Lock()
	f = file
	mu.Unlock()
	return closerFunc(Close), nil
}

// Close closes the open log file and nils the package handle. Safe to call when
// no file is open (returns nil). It returns the underlying close error, if any.
func Close() error {
	mu.Lock()
	defer mu.Unlock()
	if f == nil {
		return nil
	}
	err := f.Close()
	f = nil
	return err
}

// Printf appends one timestamped line to the open log file:
//
//	2006-01-02 15:04:05  <message>
//
// Best-effort: if no file is open (Printf called before Open, or Open failed)
// or the write fails, the call is a silent no-op — a lost log line must never
// disrupt dictation.
func Printf(format string, args ...any) {
	mu.Lock()
	defer mu.Unlock()
	if f == nil {
		return
	}
	fmt.Fprintf(f, "%s  %s\n", time.Now().Format("2006-01-02 15:04:05"), fmt.Sprintf(format, args...))
}

// logPath returns the full log file path. PRATA_DAEMON_LOG overrides it
// entirely (used for test isolation, mirroring PRATA_INSTALL_LOG in the
// installer). Otherwise it is %LOCALAPPDATA%\Prata\logs\prata-YYYY-MM-DD.log,
// using LOCALAPPDATA directly like the rest of the codebase. Returns "" when
// LOCALAPPDATA is unset and no override is given, so Open reports a clean error
// instead of writing to a bare relative path.
func logPath() string {
	if p := os.Getenv(envOverride); p != "" {
		return p
	}
	local := os.Getenv("LOCALAPPDATA")
	if local == "" {
		return ""
	}
	name := logPrefix + time.Now().Format(dateLayout) + logSuffix
	return filepath.Join(local, "Prata", "logs", name)
}

// pruneOldLogs deletes prata-YYYY-MM-DD.log files in dir whose date is more
// than keepDays before now. Best-effort: an unreadable directory or an
// undeletable file is ignored, and a name that does not match the dated
// pattern is left untouched, so an unrelated file is never removed. The date
// is read from the filename, not the modification time, so a copied or
// touched file is still pruned on its real day.
func pruneOldLogs(dir string, now time.Time, keepDays int) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	cutoff := now.AddDate(0, 0, -keepDays)
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		date, ok := logDate(e.Name())
		if !ok {
			continue
		}
		if date.Before(cutoff) {
			_ = os.Remove(filepath.Join(dir, e.Name()))
		}
	}
}

// logDate extracts the date from a daemon log filename of the form
// prata-YYYY-MM-DD.log (the inverse of logPath's naming). ok is false when
// the name does not match that exact pattern, so callers never act on an
// unrelated file.
func logDate(name string) (time.Time, bool) {
	if !strings.HasPrefix(name, logPrefix) || !strings.HasSuffix(name, logSuffix) {
		return time.Time{}, false
	}
	datePart := name[len(logPrefix) : len(name)-len(logSuffix)]
	t, err := time.Parse(dateLayout, datePart)
	if err != nil {
		return time.Time{}, false
	}
	return t, true
}
