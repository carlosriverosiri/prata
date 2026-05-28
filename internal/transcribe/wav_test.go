package transcribe

import (
	"bytes"
	"encoding/binary"
	"testing"
)

func TestEncodePCMHeader(t *testing.T) {
	pcm := []byte{0x01, 0x02, 0x03, 0x04, 0x05, 0x06}
	wav := EncodePCM(pcm)

	if len(wav) != 44+len(pcm) {
		t.Fatalf("len(wav) = %d, want %d", len(wav), 44+len(pcm))
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

	if got := binary.LittleEndian.Uint32(wav[4:8]); got != uint32(36+len(pcm)) {
		t.Errorf("RIFF chunk size = %d, want %d", got, 36+len(pcm))
	}
	if got := binary.LittleEndian.Uint32(wav[16:20]); got != 16 {
		t.Errorf("fmt sub-chunk size = %d, want 16", got)
	}
	if got := binary.LittleEndian.Uint16(wav[20:22]); got != 1 {
		t.Errorf("audio format = %d, want 1 (PCM)", got)
	}
	if got := binary.LittleEndian.Uint16(wav[22:24]); got != NumChannels {
		t.Errorf("num channels = %d, want %d", got, NumChannels)
	}
	if got := binary.LittleEndian.Uint32(wav[24:28]); got != SampleRate {
		t.Errorf("sample rate = %d, want %d", got, SampleRate)
	}
	wantByteRate := uint32(SampleRate * NumChannels * BitsPerSample / 8)
	if got := binary.LittleEndian.Uint32(wav[28:32]); got != wantByteRate {
		t.Errorf("byte rate = %d, want %d", got, wantByteRate)
	}
	wantBlockAlign := uint16(NumChannels * BitsPerSample / 8)
	if got := binary.LittleEndian.Uint16(wav[32:34]); got != wantBlockAlign {
		t.Errorf("block align = %d, want %d", got, wantBlockAlign)
	}
	if got := binary.LittleEndian.Uint16(wav[34:36]); got != BitsPerSample {
		t.Errorf("bits per sample = %d, want %d", got, BitsPerSample)
	}
	if got := binary.LittleEndian.Uint32(wav[40:44]); got != uint32(len(pcm)) {
		t.Errorf("data sub-chunk size = %d, want %d", got, len(pcm))
	}

	if !bytes.Equal(wav[44:], pcm) {
		t.Errorf("PCM payload = %v, want %v", wav[44:], pcm)
	}
}

func TestEncodePCMEmpty(t *testing.T) {
	wav := EncodePCM(nil)
	if len(wav) != 44 {
		t.Fatalf("len(wav) = %d, want 44 (header only)", len(wav))
	}
	if got := binary.LittleEndian.Uint32(wav[40:44]); got != 0 {
		t.Errorf("data size = %d, want 0", got)
	}
	if got := binary.LittleEndian.Uint32(wav[4:8]); got != 36 {
		t.Errorf("RIFF chunk size = %d, want 36", got)
	}
}
