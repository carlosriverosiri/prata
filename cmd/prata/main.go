// Command prata runs the full push-to-talk loop: F1 held →
// microphone capture; release → encode, transcribe, correct, and inject
// the text into the foreground window. Quit via the system-tray "Avsluta"
// menu (or Ctrl+C when run from a terminal).
//
// The API key comes from the BERGET_API_KEY environment variable, or a
// DPAPI-encrypted file written by prata-setkey (see internal/auth).
//
// Usage:
//
//	$env:BERGET_API_KEY = "..."
//	prata
package main

import (
	"bytes"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unicode"

	"github.com/carlosriveros/prata/internal/audio"
	"github.com/carlosriveros/prata/internal/auth"
	"github.com/carlosriveros/prata/internal/cue"
	"github.com/carlosriveros/prata/internal/dict"
	"github.com/carlosriveros/prata/internal/hotkey"
	"github.com/carlosriveros/prata/internal/icon"
	"github.com/carlosriveros/prata/internal/inject"
	"github.com/carlosriveros/prata/internal/popup"
	"github.com/carlosriveros/prata/internal/sanity"
	"github.com/carlosriveros/prata/internal/single"
	"github.com/carlosriveros/prata/internal/transcribe"
	"github.com/carlosriveros/prata/internal/tray"
)

// event is what the listener enqueues for the processor goroutine.
// Using a typed enum keeps the channel small and self-documenting.
type event int

const (
	evPress event = iota
	evRelease
)

// dictAdd is a correction rule captured by the F8 quick-fix flow and
// handed to the processor goroutine, which owns the *dict.Dict and is the
// only goroutine allowed to Save/Reload it — so no lock is needed.
type dictAdd struct {
	wrong, correct string
}

// f8Busy is the single-flight guard for the F8 quick-fix flow: at most one
// grab → popup → paste-back runs at a time. A second F8 tap while one is in
// flight is dropped.
var f8Busy atomic.Bool

// minCaptureBytes is the smallest PCM payload worth transcribing,
// roughly 0.1s of audio. Derived from the transcribe format constants
// so it tracks the sample rate.
const minCaptureBytes = transcribe.SampleRate * transcribe.NumChannels * transcribe.BitsPerSample / 8 / 10

// transcribeResult carries a finished transcription from the worker
// goroutine back to the processor. Keeping the blocking Berget call off the
// processor means a slow response never freezes F1 capture.
type transcribeResult struct {
	text    string
	elapsed time.Duration
	err     error
}

// transcribeQueueDepth bounds how many finished captures can wait for
// transcription. A single slow Berget round (~24s observed) must not freeze
// capture, but an unbounded queue under a sustained outage would hide the
// failure and pile up stale audio; past this depth the processor drops the
// capture with an error cue so the user re-dictates instead.
const transcribeQueueDepth = 8

// loadDict resolves the dictionary path (PRATA_DICT_PATH env var, or
// "dictionary-corrections.txt" next to the executable as a fallback)
// and returns the parsed Dict. A nil return paired with a non-nil
// error means dict corrections will be disabled but the app should
// still run.
func loadDict() (*dict.Dict, error) {
	path := os.Getenv("PRATA_DICT_PATH")
	if path == "" {
		exe, err := os.Executable()
		if err != nil {
			return nil, fmt.Errorf("locate executable: %w", err)
		}
		path = filepath.Join(filepath.Dir(exe), "dictionary-corrections.txt")
	}

	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", path, err)
	}
	defer f.Close()

	return dict.Load(f)
}

