// Command f9-test verifies F9 detection and selection capture in
// isolation (no popup, no dictionary, no inject-back). On each F9 tap it
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

	// SetOnF9 before Run so the hook thread observes the callback.
	listener.SetOnF9(func() {
		// onF9 runs on the hook thread and must return fast, so do the
		// clipboard work (synthesized Ctrl+C plus reads) on a goroutine.
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

	fmt.Fprintln(os.Stderr, "select text, then tap F9 to capture it. Ctrl+C to quit.")

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
