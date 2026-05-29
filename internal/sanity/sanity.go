// Package sanity guards against degenerate transcription output before it
// reaches the foreground window.
//
// KB-Whisper (like all Whisper models) can fall into a repetition loop on
// long, context-free digit strings — a dictated phone number, personal
// number, or measurement — emitting hundreds of repeated tokens such as
// "O A O A O A ... O O O O". Injecting that into a patient journal is a
// real hazard, not merely noise.
//
// Detection uses the gzip compression ratio, the same signal Whisper's own
// decoding pipeline relies on (its compression_ratio_threshold defaults to
// 2.4): repetitive text compresses far better than natural language, so a
// high ratio flags a loop.
package sanity

import (
	"bytes"
	"compress/gzip"
)

const (
	// minLength is the byte-length floor below which the gzip ratio is
	// meaningless: gzip's fixed header/footer overhead dominates short
	// input and can even push the ratio below 1.
	minLength = 60

	// maxRatio mirrors Whisper's compression_ratio_threshold. Text that
	// compresses better than this is treated as a repetition loop.
	maxRatio = 2.4
)

// Ratio returns the gzip compression ratio of s: the original length
// divided by the gzip-compressed length. Repetitive text yields a high
// ratio; natural language stays low. The empty string returns 0.
//
// gzip.Close must run before the compressed length is read so the footer
// (CRC + size) is flushed into the buffer.
func Ratio(s string) float64 {
	if len(s) == 0 {
		return 0
	}
	var buf bytes.Buffer
	w := gzip.NewWriter(&buf)
	if _, err := w.Write([]byte(s)); err != nil {
		// Writing to a bytes.Buffer cannot actually fail; on any
		// phantom error treat the text as non-degenerate so we never
		// discard a real dictation.
		return 0
	}
	if err := w.Close(); err != nil {
		return 0
	}
	return float64(len(s)) / float64(buf.Len())
}

// IsDegenerate reports whether s looks like a Whisper repetition loop:
// long enough for the gzip ratio to be meaningful (len > minLength) and
// compressing better than maxRatio.
func IsDegenerate(s string) bool {
	return len(s) > minLength && Ratio(s) > maxRatio
}
