# Changelog

All notable changes to Prata are documented in this file.
Format follows [Keep a Changelog](https://keepachangelog.com/).
Development is organised in numbered phases; the phase entries below
record that history. Tagged releases bundle the phases completed up to
that point.

## v0.1.1 — 2026-05-29

Robustness and safety release. Adds a degenerate-output guard that
discards KB-Whisper repetition loops before they reach the foreground
window (a real hazard on dictated number strings in a clinical
journal), skips empty / near-empty captures and empty transcriptions,
lowers the audio-cue volume, and adds the sanity-test calibration CLI.

### Added

- `internal/sanity/sanity.go` — a guard against degenerate
  (repetition-loop) transcriptions. KB-Whisper can fall into a loop on
  long, context-free digit strings (a dictated phone number, personal
  number, or measurement), emitting hundreds of repeated tokens such as
  "O A O A O A ...". The detector uses the gzip compression ratio — the
  same signal Whisper's own pipeline uses (its
  `compression_ratio_threshold` defaults to 2.4) — since repetitive text
  compresses far better than natural language. `Ratio` returns
  original/compressed length; `IsDegenerate` flags text longer than 60
  bytes whose ratio exceeds 2.4 (the length floor avoids false positives
  on short text, where gzip's fixed overhead makes the ratio
  meaningless). Stdlib only.
- `cmd/prata/main.go` — wires the guard into `processEvents`, after the
  empty-transcription check and before injection. A degenerate result is
  discarded rather than typed into the foreground window — a real
  patient-safety hazard in a clinical journal, not just noise. The
  discard logs the gzip ratio and a rune-safe prefix of the dropped text
  so the user sees what was lost and can re-dictate.
- `cmd/sanity-test` — dev-only calibration CLI for the gzip-ratio
  threshold. Prints a formatted table of gzip ratios and IsDegenerate
  verdicts for a fixed set of built-in example strings (natural Swedish
  sentences, spoken digit sequences, personnummer, and synthetic
  repetition loops), so the 2.4 threshold can be eyeballed against
  representative dictations. Run with `go run ./cmd/sanity-test/`.

### Changed

- `internal/cue/cue.go` — lowered the audio cue amplitude from 0.18 to
  0.07 of full scale, so the start/stop tones are quieter and less
  obtrusive.

### Fixed

- `cmd/prata/main.go` — guard against empty / near-empty captures. An
  accidental brief Ctrl+Win tap could capture little or no audio, yet
  the empty WAV was still sent to Berget and blocked for the full 30s
  HTTP timeout before failing with "context deadline exceeded". The
  release handler now skips transcription when the captured PCM is
  below a minimal threshold (`minCaptureBytes`, ~0.1s of audio derived
  from the transcribe format constants), logging "no audio captured,
  skipping" and continuing to the next event.
- `cmd/prata/main.go` — skip injection on empty transcription. When
  Berget returned empty or whitespace-only text (e.g. a very short
  capture with no clear speech), the release handler still appended a
  newline and injected a bare blank line into the foreground window.
  It now checks the trimmed result after dict correction and, when
  empty, logs "empty transcription, skipping" with the elapsed
  round-trip time and continues to the next event.

## v0.1.0 — 2026-05-28

First installable release. Bundles Phases 1–8: Berget transcription,
WASAPI capture, Ctrl+Win push-to-talk, clipboard-paste injection,
correction dictionary, DPAPI-encrypted API key, single-instance guard,
PowerShell installer with autostart, and gentle audio cues. Published
via the tag-triggered GitHub release workflow.

## Phase 8 — 2026-05-28

### Added

- `internal/cue/cue.go` — short, gentle audio cues for push-to-talk
  state changes. Tones are synthesised in-process as 16 kHz mono PCM,
  wrapped in a WAV header, and played from memory via winmm
  `PlaySoundW` with `SND_MEMORY|SND_ASYNC|SND_NODEFAULT`. Async
  playback never blocks the caller and no sound files ship with the
  app. Two distinguishable tones: 880 Hz on start, 587 Hz on stop.
  Each tone is 110 ms with a 12 ms fade in/out to avoid clicks.
  Direct P/Invoke against `winmm.dll`; stdlib only, no cgo.
- Amplitude is capped at 0.18 of full scale (lowered from an initial
  0.35) so the cues stay unobtrusive at any system volume.
- `cmd/prata/main.go` (modified) — calls `cue.PlayStart()` right after
  `audio.Start()` succeeds, and `cue.PlayStop()` right after
  `session.Stop()` returns. The stop cue is deliberately played
  *after* the microphone is closed so the tone cannot leak into the
  captured PCM (and thus into the transcription).

### Verified

- `gofmt -w`, `go build ./...`, and `go vet ./...` all clean.