func main() {
	// Per-monitor DPI awareness must be set before any window or HICON is
	// created (the tray icon below), so it renders crisp on scaled displays.
	tray.SetProcessDPIAware()

	// Refuse to start if another Prata is already running. Two instances
	// share F1 and would both capture and inject, producing
	// duplicate output (or garbled output in async target apps).
	if !single.Acquire("PrataSingleInstanceMutex") {
		fmt.Fprintln(os.Stderr, "Prata is already running; exiting.")
		return
	}

	apiKey := os.Getenv("BERGET_API_KEY")
	if apiKey == "" {
		var err error
		apiKey, err = auth.LoadAPIKey()
		if err != nil {
			fmt.Fprintln(os.Stderr, "no API key available:")
			fmt.Fprintln(os.Stderr, "  set BERGET_API_KEY env var, or")
			fmt.Fprintln(os.Stderr, "  run prata-setkey to save an encrypted key")
			os.Exit(1)
		}
	}

	d, err := loadDict()
	if err != nil {
		fmt.Fprintf(os.Stderr, "warn: dictionary disabled (%v)\n", err)
		// d will be nil here; processEvents handles nil gracefully
	} else {
		fmt.Fprintln(os.Stderr, "dictionary loaded")
	}

	client := transcribe.NewClient(apiKey)

	// Buffered so the listener's message-loop goroutine never blocks.
	// Size 4 covers any realistic press/release burst.
	events := make(chan event, 4)

	listener := hotkey.NewListener(
		func() { events <- evPress },
		func() { events <- evRelease },
	)

	// F8 quick-fix: grab the foreground selection, let the user correct it
	// in a popup, persist the rule, and paste the correction back. The dict
	// save+reload is handed over dictAdds to the processor goroutine, which
	// owns d. SetOnF8 must run before listener.Run for the same
	// happens-before guarantee the press/release callbacks rely on.
	dictAdds := make(chan dictAdd, 1)
	listener.SetOnF8(func() {
		// Listener goroutine: must return quickly to keep the message
		// loop responsive. Do only the cheap, time-critical work (grab
		// the foreground HWND before focus changes) and hand the rest
		// to a goroutine.
		if !f8Busy.CompareAndSwap(false, true) {
			return // a quick-fix is already in flight; drop this tap
		}
		sourceHwnd, ok := inject.ForegroundWindow()
		if !ok {
			f8Busy.Store(false)
			return
		}
		go f8Worker(sourceHwnd, dictAdds)
	})

	// Listener goroutine: pins itself to its OS thread and runs the
	// Windows message loop until Stop is called.
	listenerDone := make(chan error, 1)
	go func() {
		listenerDone <- listener.Run()
	}()

	// Transcription worker: the only goroutine that makes the blocking
	// Berget call. Decoupling it from the processor keeps F1 capture
	// responsive even when one request is slow (~24s seen during a Berget
	// hiccup). A single worker draining a FIFO queue preserves dictation
	// order — injected text always matches the order spoken.
	jobs := make(chan []byte, transcribeQueueDepth)
	results := make(chan transcribeResult, transcribeQueueDepth)
	go func() {
		for pcm := range jobs {
			start := time.Now()
			text, terr := client.Transcribe(bytes.NewReader(transcribe.EncodePCM(pcm)))
			results <- transcribeResult{text: text, elapsed: time.Since(start), err: terr}
		}
		close(results)
	}()

	// Processor goroutine: drains events sequentially, owning the
	// audio.Session lifecycle, and applies + injects finished
	// transcriptions. Single-goroutine ownership means no mutex is needed on
	// the session pointer or the dictionary.
	processorDone := make(chan struct{})
	go func() {
		defer close(processorDone)
		processEvents(d, events, dictAdds, jobs, results)
	}()

	// System-tray icon. Its only menu item, Avsluta, requests shutdown by
	// closing quit. onQuit runs on the tray's UI thread, must return fast,
	// and must not call t.Stop() (the tray posts its own WM_QUIT) — it only
	// nudges the shared shutdown path below. quitOnce makes repeat Avsluta
	// clicks harmless. In production (-H windowsgui, no console) Avsluta is
	// the only graceful quit path, since Ctrl+C is never delivered.
	quit := make(chan struct{})
	var quitOnce sync.Once
	t := tray.New(icon.ICO, "Prata", func() {
		quitOnce.Do(func() { close(quit) })
	})
	trayDone := make(chan error, 1)
	go func() {
		trayDone <- t.Run()
	}()
	trayAlive := true

	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, os.Interrupt)

	fmt.Fprintln(os.Stderr, "PTT ready. Hold F1 to dictate. Ctrl+C to quit.")

	// shutdown is the teardown shared by Ctrl+C and tray Avsluta: stop the
	// listener, drain the processor, then stop the tray — but only if the
	// tray is still running, since one that failed to start has already
	// returned (and its trayDone has been niled below).
	shutdown := func() {
		listener.Stop()
		<-listenerDone
		close(events)
		<-processorDone
		if trayAlive {
			t.Stop()
			<-trayDone
		}
	}

	for {
		select {
		case <-sigs:
			shutdown()
			return
		case <-quit:
			shutdown()
			return
		case err := <-listenerDone:
			// Listener returned on its own; tear down the rest and exit.
			close(events)
			<-processorDone
			if trayAlive {
				t.Stop()
				<-trayDone
			}
			if err != nil {
				fmt.Fprintf(os.Stderr, "listener error: %v\n", err)
				os.Exit(1)
			}
			return
		case err := <-trayDone:
			// Tray Run returned. A non-nil error is a fundamental setup
			// failure; the icon is only a convenience, so log it and keep
			// dictating — the same soft-degrade policy as the dictionary.
			// Nil the channel so this case can't re-fire, and mark the tray
			// dead so shutdown skips waiting on it.
			trayAlive = false
			trayDone = nil
			if err != nil {
				fmt.Fprintf(os.Stderr, "warn: tray disabled (%v)\n", err)
			}
		}
	}
}

