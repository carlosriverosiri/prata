// Command prata runs the full push-to-talk loop: Ctrl+Win held →
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
	"time"

	"github.com/carlosriveros/prata/internal/audio"
	"github.com/carlosriveros/prata/internal/auth"
	"github.com/carlosriveros/prata/internal/cue"
	"github.com/carlosriveros/prata/internal/dict"
	"github.com/carlosriveros/prata/internal/hotkey"
	"github.com/carlosriveros/prata/internal/icon"
	"github.com/carlosriveros/prata/internal/inject"
	"github.com/carlosriveros/prata/internal/sanity"
	"github.com/carlosriveros/prata/internal/single"
	"github.com/carlosriveros/prata/internal/transcribe"
	"github.com/carlosriveros/prata/internal/tray"
)

// event is what the hook callback enqueues for the processor goroutine.
// Using a typed enum keeps the channel small and self-documenting.
type event int

const (
	evPress event = iota
	evRelease
)

// minCaptureBytes is the smallest PCM payload worth transcribing,
// roughly 0.1s of audio. Derived from the transcribe format constants
// so it tracks the sample rate.
const minCaptureBytes = transcribe.SampleRate * transcribe.NumChannels * transcribe.BitsPerSample / 8 / 10

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
	// share Ctrl+Win and would both capture and inject, producing
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

	// Buffered so the hook callback (which has a 300 ms Windows timeout)
	// never blocks. Size 4 covers any realistic press/release burst.
	events := make(chan event, 4)

	listener := hotkey.NewListener(
		func() { events <- evPress },
		func() { events <- evRelease },
	)

	// Listener goroutine: pins itself to its OS thread and runs the
	// Windows message loop until Stop is called.
	listenerDone := make(chan error, 1)
	go func() {
		listenerDone <- listener.Run()
	}()

	// Processor goroutine: drains events sequentially, owning the
	// audio.Session lifecycle. Single-goroutine ownership means no
	// mutex is needed on the session pointer.
	processorDone := make(chan struct{})
	go func() {
		defer close(processorDone)
		processEvents(client, d, events)
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

	fmt.Fprintln(os.Stderr, "PTT ready. Hold Ctrl+Win to dictate. Ctrl+C to quit.")

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
// audio.Session lifecycle and dispatching to Berget on release.
//
// Defensive: ignores duplicate press while already recording, and
// release without an active session. With the current state machine in
// internal/hotkey these can't fire, but the cost of the guard is
// trivial and protects against future hook-state regressions.
func processEvents(client *transcribe.Client, d *dict.Dict, events <-chan event) {
	var session *audio.Session

	for ev := range events {
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

			fmt.Fprintf(os.Stderr, "captured %d bytes, transcribing...\n", len(pcm))
			start := time.Now()
			text, err := client.Transcribe(bytes.NewReader(transcribe.EncodePCM(pcm)))
			if err != nil {
				fmt.Fprintf(os.Stderr, "transcribe: %v\n", err)
				continue
			}
			if d != nil {
				text = d.Apply(text)
			}
			// Empty / whitespace-only result (e.g. a short capture with
			// no clear speech) would otherwise inject a bare newline.
			if strings.TrimSpace(text) == "" {
				fmt.Fprintf(os.Stderr, "empty transcription, skipping (%.2fs)\n", time.Since(start).Seconds())
				continue
			}
			// A Whisper repetition loop (common on long digit strings)
			// would otherwise be injected verbatim into the patient
			// journal — a safety hazard, not just noise. Discard it and
			// log a prefix so the dropped text stays visible and the
			// user can re-dictate.
			if sanity.IsDegenerate(text) {
				fmt.Fprintf(os.Stderr, "discarded degenerate transcription (ratio %.1f), skipping: %q\n", sanity.Ratio(text), preview(text, 80))
				continue
			}
			if !strings.HasSuffix(text, "\n") {
				text += "\n"
			}
			elapsed := time.Since(start)
			if err := inject.TypeAuto(text); err != nil {
				fmt.Fprintf(os.Stderr, "inject: %v\n", err)
				continue
			}
			fmt.Fprintf(os.Stderr, "injected %q (%.2fs)\n", text, elapsed.Seconds())
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