### To confirm on device

- Actual audible playback and perceived loudness at 0.18 amplitude
  cannot be verified in CI/headless; confirm the start/stop tones are
  audible but unobtrusive during a real dictation cycle.

## Phase 7 — 2026-05-28

### Added

- `install.ps1` (repo root) — PowerShell installer that copies the
  binaries to `%LOCALAPPDATA%\Prata`, encrypts the API key via
  `prata-setkey`, and registers a Task Scheduler entry for autostart
  at login. Supports `-Local` for building from the working tree
  (development) or default GitHub-release download (end users).
- `.github/workflows/release.yml` — tag-triggered Windows pipeline
  (`v*`) that builds `prata.exe` (with `-H windowsgui`) and
  `prata-setkey.exe`, then publishes them along with
  `dictionary-corrections.txt` and `install.ps1` via
  `softprops/action-gh-release@v2`.
- `internal/inject/inject.go` (rewritten) — text injection now uses
  the Windows clipboard (`CF_UNICODETEXT` via `OpenClipboard`,
  `GlobalAlloc`, `SetClipboardData`) plus a `Ctrl+V` chord sent with
  `SendInput`. Previous `KEYEVENTF_UNICODE` path was unreliable in
  Chromium/Electron apps (Claude Desktop) and modern Notepad: dropped
  key-up events caused the OS to autorepeat the last character, e.g.
  `"Detta ar ett test utan radbrytning"` →
  `"Detta        ggggggggggggggggggggg"`. Per-rune batching and
  inter-event delays helped but not consistently. Clipboard paste
  goes through the target app's standard paste handler and bypasses
  the keyboard input queue entirely.
- Clipboard preservation in `internal/inject` — `Type` reads any
  prior `CF_UNICODETEXT` content (`IsClipboardFormatAvailable` +
  `GetClipboardData` + `GlobalSize` + `RtlMoveMemory`) before
  pasting and restores it ~50 ms after the paste settles. If there
  was no prior text, the clipboard is emptied so the dictation does
  not leak into the user's next paste.
- `cmd/prata/main.go` (modified) — appends `\n` to each transcription
  before injection so consecutive dictations land on separate lines.

### Verified

- **Notepad** — `"Detta ar ett test utan radbrytning"` injected three
  times back-to-back produces the literal text three times, no
  autorepeat artifacts.
- **Claude Desktop (Electron)** — same input, same result, three
  times in a row.
- **Newlines** — full PTT cycle dictating two sentences puts each
  sentence on its own line in both Notepad and Claude Desktop.
- **Clipboard preservation** (three scenarios):
  - Empty clipboard before → empty clipboard after.
  - Text clipboard before → exact text restored after.
  - Image (PrintScreen) clipboard before → empty clipboard after
    (image lost, but no dictation text leaked either).

### Known limitation

- Clipboard restore preserves only `CF_UNICODETEXT`. Non-text formats
  (bitmaps, files, rich text from Word, HTML clipboard fragments) are
  destroyed by the dictation paste cycle. Full enumeration via
  `EnumClipboardFormats` and per-format reallocation is possible but
  significantly more complex; deferred until a real-world use case
  demands it.

## Phase 6 — 2026-05-27

### Added

- `internal/auth/dpapi.go` — Windows DPAPI wrapper exposing
  `SaveAPIKey`, `LoadAPIKey`, and `KeyPath`. Direct P/Invoke against
  `crypt32.dll` (`CryptProtectData` / `CryptUnprotectData`) and
  `kernel32.dll` (`LocalFree`). Stdlib only, no cgo. The encrypted
  blob is bound to both the current user and current machine — it
  cannot be decrypted by another user nor copied to another PC.
- `cmd/prata-setkey/` — one-shot CLI that takes the API key from
  `os.Args[1]` (or interactive stdin) and encrypts it to
  `%LOCALAPPDATA%\Prata\apikey.dat`.
- `cmd/ptt-test/` (modified) — falls back to `auth.LoadAPIKey()`
  when `BERGET_API_KEY` env var is empty or unset. Both paths
  remain supported: env var for development, DPAPI for production.

### Verified

- New API key (rotated in this session, replacing one that had
  been exposed in plaintext earlier) encrypted via `prata-setkey`
  and saved to disk. File is 278 bytes for a ~65-character key —
  DPAPI overhead confirms encryption. First byte is 0x01, the
  DPAPI blob version marker, ruling out plaintext storage.
- `ptt-test` runs with `BERGET_API_KEY=""` and successfully
  transcribes via the DPAPI-loaded key.

### Deferred

- Task Scheduler autostart will be handled by `install.ps1` in
  Phase 7. The Go side of Phase 6 (DPAPI) is complete; the
  remaining piece is deployment scripting.

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