// processEvents drains the event channel sequentially, managing the
// audio.Session lifecycle and handing finished captures to the transcription
// worker over jobs. Transcribed text comes back over results, where the
// dictionary is applied and the text injected — all on this one goroutine,
// so d needs no lock. Transcription itself runs on the worker, so a slow
// Berget response never blocks the next F1 press.
//
// Defensive: ignores duplicate press while already recording, and
// release without an active session. With the current state machine in
// internal/hotkey these can't fire, but the cost of the guard is
// trivial and protects against future listener-state regressions.
func processEvents(d *dict.Dict, events <-chan event, dictAdds <-chan dictAdd, jobs chan<- []byte, results <-chan transcribeResult) {
	var session *audio.Session

	for {
		select {
		case ev, ok := <-events:
			if !ok {
				// Shutdown: close(events) ends the loop. Close jobs too so
				// the worker drains and exits; any in-flight result is
				// abandoned (the process is exiting anyway).
				close(jobs)
				return
			}
			switch ev {
			case evPress:
				if session != nil {
					continue
				}
				s, err := audio.Start()
				if err != nil {
					fmt.Fprintf(os.Stderr, "audio start: %v\n", err)
					continue
				}
				session = s
				cue.PlayStart()
				fmt.Fprintln(os.Stderr, "recording...")

			case evRelease:
				if session == nil {
					continue
				}
				pcm, err := session.Stop()
				session = nil
				if err != nil {
					fmt.Fprintf(os.Stderr, "audio stop: %v\n", err)
					continue
				}
				cue.PlayStop()

				// An empty / near-empty capture (e.g. an accidental brief
				// tap) would otherwise be sent to Berget and block for the
				// full 30s HTTP timeout before failing. Skip it instead.
				if len(pcm) < minCaptureBytes {
					fmt.Fprintln(os.Stderr, "no audio captured, skipping")
					continue
				}

				// Hand the capture to the transcription worker instead of
				// transcribing inline, so a slow Berget response never
				// freezes the next F1 press. Non-blocking: if the queue is
				// full (sustained outage) drop with an error cue rather than
				// stalling capture or piling up stale audio.
				select {
				case jobs <- pcm:
					fmt.Fprintf(os.Stderr, "captured %d bytes, transcribing...\n", len(pcm))
				default:
					fmt.Fprintln(os.Stderr, "transcription queue full, dropping capture")
					cue.PlayError()
				}
			}

		case res := <-results:
			// A finished transcription from the worker: apply corrections,
			// guard against degenerate output, and inject. Production runs
			// -H windowsgui (no console), so the error cue is the only
			// failure signal on each discard path below.
			if res.err != nil {
				fmt.Fprintf(os.Stderr, "transcribe: %v\n", res.err)
				cue.PlayError()
				continue
			}
			text := res.text
			if d != nil {
				text = d.Apply(text)
			}
			// Empty / whitespace-only result (e.g. a short capture with
			// no clear speech) would otherwise inject a bare newline.
			if strings.TrimSpace(text) == "" {
				fmt.Fprintf(os.Stderr, "empty transcription, skipping (%.2fs)\n", res.elapsed.Seconds())
				cue.PlayError()
				continue
			}
			// A Whisper repetition loop (common on long digit strings)
			// would otherwise be injected verbatim into the patient
			// journal — a safety hazard, not just noise. Discard it and
			// log a prefix so the dropped text stays visible and the
			// user can re-dictate.
			if sanity.IsDegenerate(text) {
				fmt.Fprintf(os.Stderr, "discarded degenerate transcription (ratio %.1f), skipping: %q\n", sanity.Ratio(text), preview(text, 80))
				cue.PlayError()
				continue
			}
			if !strings.HasSuffix(text, "\n") {
				text += "\n"
			}
			if err := inject.TypeAuto(text); err != nil {
				fmt.Fprintf(os.Stderr, "inject: %v\n", err)
				cue.PlayError()
				continue
			}
			fmt.Fprintf(os.Stderr, "injected %q (%.2fs)\n", text, res.elapsed.Seconds())
		case da := <-dictAdds:
			// Only this goroutine touches d, so Save+Reload need no lock.
			// dict.Save persists the rule even when d is nil (corrections
			// disabled at startup); reload the running dict only if there
			// is one.
			saved, err := dict.Save(da.wrong, da.correct)
			if err != nil {
				fmt.Fprintf(os.Stderr, "dict save: %v\n", err)
			}
			switch {
			case saved && d != nil:
				if rerr := d.Reload(); rerr != nil {
					fmt.Fprintf(os.Stderr, "dict reload: %v\n", rerr)
				} else {
					fmt.Fprintf(os.Stderr, "dict rule saved: %q = %q\n", da.wrong, da.correct)
				}
			case saved:
				fmt.Fprintf(os.Stderr, "dict rule saved (corrections disabled until restart): %q = %q\n", da.wrong, da.correct)
			}
		}
	}
}

