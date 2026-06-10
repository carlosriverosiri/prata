// Command hotkey-test verifies the RegisterHotKey listener in isolation
// (no audio, no Berget). It prints PRESS when F1 is first pressed and
// RELEASE when it is released. Exits on Ctrl+C in the terminal.
//
// Note: hotkey-test does not call SetOnF9, so F9 is not registered and
// passes through untouched to the foreground app.
package main

import (
	"fmt"
	"os"
	"os/signal"

	"github.com/carlosriveros/prata/internal/hotkey"
)

func main() {
	listener := hotkey.NewListener(
		func() { fmt.Println("PRESS") },
		func() { fmt.Println("RELEASE") },
	)

	done := make(chan error, 1)
	go func() {
		done <- listener.Run()
	}()

	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, os.Interrupt)

	fmt.Fprintln(os.Stderr, "hold F1 to test. Ctrl+C to quit.")

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
