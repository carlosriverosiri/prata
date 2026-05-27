// Command record-test verifies Phase 2 (audio capture) end-to-end by
// recording from the default microphone for a fixed duration, encoding
// the PCM as WAV, sending it to Berget, and printing the transcription.
//
// Usage:
//
//	$env:BERGET_API_KEY = "..."
//	record-test [seconds]   (defaults to 5 seconds)
package main

import (
	"bytes"
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/carlosriveros/prata/internal/audio"
	"github.com/carlosriveros/prata/internal/transcribe"
)

func main() {
	seconds := 5
	if len(os.Args) >= 2 {
		n, err := strconv.Atoi(os.Args[1])
		if err != nil || n <= 0 {
			fmt.Fprintln(os.Stderr, "usage: record-test [seconds]")
			os.Exit(2)
		}
		seconds = n
	}

	apiKey := os.Getenv("BERGET_API_KEY")
	if apiKey == "" {
		fmt.Fprintln(os.Stderr, "BERGET_API_KEY not set")
		os.Exit(1)
	}

	fmt.Fprintf(os.Stderr, "recording for %d seconds...\n", seconds)

	session, err := audio.Start()
	if err != nil {
		fmt.Fprintf(os.Stderr, "audio start: %v\n", err)
		os.Exit(1)
	}

	time.Sleep(time.Duration(seconds) * time.Second)

	pcm, err := session.Stop()
	if err != nil {
		fmt.Fprintf(os.Stderr, "audio stop: %v\n", err)
		os.Exit(1)
	}

	fmt.Fprintf(os.Stderr, "captured %d bytes of PCM\n", len(pcm))

	wav := transcribe.EncodePCM(pcm)
	fmt.Fprintf(os.Stderr, "encoded to %d-byte WAV; sending to Berget...\n", len(wav))

	client := transcribe.NewClient(apiKey)
	text, err := client.Transcribe(bytes.NewReader(wav))
	if err != nil {
		fmt.Fprintf(os.Stderr, "transcribe: %v\n", err)
		os.Exit(1)
	}

	fmt.Println(text)
}
