package main

import (
	"testing"
	"time"

	"github.com/carlosriveros/prata/internal/transcribe"
)

// TestEffectiveMaxInjectAge pins the staleness window's three regimes: a short
// tap keeps the tight maxInjectAge floor (where a late inject is the real
// mid-sentence hazard), a longer dictation grows the window to a multiple of the
// spoken length (so an 8-10s transcription of a long dictation is no longer
// dropped — the 2026-06-28 symptom), and a very long dictation is still capped
// at injectAgeMax so a stuck result can never inject after the user has moved on.
func TestEffectiveMaxInjectAge(t *testing.T) {
	cases := []struct {
		name  string
		audio time.Duration
		want  time.Duration
	}{
		{"silent tap stays at floor", 0, maxInjectAge},
		{"short dictation stays at floor", 3 * time.Second, maxInjectAge},
		{"floor boundary", maxInjectAge / injectAgeDictationFactor, maxInjectAge},
		{"medium dictation scales", 10 * time.Second, 20 * time.Second},
		{"long dictation is capped", 20 * time.Second, injectAgeMax},
		{"very long dictation is capped", 60 * time.Second, injectAgeMax},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := effectiveMaxInjectAge(c.audio); got != c.want {
				t.Errorf("effectiveMaxInjectAge(%v) = %v, want %v", c.audio, got, c.want)
			}
		})
	}
}

// TestEffectiveMaxInjectAgeNeverBelowFloor guards the invariant the whole guard
// rests on: no spoken length ever yields a window shorter than the floor.
func TestEffectiveMaxInjectAgeNeverBelowFloor(t *testing.T) {
	for d := time.Duration(0); d <= 90*time.Second; d += time.Second {
		if got := effectiveMaxInjectAge(d); got < maxInjectAge {
			t.Fatalf("effectiveMaxInjectAge(%v) = %v, below floor %v", d, got, maxInjectAge)
		}
		if got := effectiveMaxInjectAge(d); got > injectAgeMax {
			t.Fatalf("effectiveMaxInjectAge(%v) = %v, above cap %v", d, got, injectAgeMax)
		}
	}
}

// TestAudioDuration checks the PCM-length-to-duration conversion against the
// transcribe format constants: one second of audio is exactly one byte-rate of
// bytes, and the function never returns negative for a short/empty buffer.
func TestAudioDuration(t *testing.T) {
	bytesPerSec := transcribe.SampleRate * transcribe.NumChannels * transcribe.BitsPerSample / 8

	if got := audioDuration(bytesPerSec); got != time.Second {
		t.Errorf("audioDuration(1s worth) = %v, want 1s", got)
	}
	if got := audioDuration(bytesPerSec / 2); got != 500*time.Millisecond {
		t.Errorf("audioDuration(0.5s worth) = %v, want 500ms", got)
	}
	if got := audioDuration(0); got != 0 {
		t.Errorf("audioDuration(0) = %v, want 0", got)
	}
}
