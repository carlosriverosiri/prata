# Prata

Push-to-talk Swedish dictation for Windows. Hold **Ctrl+Win**, speak,
release — your speech is transcribed and typed into whatever window has
focus. Transcription runs on [Berget AI](https://berget.ai) using
KBLab's `kb-whisper-large` model.

Prata is a lightweight background utility: no application window, just a
single system-tray icon you can right-click to quit. It registers a global
hotkey, captures the microphone while the keys are held, sends the audio to
Berget, applies a correction dictionary, and pastes the result.

## Features

- **Push-to-talk** via a global Ctrl+Win keyboard hook — works in any
  foreground application.
- **Swedish transcription** through Berget AI (`KBLab/kb-whisper-large`).
- **Gentle audio cues** — a higher tone when recording starts, a lower
  tone when it stops, synthesised in-process (no sound files).
- **Correction dictionary** — word-boundary text replacements fix
  recurring Whisper errors (e.g. `adoption` → `abduktion`).
- **Clipboard-paste injection** — reliable in Chromium/Electron apps and
  modern Notepad; the previous clipboard content is preserved and
  restored.
- **Encrypted API key** at rest via Windows DPAPI (per-user, per-machine).
- **Single-instance guard** — a named mutex prevents two copies from
  double-typing.
- **System-tray icon** — a small red Prata icon in the notification area;
  right-click and choose **Avsluta** to quit. This is the primary way to
  exit when Prata runs at login with no console window.
- **Autostart at login** via Task Scheduler, set up by the installer.

## How it works

```
Ctrl+Win held ──► WASAPI capture (16 kHz mono PCM)
release       ──► WAV encode ──► Berget AI ──► dictionary corrections ──► clipboard paste
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

Matching is case-sensitive with ASCII word boundaries; rules apply in
file order. Prata looks for the file at `PRATA_DICT_PATH`, falling back
to `dictionary-corrections.txt` next to the executable. If it is missing
or malformed, Prata logs a warning and runs without corrections.

## Usage

1. Start Prata (autostarts at login, or run `prata.exe`).
2. Hold **Ctrl+Win**. You hear the start tone; speak.
3. Release. You hear the stop tone; a moment later the transcribed text
   is pasted into the focused window.

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

## Project layout

| Path | Purpose |
| --- | --- |
| `cmd/prata/` | Main push-to-talk application. |
| `cmd/prata-setkey/` | Encrypts the Berget API key to disk (DPAPI). |
| `internal/audio/` | WASAPI microphone capture via `malgo` (16 kHz mono PCM). |
| `internal/transcribe/` | Berget AI HTTP client + PCM→WAV encoder. |
| `internal/hotkey/` | Global `WH_KEYBOARD_LL` hook for Ctrl+Win. |
| `internal/inject/` | Clipboard-paste text injection with clipboard preservation. |
| `internal/dict/` | Word-boundary correction dictionary. |
| `internal/sanity/` | Degenerate-output guard: discards Whisper repetition loops via gzip compression ratio. |
| `internal/auth/` | DPAPI key encryption (`crypt32.dll`). |
| `internal/single/` | Single-instance named-mutex guard. |
| `internal/cue/` | In-process audio cue tones (winmm `PlaySoundW`). |
| `internal/tray/` | System-tray icon + right-click "Avsluta" menu (P/Invoke `shell32`/`user32`). |
| `internal/icon/` | Embedded application icon (`//go:embed Prata.ico`). |

The `cmd/*-test/` directories (`hotkey-test`, `record-test`,
`inject-test`, `transcribe-test`, `wav-roundtrip-test`, `sanity-test`,
`tray-test`) are isolated smoke-test and calibration utilities for
individual subsystems. `sanity-test` prints gzip compression ratios for built-in
example strings to calibrate the degenerate-output threshold.

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

Personal project, Windows-only. See [`CHANGELOG.md`](CHANGELOG.md) for
the development history.