// preview returns the first n runes of s for log output, appending an
// ellipsis when truncated. Rune-based so Swedish characters (å, ä, ö) are
// never split mid-byte in the log.
func preview(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "..."
}

// f8Worker runs the F8 quick-fix flow off the listener goroutine: grab the
// foreground selection, let the user correct it in a popup, hand the rule
// to the processor goroutine for save+reload, restore focus to the source
// window, and paste the corrected text back over the selection. It always
// clears the single-flight guard on return.
func f8Worker(sourceHwnd uintptr, dictAdds chan<- dictAdd) {
	defer f8Busy.Store(false)

	sel, ok, err := inject.CopySelection()
	if err != nil {
		fmt.Fprintf(os.Stderr, "f8 copy selection: %v\n", err)
		return
	}
	if !ok {
		return // nothing selected; clipboard already restored by CopySelection
	}

	leading, core, trailing := splitEnvelope(sel)
	if core == "" {
		return // selection was empty or all whitespace
	}

	edited, ok, err := popup.Prompt(core)
	if err != nil {
		fmt.Fprintf(os.Stderr, "f8 popup: %v\n", err)
		return
	}
	if !ok {
		return // Esc / clicked away: no save, no paste-back
	}
	edited = strings.TrimSpace(edited)
	if edited == "" || edited == core {
		// No-op: never delete the selected word, and skip re-injecting
		// unchanged text. This guard gates PASTE-BACK and is NOT redundant
		// with dict.Save's own identity filter — do not remove it.
		return
	}

	// Hand the rule to the processor goroutine (it owns d). Non-blocking, so
	// the worker never blocks or leaks if the processor is busy or has
	// already exited at shutdown. Sent before the foreground gate so the
	// rule persists even if paste-back is aborted.
	select {
	case dictAdds <- dictAdd{wrong: core, correct: edited}:
	default:
		fmt.Fprintln(os.Stderr, "dict rule dropped (processor busy)")
	}

	// Foreground gate: only paste once the source window is confirmed
	// foreground again, so a correction never lands in the wrong window.
	ok, err = inject.RestoreForeground(sourceHwnd)
	if err != nil || !ok {
		fmt.Fprintf(os.Stderr, "f8 restore foreground failed (ok=%v): %v\n", ok, err)
		return
	}

	// Paste-back over the still-selected word. No trailing newline (unlike
	// the dictation path) — this replaces an inline selection.
	if err := inject.TypeAuto(leading + edited + trailing); err != nil {
		fmt.Fprintf(os.Stderr, "f8 paste-back: %v\n", err)
	}
}

// splitEnvelope splits s into its leading whitespace run, the trimmed core,
// and its trailing whitespace run, so the F8 flow can show/save the core
// while reapplying the exact surrounding whitespace on paste-back. Splits
// are byte-offset-exact and rune-aware via unicode.IsSpace.
func splitEnvelope(s string) (leading, core, trailing string) {
	rest := strings.TrimLeftFunc(s, unicode.IsSpace)
	leading = s[:len(s)-len(rest)]
	core = strings.TrimRightFunc(rest, unicode.IsSpace)
	trailing = rest[len(core):]
	return leading, core, trailing
}
