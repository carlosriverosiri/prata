// Command transcribe-test sends a local WAV file to Berget AI and prints
// the transcription. Used to verify Phase 1 (HTTP client + Berget
// integration) in isolation, before audio capture (Phase 2) and hotkey
// handling (Phase 3) layers exist.
//
// Usage:
//
//	$env:BERGET_API_KEY = "..."
//	transcribe-test path\to\audio.wav
package main

import (
	"fmt"
	"os"

	"github.com/carlosriveros/prata/internal/transcribe"
)

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintln(os.Stderr, "usage: transcribe-test <path-to-wav>")
		os.Exit(2)
	}

	apiKey := os.Getenv("BERGET_API_KEY")
	if apiKey == "" {
		fmt.Fprintln(os.Stderr, "BERGET_API_KEY not set")
		os.Exit(1)
	}

	f, err := os.Open(os.Args[1])
	if err != nil {
		fmt.Fprintf(os.Stderr, "open: %v\n", err)
		os.Exit(1)
	}
	defer f.Close()

	client := transcribe.NewClient(apiKey)
	text, err := client.Transcribe(f)
	if err != nil {
		fmt.Fprintf(os.Stderr, "transcribe: %v\n", err)
		os.Exit(1)
	}

	fmt.Println(text)
}
