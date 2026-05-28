// Command prata runs the full push-to-talk loop: Ctrl+Win held →
// microphone capture; release → encode, transcribe, correct, and inject
// the text into the foreground window. Exits on Ctrl+C in the terminal.
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
	"time"

	"github.com/carlosriveros/prata/internal/audio"
	"github.com/carlosriveros/prata/internal/auth"
	"github.com/carlosriveros/prata/internal/cue"
	"github.com/carlosriveros/prata/internal/dict"
	"github.com/carlosriveros/prata/internal/hotkey"
	"github.com/carlosriveros/prata/internal/inject"
	"github.com/carlosriveros/prata/internal/single"
	"github.com/carlosriveros/prata/internal/transcribe"
)

// event is what the hook callback enqueues for the processor goroutine.
// Using a typed enum keeps the channel small and self-documenting.
type event int

const (
	evPress event = iota
	evRelease
)

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

	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, os.Interrupt)

	fmt.Fprintln(os.Stderr, "PTT ready. Hold Ctrl+Win to dictate. Ctrl+C to quit.")

	select {
	case <-sigs:
		listener.Stop()
		<-listenerDone
		close(events)
		<-processorDone
	case err := <-listenerDone:
		close(events)
		<-processorDone
		if err != nil {
			fmt.Fprintf(os.Stderr, "listener error: %v\n", err)
			os.Exit(1)
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
			if !strings.HasSuffix(text, "\n") {
				text += "\n"
			}
			elapsed := time.Since(start)
			if err := inject.Type(text); err != nil {
				fmt.Fprintf(os.Stderr, "inject: %v\n", err)
				continue
			}
			fmt.Fprintf(os.Stderr, "injected %q (%.2fs)\n", text, elapsed.Seconds())
		}
	}
}
