package cue

import (
	"bytes"
	"encoding/binary"
	"testing"
)

// expectedSamples is the per-tone sample count derived the same way the
// generator computes it, so the test tracks the constants automatically.
const expectedSamples = sampleRate * toneMs / 1000

func TestMakeToneWAVHeader(t *testing.T) {
	wav := makeToneWAV(880)

	dataLen := expectedSamples * 2
	if len(wav) != 44+dataLen {
		t.Fatalf("len(wav) = %d, want %d", len(wav), 44+dataLen)
	}

	if got := string(wav[0:4]); got != "RIFF" {
		t.Errorf("RIFF magic = %q, want %q", got, "RIFF")
	}
	if got := string(wav[8:12]); got != "WAVE" {
		t.Errorf("WAVE magic = %q, want %q", got, "WAVE")
	}
	if got := string(wav[12:16]); got != "fmt " {
		t.Errorf("fmt magic = %q, want %q", got, "fmt ")
	}
	if got := string(wav[36:40]); got != "data" {
		t.Errorf("data magic = %q, want %q", got, "data")
	}
	if got := binary.LittleEndian.Uint32(wav[24:28]); got != sampleRate {
		t.Errorf("sample rate = %d, want %d", got, sampleRate)
	}
	if got := binary.LittleEndian.Uint32(wav[40:44]); got != uint32(dataLen) {
		t.Errorf("data size = %d, want %d", got, dataLen)
	}
}

func TestMakeToneWAVAmplitudeIsCapped(t *testing.T) {
	wav := makeToneWAV(880)
	// Peak sample must respect the gentle amplitude ceiling. Allow +1
	// for int16 truncation rounding. Computed via a float variable so
	// the (non-integer) constant is converted at runtime.
	ceiling := float64(amplitude) * 32767
	maxAllowed := int16(ceiling) + 1

	samples := wav[44:]
	for i := 0; i+1 < len(samples); i += 2 {
		s := int16(binary.LittleEndian.Uint16(samples[i : i+2]))
		if s < 0 {
			s = -s
		}
		if s > maxAllowed {
			t.Fatalf("sample %d = %d exceeds amplitude cap %d", i/2, s, maxAllowed)
		}
	}
}

func TestMakeToneWAVFadesInFromZero(t *testing.T) {
	wav := makeToneWAV(880)
	first := int16(binary.LittleEndian.Uint16(wav[44:46]))
	if first != 0 {
		t.Errorf("first sample = %d, want 0 (fade-in starts at silence)", first)
	}
}

func TestStartAndStopTonesDiffer(t *testing.T) {
	if bytes.Equal(makeToneWAV(880), makeToneWAV(587)) {
		t.Error("start and stop tones are identical; they must be distinguishable")
	}
}
