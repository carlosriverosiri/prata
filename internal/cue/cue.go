// Package cue plays short, gentle audio tones that signal push-to-talk
// state changes (recording start/stop). Tones are generated in-process
// as PCM, wrapped in a WAV header, and played from memory via winmm
// PlaySound (SND_MEMORY|SND_ASYNC), so playback never blocks the caller
// and no sound files are needed. Amplitude is capped well below full
// scale so the cues stay unobtrusive at any system volume.
package cue

import (
	"bytes"
	"encoding/binary"
	"math"
	"syscall"
	"unsafe"
)

const (
	sampleRate = 16000
	// amplitude is the peak sample level as a fraction of full scale.
	// Kept gentle at 0.35, well under the 0.50 ceiling we want. Raise
	// toward 0.50 for louder cues, lower for quieter.
	amplitude = 0.18
	toneMs    = 110 // length of each cue tone
	fadeMs    = 12  // fade in/out to avoid clicks
)

const (
	sndAsync     = 0x0001
	sndNodefault = 0x0002
	sndMemory    = 0x0004
)

var (
	winmm          = syscall.NewLazyDLL("winmm.dll")
	procPlaySoundW = winmm.NewProc("PlaySoundW")

	startWAV []byte // higher tone: recording started
	stopWAV  []byte // lower tone: recording stopped
)

func init() {
	startWAV = makeToneWAV(880) // higher
	stopWAV = makeToneWAV(587)  // lower, so the two are distinguishable
}

// PlayStart plays the "recording started" cue. Non-blocking.
func PlayStart() { play(startWAV) }

// PlayStop plays the "recording stopped" cue. Non-blocking.
func PlayStop() { play(stopWAV) }

func play(wav []byte) {
	if len(wav) == 0 {
		return
	}
	// The package-level buffers live for the whole process, so the
	// async read by PlaySound always sees valid memory.
	procPlaySoundW.Call(
		uintptr(unsafe.Pointer(&wav[0])),
		0,
		uintptr(sndMemory|sndAsync|sndNodefault),
	)
}

func makeToneWAV(freq float64) []byte {
	n := sampleRate * toneMs / 1000
	fade := sampleRate * fadeMs / 1000
	pcm := make([]int16, n)
	for i := 0; i < n; i++ {
		env := 1.0
		if i < fade {
			env = float64(i) / float64(fade)
		} else if i >= n-fade {
			env = float64(n-i) / float64(fade)
		}
		s := math.Sin(2 * math.Pi * freq * float64(i) / float64(sampleRate))
		pcm[i] = int16(s * env * amplitude * 32767)
	}
	return wrapWAV(pcm)
}

func wrapWAV(pcm []int16) []byte {
	const (
		numChannels   = 1
		bitsPerSample = 16
	)
	dataLen := len(pcm) * 2
	byteRate := sampleRate * numChannels * bitsPerSample / 8
	blockAlign := numChannels * bitsPerSample / 8

	buf := &bytes.Buffer{}
	buf.WriteString("RIFF")
	binary.Write(buf, binary.LittleEndian, uint32(36+dataLen))
	buf.WriteString("WAVE")
	buf.WriteString("fmt ")
	binary.Write(buf, binary.LittleEndian, uint32(16))
	binary.Write(buf, binary.LittleEndian, uint16(1)) // PCM
	binary.Write(buf, binary.LittleEndian, uint16(numChannels))
	binary.Write(buf, binary.LittleEndian, uint32(sampleRate))
	binary.Write(buf, binary.LittleEndian, uint32(byteRate))
	binary.Write(buf, binary.LittleEndian, uint16(blockAlign))
	binary.Write(buf, binary.LittleEndian, uint16(bitsPerSample))
	buf.WriteString("data")
	binary.Write(buf, binary.LittleEndian, uint32(dataLen))
	binary.Write(buf, binary.LittleEndian, pcm)
	return buf.Bytes()
}
