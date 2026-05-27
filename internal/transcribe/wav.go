// Package transcribe — wav.go provides WAV (RIFF) container encoding
// for the raw PCM samples captured in Phase 2. See client.go for the
// package overview.
package transcribe

import "encoding/binary"

// Audio format constants for Prata. Phase 2 (audio capture via malgo)
// must record in this exact format; KB-Whisper-Large expects it.
// Defined here, in the file that encodes the format, so they live
// next to their assumptions.
const (
	SampleRate    = 16000 // Hz
	NumChannels   = 1     // mono
	BitsPerSample = 16    // signed little-endian
)

// EncodePCM wraps raw 16-bit signed little-endian mono PCM samples at
// 16 kHz in a WAV (RIFF) container suitable for sending to Berget AI's
// transcription endpoint.
//
// The input must already be in the expected format: PCM_S16LE, mono,
// 16 kHz. EncodePCM performs no resampling, channel conversion, or
// bit-depth adjustment — it only prepends a standard 44-byte RIFF/WAVE
// header to the PCM data.
func EncodePCM(pcm []byte) []byte {
	const headerSize = 44

	buf := make([]byte, headerSize+len(pcm))

	// RIFF chunk descriptor
	copy(buf[0:4], "RIFF")
	binary.LittleEndian.PutUint32(buf[4:8], uint32(36+len(pcm))) // file size minus 8
	copy(buf[8:12], "WAVE")

	// fmt sub-chunk (PCM = always 16 bytes long)
	copy(buf[12:16], "fmt ")
	binary.LittleEndian.PutUint32(buf[16:20], 16)                                     // sub-chunk size
	binary.LittleEndian.PutUint16(buf[20:22], 1)                                      // audio format (1 = PCM)
	binary.LittleEndian.PutUint16(buf[22:24], NumChannels)                            // num channels
	binary.LittleEndian.PutUint32(buf[24:28], SampleRate)                             // sample rate
	binary.LittleEndian.PutUint32(buf[28:32], SampleRate*NumChannels*BitsPerSample/8) // byte rate
	binary.LittleEndian.PutUint16(buf[32:34], NumChannels*BitsPerSample/8)            // block align
	binary.LittleEndian.PutUint16(buf[34:36], BitsPerSample)                          // bits per sample

	// data sub-chunk
	copy(buf[36:40], "data")
	binary.LittleEndian.PutUint32(buf[40:44], uint32(len(pcm)))
	copy(buf[44:], pcm)

	return buf
}
