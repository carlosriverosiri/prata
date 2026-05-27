// Command hotkey-test verifies the WH_KEYBOARD_LL hook in isolation
// (no audio, no Berget). It prints PRESS when Ctrl+Win is first held
// down and RELEASE when either key is released. Exits on Ctrl+C in
// the terminal.
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

	fmt.Fprintln(os.Stderr, "hold Ctrl+Win to test. Ctrl+C to quit.")

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
