# Changelog

All notable changes to Prata are documented in this file.
Format follows [Keep a Changelog](https://keepachangelog.com/);
versions will be tagged once Phase 7 produces installable releases.

## Phase 1 — 2026-05-27

### Added

- `internal/transcribe/client.go` — HTTP client against Berget AI's
  `/v1/audio/transcriptions` endpoint. Uses Go's standard library only
  (`net/http`, `mime/multipart`, `encoding/json`). Bearer authentication,
  30-second timeout, hardcoded to `KBLab/kb-whisper-large` and Swedish.
- `internal/transcribe/wav.go` — PCM_S16LE → WAV (RIFF) encoder with a
  spec-minimum 44-byte header. Exposes `EncodePCM([]byte) []byte` and the
  audio-format constants `SampleRate`, `NumChannels`, `BitsPerSample`
  that will be the contract for Phase 2 audio capture.
- `cmd/transcribe-test/` — smoke-test CLI: WAV file → Berget → printed text.
- `cmd/wav-roundtrip-test/` — integration test for `EncodePCM`: extracts
  PCM from a known-good WAV, re-encodes with our encoder, sends to Berget,
  verifies the transcription matches the reference.
- `.gitignore` — excludes Windows binaries, Go test artifacts, IDE files,
  and personal voice fixtures.

### Verified

- End-to-end transcription against Berget AI works from Go.
- Mean latency 2.85s, spread 0.36s over 5 sequential calls on 19.5s audio.
- No cold-start effect; Run 1 (2.96s) falls within the spread of Runs 2–5.
- Whisper error pattern matches the local Diktell installation exactly,
  confirming `dictionary-corrections.txt` is directly reusable in Phase 5.
