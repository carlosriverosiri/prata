// Command tray-test verifies the notification-area icon in isolation (no
// audio, no Berget). It shows the Prata tray icon; right-click it and choose
// "Avsluta" to quit, or press Ctrl+C in the terminal.
package main

import (
	"fmt"
	"os"
	"os/signal"

	"github.com/carlosriveros/prata/internal/icon"
	"github.com/carlosriveros/prata/internal/tray"
)

func main() {
	// Before any window or HICON: makes the tray icon crisp on scaled
	// displays. cmd/prata should call this too.
	tray.SetProcessDPIAware()

	t := tray.New(icon.ICO, "Prata", func() {
		fmt.Fprintln(os.Stderr, "Avsluta clicked")
	})

	done := make(chan error, 1)
	go func() {
		done <- t.Run()
	}()

	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, os.Interrupt)

	fmt.Fprintln(os.Stderr, "tray running. Right-click the icon and choose Avsluta, or Ctrl+C to quit.")

	select {
	case <-sigs:
		t.Stop()
		<-done
	case err := <-done:
		if err != nil {
			fmt.Fprintf(os.Stderr, "tray error: %v\n", err)
			os.Exit(1)
		}
	}
}
