// Command f8-test verifies F8 detection and selection capture in
// isolation (no popup, no dictionary, no inject-back). On each F8 tap it
// grabs the foreground window's current selection via the clipboard and
// prints it to stderr. Exits on Ctrl+C in the terminal.
package main

import (
	"fmt"
	"os"
	"os/signal"

	"github.com/carlosriveros/prata/internal/hotkey"
	"github.com/carlosriveros/prata/internal/inject"
)

func main() {
	listener := hotkey.NewListener(nil, nil)

	// SetOnF8 before Run so the RegisterHotKey message-loop thread observes
	// the callback.
	listener.SetOnF8(func() {
		// onF8 runs on the message-loop thread and must return fast, so do
		// the clipboard work (synthesized Ctrl+C plus reads) on a goroutine.
		go func() {
			text, ok, err := inject.CopySelection()
			switch {
			case err != nil:
				fmt.Fprintf(os.Stderr, "copy error: %v\n", err)
			case !ok:
				fmt.Fprintln(os.Stderr, "no selection")
			default:
				fmt.Fprintf(os.Stderr, "selected: %q\n", text)
			}
		}()
	})

	done := make(chan error, 1)
	go func() {
		done <- listener.Run()
	}()

	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, os.Interrupt)

	fmt.Fprintln(os.Stderr, "select text, then tap F8 to capture it. Ctrl+C to quit.")

	select {
	case <-sigs:
		listener.Stop()
		<-done
	case err := <-done:
		if err != nil {
			fmt.Fprintf(os.Stderr, "listener error: %v\n", err)
			os.Exit(1)
		}
	}
}
