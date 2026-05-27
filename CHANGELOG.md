# Changelog

All notable changes to Prata are documented in this file.
Format follows [Keep a Changelog](https://keepachangelog.com/);
versions will be tagged once Phase 7 produces installable releases.

## Phase 5 — 2026-05-27

### Added

- `internal/dict/dict.go` — word-boundary text replacement applied
  to transcribed text before injection. Loads rules from a key=value
  file (lines starting with `#` are comments, blank lines ignored);
  each rule compiles to a `\bkey\b` regex applied case-sensitively.
  Pure Go, stdlib only.
- `dictionary-corrections.txt` — copied verbatim from the Diktell
  project (same KB-Whisper-Large model produces the same error
  patterns) plus one new rule `adoption = abduktion` confirmed in
  Phase 4 testing.
- `cmd/ptt-test/` (modified) — loads the dictionary on startup from
  `PRATA_DICT_PATH` (env var), falling back to
  `dictionary-corrections.txt` next to the executable. Applies all
  rules to every transcription between Berget's response and
  `inject.Type`. If the file is missing or malformed, the app logs
  a warning and continues without corrections.

### Verified

- "abduktion" survives end-to-end despite Whisper's consistent
  transcription of the word as "adoption": the new rule catches it
  before injection. Notepad output contains "abduktion".
- Startup log shows `dictionary loaded` when the file is found and
  parsed successfully.

### Known limitation

- Word-boundary matching uses Go's `\b`, which treats å/ä/ö as
  non-word characters. Rules whose key starts or ends with å/ä/ö
  may not match correctly. None of the current rules are affected;
  this can be revisited in a follow-up if it ever bites.

## Phase 4 — 2026-05-27

### Added

- `internal/inject/inject.go` — Unicode text injection into the
  foreground window via Win32 `SendInput` with `KEYEVENTF_UNICODE`.
  Direct P/Invoke via `syscall`; stdlib only, no cgo. Each UTF-16
  code unit produces a key-down + key-up event; characters outside
  the BMP are emitted as surrogate pairs via `unicode/utf16.Encode`.
- `cmd/inject-test/` — isolated verification of the inject package.
  Types a supplied text argument into whichever window has focus
  3 seconds after launch.
- `cmd/ptt-test/` (modified) — now injects the transcribed text into
  the foreground window via `inject.Type`, instead of printing to
  stdout. All status messages remain on stderr.

### Verified

- `å`, `ä`, `ö` and other non-ASCII characters injected correctly,
  confirming UTF-16 + KEYEVENTF_UNICODE works end-to-end.
- Full PTT cycle works in real applications: Ctrl+Win → speak →
  release → text appears in the active window (Notepad tested).
- Multiple consecutive dictations behave independently — no session
  leakage, no state drift between cycles.

### Known interaction

- Prata and Diktell share the Ctrl+Win hotkey. Running both
  concurrently produces duplicate text in the active window: both
  apps capture the same audio in parallel and inject independently
  (with slight Whisper variation between local CUDA and Berget).
  The intended deployment is one-or-the-other per machine
  (Diktell on GPU machines, Prata elsewhere), so this is by design,
  but it is worth documenting.

## Phase 3 — 2026-05-27

### Added

- `internal/hotkey/listener.go` — global Win32 `WH_KEYBOARD_LL`
  keyboard hook for detecting the Ctrl+Win combination. Uses direct
  P/Invoke via Go's `syscall` package; stdlib only, no cgo.
  `Listener.Run()` pins itself to its OS thread (`runtime.LockOSThread`)
  and runs the Windows message loop; `Stop()` posts `WM_QUIT` to that
  thread. Press/release callbacks fire on the hook thread and must
  return within 300 ms (Windows' `LowLevelHooksTimeout`).
- `cmd/hotkey-test/` — isolated verification of the hook (no audio, no
  Berget). Prints `PRESS` / `RELEASE` to stdout.
- `cmd/ptt-test/` — wires hotkey + audio + transcribe into a full
  push-to-talk loop. Hook callbacks enqueue events on a buffered
  channel; a separate processor goroutine owns the `audio.Session`
  lifecycle and dispatches to Berget on release.

### Verified

- Hook detects Ctrl+Win press and release across multiple cycles with
  no state drift. Modifier-state machine handles arbitrary ordering of
  ctrl/win down/up events correctly.
- Full PTT loop: 5.86s recording transcribed in 2.37s end-to-end
  (press → speech → release → text), in line with the Phase 1 latency
  baseline.
- The familiar "adoption" → "abduktion" Whisper error reproduced,
  confirming again that Phase 5 dictionary corrections will be the
  right place to address it.

## Phase 2 — 2026-05-27

### Added

- `internal/audio/capture.go` — WASAPI audio capture via malgo
  (Go binding for miniaudio). Session-based API: `Start()` returns a
  `*Session`, `Stop()` returns the recorded PCM bytes. Captures at
  16 kHz mono PCM_S16LE; imports the format constants from
  `internal/transcribe` to make the contract between capture and
  encoder explicit.
- `cmd/record-test/` — smoke-test CLI that records N seconds (default
  5) from the default microphone, encodes to WAV via `transcribe.EncodePCM`,
  sends to Berget, and prints the transcription.
- `github.com/gen2brain/malgo v0.11.25` — first external dependency
  (cgo). Requires a C toolchain at build time; TDM-GCC 10.3.0 used on
  the development machine.

### Verified

- 5-second recording captured 159 680 bytes of PCM = 4.99 seconds at
  16 kHz mono 16-bit (99.8% of the requested duration; the 10 ms
  deficit is malgo's startup latency, expected).
- Berget transcribed the recording correctly, confirming the format
  contract between audio capture and WAV encoding is intact end-to-end
  (sample rate, byte order, channel count, bit depth all correct).
- First cgo build took ~14 seconds; subsequent builds use Go's build
  cache and are faster.

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
