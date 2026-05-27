// Command wav-roundtrip-test verifies that internal/transcribe.EncodePCM
// produces a WAV that Berget AI accepts.
//
// It reads a known-good WAV file (typically the ffmpeg-generated
// prata-tests.wav), extracts the raw PCM data from the "data" chunk,
// re-encodes it with EncodePCM, and sends the re-encoded WAV to Berget.
//
// If the transcription matches what we got from the original file, the
// encoder produces a structurally-valid, server-acceptable WAV.
package main

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"os"

	"github.com/carlosriveros/prata/internal/transcribe"
)

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintln(os.Stderr, "usage: wav-roundtrip-test <path-to-wav>")
		os.Exit(2)
	}

	apiKey := os.Getenv("BERGET_API_KEY")
	if apiKey == "" {
		fmt.Fprintln(os.Stderr, "BERGET_API_KEY not set")
		os.Exit(1)
	}

	raw, err := os.ReadFile(os.Args[1])
	if err != nil {
		fmt.Fprintf(os.Stderr, "read: %v\n", err)
		os.Exit(1)
	}

	pcm, err := extractPCM(raw)
	if err != nil {
		fmt.Fprintf(os.Stderr, "extract PCM: %v\n", err)
		os.Exit(1)
	}
	fmt.Fprintf(os.Stderr, "extracted %d bytes of PCM from %d-byte WAV\n", len(pcm), len(raw))

	reencoded := transcribe.EncodePCM(pcm)
	fmt.Fprintf(os.Stderr, "re-encoded to %d bytes (header overhead: %d bytes)\n",
		len(reencoded), len(reencoded)-len(pcm))

	client := transcribe.NewClient(apiKey)
	text, err := client.Transcribe(bytes.NewReader(reencoded))
	if err != nil {
		fmt.Fprintf(os.Stderr, "transcribe: %v\n", err)
		os.Exit(1)
	}

	fmt.Println(text)
}

// extractPCM finds the "data" chunk in a RIFF/WAVE byte slice and returns
// its contents. Minimal parser — only validates RIFF/WAVE magic and
// locates the data chunk; ignores fmt chunk contents and any LIST/INFO
// chunks that tools like ffmpeg may insert.
func extractPCM(wav []byte) ([]byte, error) {
	if len(wav) < 12 ||
		!bytes.Equal(wav[0:4], []byte("RIFF")) ||
		!bytes.Equal(wav[8:12], []byte("WAVE")) {
		return nil, fmt.Errorf("not a RIFF/WAVE file")
	}

	for i := 12; i+8 <= len(wav); {
		chunkID := wav[i : i+4]
		chunkSize := int(binary.LittleEndian.Uint32(wav[i+4 : i+8]))

		if bytes.Equal(chunkID, []byte("data")) {
			start := i + 8
			end := start + chunkSize
			if end > len(wav) {
				end = len(wav) // tolerate size mismatch
			}
			return wav[start:end], nil
		}

		// Chunks pad to even byte boundaries
		i += 8 + chunkSize
		if chunkSize%2 != 0 {
			i++
		}
	}
	return nil, fmt.Errorf("data chunk not found")
}
