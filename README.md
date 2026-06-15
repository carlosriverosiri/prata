# Prata

Push-to-talk Swedish dictation for Windows. Hold **F1**, speak,
release — your speech is transcribed and typed into whatever window has
focus. Transcription runs on [Berget AI](https://berget.ai) using
KBLab's `kb-whisper-large` model.

Prata is a lightweight background utility: no application window, just a
single system-tray icon you can right-click to quit. It registers a global
hotkey, captures the microphone while the keys are held, sends the audio to
Berget, applies a correction dictionary, and inserts the result.

## Features

- **Push-to-talk** via a global F1 hotkey (`RegisterHotKey`) — works in
  any foreground application.
- **Swedish transcription** through Berget AI (`KBLab/kb-whisper-large`).
- **Gentle audio cues** — a higher tone when recording starts, a lower
  tone when it stops, synthesised in-process (no sound files).
- **Correction dictionary** — word-boundary text replacements fix
  recurring Whisper errors (e.g. `adoption` → `abduktion`).
- **Hybrid text injection** — routed on the foreground window's class.
  Chromium/Electron apps and the web-based journal receive the text via
  SendInput Unicode, leaving the clipboard untouched (so a copied
  screenshot survives and dictated text stays out of Win+V / cloud
  clipboard); every other window uses clipboard paste with the previous
  clipboard content preserved and restored. Anything uncertain defaults to
  clipboard paste.
- **Encrypted API key** at rest via Windows DPAPI (per-user, per-machine).
- **Single-instance guard** — a named mutex prevents two copies from
  double-typing.
- **System-tray icon** — a small red Prata icon in the notification area;
  right-click and choose **Avsluta** to quit. This is the primary way to
  exit when Prata runs at login with no console window.
- **Autostart at login** via Task Scheduler, set up by the installer.

## How it works

```
F1 held  ──► WASAPI capture (16 kHz mono PCM)
release  ──► WAV encode ──► Berget AI ──► dictionary corrections ──► inject (SendInput or clipboard, by class)
```

## Requirements

- **Windows 10/11.**
- A **Berget AI API key** (the transcription backend).
- For building from source only:
  - **Go** (version pinned in `go.mod`).
  - A **C toolchain** — audio capture uses
    [`malgo`](https://github.com/gen2brain/malgo) (cgo). TDM-GCC 10.3.0
    is used on the development machine; any compatible MinGW-w64 GCC
    works.

End users installing a release do **not** need Go or a C compiler — the
installer downloads prebuilt binaries.

## Installation

### End users (prebuilt release)

Run in PowerShell:

```powershell
iwr https://raw.githubusercontent.com/carlosriverosiri/prata/master/install.ps1 | iex
```

The installer:

1. Downloads the latest release to `%LOCALAPPDATA%\Prata`.
2. Prompts for your Berget API key and encrypts it with DPAPI.
3. Registers a Task Scheduler entry so Prata starts at login.

Start it immediately without re-logging in:

```powershell
Start-ScheduledTask -TaskName Prata
```

### Developers (build from the working tree)

From a clone of the repo, with Go and a C toolchain on `PATH`:

```powershell
.\install.ps1 -Local
```

This builds `prata.exe` and `prata-setkey.exe` from source and installs
them the same way.

## Configuration

### API key

Two ways to provide the Berget key, checked in this order:

1. **`BERGET_API_KEY`** environment variable (handy for development).
2. **Encrypted file** at `%LOCALAPPDATA%\Prata\apikey.dat`, written by
   `prata-setkey`:

   ```powershell
   prata-setkey "your-berget-api-key"
   ```

   The key is encrypted with Windows DPAPI and bound to the current user
   and machine — it cannot be decrypted by another account or copied to
   another PC.

### Correction dictionary

Whisper mistakes are corrected before the text is typed. Rules live in a
plain-text file, one per line:

```
# comments and blank lines are ignored
misspelling = correction
adoption = abduktion
```

Matching is case-sensitive with Unicode-aware word boundaries
(`[\p{L}\p{N}_]`); rules apply in file order. Prata looks for the file at
`PRATA_DICT_PATH`, falling back
to `dictionary-corrections.txt` next to the executable. If it is missing
or malformed, Prata logs a warning and runs without corrections.

## Usage

1. Start Prata (autostarts at login, or run `prata.exe`).
2. Hold **F1**. You hear the start tone; speak.
3. Release. You hear the stop tone; a moment later the transcribed text
   is inserted into the focused window.

When run from a terminal, status messages go to stderr (`recording...`,
`transcribing...`, injected text and latency). Press **Ctrl+C** to quit.
When Prata runs at login (no console), right-click the tray icon and choose
**Avsluta** to quit.

## Build from source

```powershell
# main binary (no console window)
go build -ldflags="-s -w -H windowsgui" -o prata.exe ./cmd/prata/

# API-key tool
go build -ldflags="-s -w" -o prata-setkey.exe ./cmd/prata-setkey/
```

`CGO_ENABLED=1` is required (it is the default when a C compiler is
present).

> **Antivirus / EDR note.** Behavioural security products (e.g. Webroot
> SecureAnywhere) may block a freshly built, unsigned `prata.exe` from
> launching — Prata registers global hotkeys, captures the microphone, and
> synthesizes keystrokes, which reads as suspicious for an unknown binary.
> Symptoms are loader-level rejections ("not a valid Win32 application" /
> "Access denied") with no crash logged. For development, `go run
> ./cmd/prata/` runs from the Go build cache and is typically tolerated.
> For deployment, allowlist the install folder or code-sign the binary.
> See PRATA-DESIGN-LOG.md (2026-06-15).

## Project layout

| Path | Purpose |
| --- | --- |
| `cmd/prata/` | Main push-to-talk application. |
| `cmd/prata-setkey/` | Encrypts the Berget API key to disk (DPAPI). |
| `internal/audio/` | WASAPI microphone capture via `malgo` (16 kHz mono PCM). |
| `internal/transcribe/` | Berget AI HTTP client + PCM→WAV encoder. |
| `internal/hotkey/` | Global `RegisterHotKey` listener for F1 (PTT) and F8 (dictionary quick-fix). |
| `internal/inject/` | Hybrid text injection — SendInput Unicode for allowlisted (Chromium/Electron) windows, clipboard paste with preservation otherwise. |
| `internal/dict/` | Word-boundary correction dictionary. |
| `internal/sanity/` | Degenerate-output guard: discards Whisper repetition loops via gzip compression ratio. |
| `internal/auth/` | DPAPI key encryption (`crypt32.dll`). |
| `internal/single/` | Single-instance named-mutex guard. |
| `internal/cue/` | In-process audio cue tones (winmm `PlaySoundW`). |
| `internal/tray/` | System-tray icon + right-click "Avsluta" menu (P/Invoke `shell32`/`user32`). |
| `internal/icon/` | Embedded application icon (`//go:embed Prata.ico`). |

The `cmd/*-test/` directories (`hotkey-test`, `f8-test`, `record-test`,
`inject-test`, `popup-test`, `transcribe-test`, `wav-roundtrip-test`,
`sanity-test`, `tray-test`, `regkey-test`) are isolated smoke-test and
calibration utilities for individual subsystems. `sanity-test` prints gzip
compression ratios for built-in example strings to calibrate the
degenerate-output threshold; `regkey-test` is the `RegisterHotKey` canary
(see ADR 2026-06-09).

## Releasing

Pushing a `v*` tag triggers `.github/workflows/release.yml`, which builds
`prata.exe` and `prata-setkey.exe` on `windows-latest` and publishes them
along with `dictionary-corrections.txt` and `install.ps1` as a GitHub
release.

```powershell
git tag v0.1.0
git push origin v0.1.0
```

## Dependencies

Aside from the Go standard library, Prata depends only on
[`github.com/gen2brain/malgo`](https://github.com/gen2brain/malgo) for
audio capture. Everything else (HTTP, DPAPI, clipboard, hotkey, audio
cues) is direct P/Invoke against Windows DLLs via `syscall`.

## Status

Personal project, Windows-only. See [`CHANGELOG.md`](CHANGELOG.md) for the
development history and [`PRATA-DESIGN-LOG.md`](PRATA-DESIGN-LOG.md) for
architecture decision records.
