// Package audio captures microphone input via WASAPI (through miniaudio,
// wrapped by malgo) and produces raw PCM samples in the format that
// internal/transcribe expects.
package audio

import (
	"bytes"
	"fmt"
	"sync"

	"github.com/gen2brain/malgo"

	"github.com/carlosriveros/prata/internal/transcribe"
)

// Session represents an active microphone capture.
//
// Use Start to begin capturing and Stop to finalize and retrieve the
// recorded PCM data. A Session is single-use: do not reuse after Stop.
type Session struct {
	ctx    *malgo.AllocatedContext
	device *malgo.Device

	mu  sync.Mutex
	pcm bytes.Buffer
}

// Start initializes the default input device and begins capturing at
// 16 kHz mono PCM_S16LE — the format expected by internal/transcribe.
//
// On error, no resources are leaked.
func Start() (*Session, error) {
	ctx, err := malgo.InitContext(nil, malgo.ContextConfig{}, nil)
	if err != nil {
		return nil, fmt.Errorf("init malgo context: %w", err)
	}

	s := &Session{ctx: ctx}

	deviceConfig := malgo.DefaultDeviceConfig(malgo.Capture)
	deviceConfig.Capture.Format = malgo.FormatS16
	deviceConfig.Capture.Channels = uint32(transcribe.NumChannels)
	deviceConfig.SampleRate = uint32(transcribe.SampleRate)

	callbacks := malgo.DeviceCallbacks{
		Data: func(_, in []byte, _ uint32) {
			s.mu.Lock()
			s.pcm.Write(in)
			s.mu.Unlock()
		},
	}

	device, err := malgo.InitDevice(ctx.Context, deviceConfig, callbacks)
	if err != nil {
		_ = ctx.Uninit()
		ctx.Free()
		return nil, fmt.Errorf("init capture device: %w", err)
	}
	s.device = device

	if err := device.Start(); err != nil {
		device.Uninit()
		_ = ctx.Uninit()
		ctx.Free()
		return nil, fmt.Errorf("start capture: %w", err)
	}

	return s, nil
}

// Stop finalizes the capture and returns the recorded PCM data as a new
// byte slice. The Session must not be used after Stop.
func (s *Session) Stop() ([]byte, error) {
	if err := s.device.Stop(); err != nil {
		return nil, fmt.Errorf("stop capture: %w", err)
	}
	s.device.Uninit()
	if err := s.ctx.Uninit(); err != nil {
		return nil, fmt.Errorf("uninit malgo context: %w", err)
	}
	s.ctx.Free()

	s.mu.Lock()
	defer s.mu.Unlock()

	out := make([]byte, s.pcm.Len())
	copy(out, s.pcm.Bytes())
	return out, nil
}

// Peak returns the largest absolute 16-bit sample value in pcm (S16LE), i.e.
// the loudest instant of the capture. A muted, disconnected, or wrong-device
// microphone yields near-silence (a peak close to zero) even when the user
// spoke; real speech peaks in the thousands. The caller uses this to drop a
// silent capture instead of sending silence to Whisper, which hallucinates
// short phrases ("Tack för att ni tittade") on it. A trailing odd byte (which
// a well-formed S16LE buffer never has) is ignored.
func Peak(pcm []byte) int16 {
	var peak int16
	for i := 0; i+1 < len(pcm); i += 2 {
		s := int16(uint16(pcm[i]) | uint16(pcm[i+1])<<8)
		// -32768 has no positive counterpart; clamp its magnitude to 32767.
		mag := s
		if mag < 0 {
			if mag == -32768 {
				return 32767
			}
			mag = -mag
		}
		if mag > peak {
			peak = mag
		}
	}
	return peak
}
