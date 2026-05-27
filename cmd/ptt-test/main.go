// Command ptt-test verifies the full push-to-talk loop:
// Ctrl+Win held → microphone capture; release → encode, transcribe,
// print. Exits on Ctrl+C in the terminal.
//
// Usage:
//
//	$env:BERGET_API_KEY = "..."
//	ptt-test
package main

import (
	"bytes"
	"fmt"
	"os"
	"os/signal"
	"time"

	"github.com/carlosriveros/prata/internal/audio"
	"github.com/carlosriveros/prata/internal/hotkey"
	"github.com/carlosriveros/prata/internal/transcribe"
)

// event is what the hook callback enqueues for the processor goroutine.
// Using a typed enum keeps the channel small and self-documenting.
type event int

const (
	evPress event = iota
	evRelease
)

func main() {
	apiKey := os.Getenv("BERGET_API_KEY")
	if apiKey == "" {
		fmt.Fprintln(os.Stderr, "BERGET_API_KEY not set")
		os.Exit(1)
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
		processEvents(client, events)
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
func processEvents(client *transcribe.Client, events <-chan event) {
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

			fmt.Fprintf(os.Stderr, "captured %d bytes, transcribing...\n", len(pcm))
			start := time.Now()
			text, err := client.Transcribe(bytes.NewReader(transcribe.EncodePCM(pcm)))
			if err != nil {
				fmt.Fprintf(os.Stderr, "transcribe: %v\n", err)
				continue
			}
			fmt.Printf("%s (%.2fs)\n", text, time.Since(start).Seconds())
		}
	}
}
