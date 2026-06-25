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
//
// Two complementary signals (analysis 2026-06-25, PRATA-REVIEW §15 #7):
//
//   - The gzip ratio targets HIGH-repetition token loops, the real KB-Whisper
//     failure mode (ratios 8–12, far above the threshold). Legitimate but
//     repetitive clinical dictation — "ingen X, ingen Y, ...", bilateral
//     findings, "utan anmärkning" lists — tops out around 1.8, so there is wide
//     margin and no false positives; the threshold is deliberately NOT lowered,
//     because doing so would start discarding that legitimate text.
//   - looksRepeated catches LOW-repetition phrase loops the ratio misses (a
//     sentence repeated ~4x compresses to only ~1.9). It requires a multi-word
//     phrase repeated back-to-back, which legitimate repetition never does.
//
// Remaining (accepted) gaps: a phrase repeated only 2–3x, and short single-word
// runs, are left alone — they are ambiguous with legitimate speech or a spoken
// read-back, they are short and visible to the user, and there is no execution
// fallback to undo, so they are accepted rather than risk a false positive that
// drops a real dictation.
package sanity

import (
	"bytes"
	"compress/gzip"
	"strings"
)

const (
	// minLength is the byte-length floor below which the gzip ratio is
	// meaningless: gzip's fixed header/footer overhead dominates short
	// input and can even push the ratio below 1.
	minLength = 60

	// maxRatio mirrors Whisper's compression_ratio_threshold. Text that
	// compresses better than this is treated as a repetition loop.
	maxRatio = 2.4

	// minPhraseWords / minPhraseReps drive looksRepeated, the complementary
	// guard for LOW-repetition phrase loops the gzip ratio misses. A unit of at
	// least minPhraseWords words must repeat back-to-back at least minPhraseReps
	// times. Four consecutive identical 2+-word phrases do not occur in real
	// clinical dictation, so this never drops legitimate text; a phrase repeated
	// only 2–3x is left alone (ambiguous with a spoken read-back), and
	// single-word loops stay the gzip path's job.
	minPhraseWords = 2
	minPhraseReps  = 4
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

// IsDegenerate reports whether s looks like a Whisper repetition loop, by
// either of two complementary signals: a high gzip ratio (long enough to be
// meaningful, len > minLength, and compressing better than maxRatio — catches
// HIGH-repetition token loops), or a multi-word phrase repeated back-to-back
// (looksRepeated — catches LOW-repetition phrase loops the ratio misses).
func IsDegenerate(s string) bool {
	return looksRepeated(s) || (len(s) > minLength && Ratio(s) > maxRatio)
}

// looksRepeated reports whether s contains a phrase of at least minPhraseWords
// words repeated back-to-back at least minPhraseReps times — e.g. a sentence
// emitted 4x, whose gzip ratio (~1.9) stays under maxRatio. It compares word
// windows anywhere in the text (so a loop at the end, after real dictation, is
// caught too), and the whole window must match, including attached punctuation.
// Legitimate repetition that only repeats a word across varied content ("ingen
// X, ingen Y, ...") never matches, because the words after the repeated one
// differ. The scan is O(n^2) in the word count but runs once per finished
// transcription, never in a hot path.
func looksRepeated(s string) bool {
	words := strings.Fields(s)
	n := len(words)
	for unit := minPhraseWords; unit <= n/minPhraseReps; unit++ {
		for i := 0; i+unit*minPhraseReps <= n; i++ {
			reps := 1
			for off := i + unit; off+unit <= n; off += unit {
				if !wordWindowEqual(words, i, off, unit) {
					break
				}
				reps++
			}
			if reps >= minPhraseReps {
				return true
			}
		}
	}
	return false
}

// wordWindowEqual reports whether the unit-length word windows starting at a
// and b are identical.
func wordWindowEqual(words []string, a, b, unit int) bool {
	for k := 0; k < unit; k++ {
		if words[a+k] != words[b+k] {
			return false
		}
	}
	return true
}
