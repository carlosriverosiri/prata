# Prata

Push-to-talk Swedish dictation for Windows. Hold **F1**, speak,
release — your speech is transcribed and typed back into the window that
was active when you started dictating. Transcription uses KBLab's
`kb-whisper-large` model against a **selected backend**: a local whisper.cpp
GPU server over the network (**Rngv GPU-server (Tailscale)** at home, **LAN GPU-server**
at the clinic) or **[Berget Ai](https://berget.ai)** as the cloud fallback.

Prata is a lightweight background utility: no application window, just a
single system-tray icon you can right-click to quit or switch backend. It
registers a global hotkey, captures the microphone while the key is held,
sends the audio to the active backend, applies a correction dictionary, and
inserts the result.

## Features

- **Push-to-talk** via a global F1 hotkey (`RegisterHotKey`) — works in
  any foreground application.
- **Swedish transcription** via `KBLab/kb-whisper-large` — against a local
  GPU server (whisper.cpp over the LAN/Tailscale) or Berget Ai in the cloud.
- **Backend selector** — right-click the tray icon and pick **Rngv GPU-server (Tailscale)**,
  **LAN GPU-server**, or **Berget Ai** (radio items). The active backend is
  shown in the tooltip and a balloon on change. The choice is persisted as a
  stable ID (`Hemma` / `Jobb` / `Berget`) in `%LOCALAPPDATA%\Prata\backend.txt`.
  Local GPU backends need no API key; Berget Ai requires one. No silent failover
  — if the chosen server is down you get an error cue, not an automatic switch.
  See [`PRATA-GPU-SERVER.md`](PRATA-GPU-SERVER.md) for server setup.
- **Gentle audio cues** — a higher tone when recording starts, a lower
  tone when it stops, and a distinct double low pulse when a capture,
  transcription, injection, or quick-fix step fails. Cues are synthesised
  in-process (no sound files).
- **Correction dictionary** — word-boundary text replacements fix
  recurring Whisper errors (e.g. `adoption` → `abduktion`). A baseline
  ships embedded in the binary; per-user overrides (including F8
  quick-fix rules) live in `%LOCALAPPDATA%\Prata\dictionary-corrections.txt`.
- **F8 quick-fix** — select a mis-transcribed word or phrase, tap **F8**,
  edit it in a small popup anchored over the selection, and press Enter.
  Prata saves the rule to the dictionary and pastes the corrected text back.
- **Async transcription** — a slow backend response does not freeze the next
  F1 capture. Dictations are transcribed by one FIFO worker and injected
  back into the window that was active when each capture started.
- **Hybrid text injection** — routed on the foreground window's class.
  Chromium/Electron apps and the web-based journal receive the text via
  SendInput Unicode, leaving the clipboard untouched (so a copied
  screenshot survives and dictated text stays out of Win+V / cloud
  clipboard); every other window uses clipboard paste with the previous
  clipboard content preserved and restored. Anything uncertain defaults to
  clipboard paste.
- **Encrypted API key** at rest via Windows DPAPI (per-user, per-machine).
  Required only for the Berget Ai backend; local GPU servers take no auth.
- **Single-instance guard** — a named mutex prevents two copies from
  double-typing.
- **System-tray icon** — a small red Prata icon in the notification area;
  right-click and choose **Avsluta** to quit. This is the primary way to
  exit when Prata runs at login with no console window.
- **Update check** — the same right-click menu has **Sök efter
  uppdatering…**, which asks GitHub whether a newer release exists and
  reports the result in a tray balloon. It is notify-only: it never
  downloads or replaces the binary itself; you upgrade by re-running the
  installer (see [Updating](#updating)).
- **Autostart at login** via Task Scheduler. A machine-wide install
  (`prata.exe --install`) registers one logon task for all users (see
  [Installation](#installation)).

## How it works

```
F1 held  ──► capture target window + WASAPI capture (16 kHz mono PCM)
release  ──► WAV encode ──► active backend ──► dictionary corrections ──► restore target ──► inject (SendInput or clipboard, by class)

F8 tap   ──► copy current selection ──► popup edit ──► save dictionary rule ──► restore source ──► paste corrected text
```

## Requirements

- **Windows 10/11.**
- A **Berget Ai API key** only if you use the cloud backend (not needed for
  the local GPU servers). Prata starts without a key; the default backend is
  the clinic GPU server, which needs no key.
- For building from source only:
  - **Go** (version pinned in `go.mod`).
  - A **C toolchain** — audio capture uses
    [`malgo`](https://github.com/gen2brain/malgo) (cgo). TDM-GCC 10.3.0
    is used on the development machine; any compatible MinGW-w64 GCC
    works.

End users installing a release do **not** need Go or a C compiler — the
release ships a prebuilt `prata.exe`.

## Installation

Prata installs **machine-wide from a USB stick**: copy the release folder
(`prata.exe` plus the `Installera-Prata.bat` / `Avinstallera-Prata.bat`
wrappers) to the machine and run the installer once with elevation.

### Install (machine-wide)

Double-click **`Installera-Prata.bat`** — it runs `prata.exe --install` and
keeps its window open if the launch is blocked (e.g. by AV). Or run directly:

```powershell
.\prata.exe --install
```

Approve the UAC prompt. The installer:

1. Copies the running binary to `%ProgramFiles%\Prata\prata.exe`.
2. Registers a machine-wide Task Scheduler entry (`Prata`) so the daemon
   starts at every user's logon at **medium integrity** (required for
   SendInput into non-elevated apps such as Webdoc).
3. Starts Prata in the current session when possible (`schtasks /Run`).

Per-user data (`apikey.dat`, `backend.txt`, dictionary overrides) lives under
`%LOCALAPPDATA%\Prata` — the Program Files copy is read-only.

> **Antivirus / EDR.** Unsigned binaries may be blocked until the install
> folder is allowlisted (e.g. Webroot). See [Build from source](#build-from-source)
> and PRATA-DESIGN-LOG.md (2026-06-15). A hardware smoke test on an
> allowlisted machine is documented in PRATA-DESIGN-LOG.md (2026-06-17).

Set the Berget Ai key when needed (cloud backend only):

```powershell
prata.exe --set-key "your-berget-api-key"
```

### Uninstall

Double-click **`Avinstallera-Prata.bat`** (or run `prata.exe --uninstall`)
from the USB/original copy. It stops Prata, removes the `Prata` task and
`%ProgramFiles%\Prata`, and **leaves your per-user data** (API key, backend
choice, dictionary) under `%LOCALAPPDATA%\Prata` so a reinstall keeps it. Run
it from the USB copy rather than the installed binary — a running `.exe`
cannot delete itself.

### Updating

Updating is the same flow re-run from the new release on the USB stick:
double-click `Installera-Prata.bat` (or run `prata.exe --install`) from the
**new** copy. `--install` terminates the running daemon, overwrites
`%ProgramFiles%\Prata\prata.exe`, re-registers the task, and restarts Prata,
so an in-place update needs no separate step. Re-run it from the USB copy, not
the installed binary: running the already-installed `prata.exe --install` only
repairs the task and restarts (the `samePath` guard skips the copy), without
picking up a new version. On a shared clinic PC this also stops any other
signed-in user's daemon, so update when nobody is dictating.

Right-click the tray icon and choose **Sök efter uppdatering…** to check
whether a newer release exists; Prata reports the result in a balloon but
never updates itself. The update check is deliberately notify-only rather than
a self-updater: a binary that downloads and runs a new binary over itself is
exactly the behaviour that behavioural AV/EDR products (e.g. Webroot) flag for
an unsigned executable. See PRATA-DESIGN-LOG.md (2026-06-15).

### Developers (build from the working tree)

From a clone of the repo, with Go and a C toolchain on `PATH`:

```powershell
go build -ldflags="-s -w -H windowsgui -X main.version=dev" -o prata.exe ./cmd/prata/
.\prata.exe --install          # machine-wide (UAC); --uninstall to remove
```

## Configuration

### Transcription backend

Right-click the tray icon and choose one of three backends:

| Display name | Stable ID | Where it points |
|---|---|---|
| Rngv GPU-server (Tailscale) | `Hemma` | Home GPU server over Tailscale |
| LAN GPU-server | `Jobb` | Clinic GPU server on the LAN |
| Berget Ai | `Berget` | Berget Ai cloud API |

The active choice is saved to `%LOCALAPPDATA%\Prata\backend.txt` as the
stable ID and survives restarts. **Default on first run is LAN GPU-server**
(`Jobb`) — the clinic LAN GPU server, which needs no API key. Endpoint URLs
are hardcoded constants in the binary — see
[`PRATA-GPU-SERVER.md`](PRATA-GPU-SERVER.md) for server setup and how to
change them.

### API key

Only required for the **Berget Ai** backend. Two ways to provide it, checked
in this order:

1. **`BERGET_API_KEY`** environment variable (handy for development).
2. **Encrypted file** at `%LOCALAPPDATA%\Prata\apikey.dat`, written by
   `prata --set-key`:

   ```powershell
   prata.exe --set-key "your-berget-api-key"
   ```

   The key is encrypted with Windows DPAPI and bound to the current user
   and machine — it cannot be decrypted by another account or copied to
   another PC.

### Correction dictionary

Whisper mistakes are corrected before the text is typed. The active dictionary
has two layers:

1. **Baseline** — embedded in the binary at build time (always present).
2. **Per-user override** — `%LOCALAPPDATA%\Prata\dictionary-corrections.txt`
   (created on first F8 save). Override rules add to, or replace by key,
   baseline rules.

Override file format, one rule per line:

```
# comments and blank lines are ignored
misspelling = correction
adoption = abduktion
```

Matching is case-sensitive with Unicode-aware word boundaries
(`[\p{L}\p{N}_]`); rules apply in file order.

For development, set **`PRATA_DICT_PATH`** to point at a specific file
(highest priority). `go run ./cmd/prata/` works without it — the embedded
baseline always loads.

## Usage

1. Start Prata (autostarts at login, or run `prata.exe`).
2. Right-click the tray icon and confirm the active backend (Rngv GPU-server (Tailscale),
   LAN GPU-server, or Berget Ai). Switch if needed.
3. Hold **F1**. You hear the start tone; speak.
4. Release. You hear the stop tone; a moment later the transcribed text
   is inserted into the window that was active when you pressed F1. If that
   window cannot be safely restored, Prata skips the injection and plays the
   error cue instead of typing into the wrong place.
5. To add a dictionary rule, select the incorrect word/phrase, tap **F8**,
   edit the popup text, then press **Enter**. Press **Esc** or click away to
   cancel. F8 and PTT injections are serialized so their clipboard and
   keystroke operations cannot interleave.

When run from a terminal, status messages go to stderr (`recording...`,
`transcribing...`, injected text and latency). Press **Ctrl+C** to quit.
When Prata runs at login (no console), right-click the tray icon and choose
**Avsluta** to quit.

## Build from source

```powershell
# main binary (no console window); -X stamps the version the in-app
# update check compares against (use a tag, or "dev" for throwaway builds)
go build -ldflags="-s -w -H windowsgui -X main.version=dev" -o prata.exe ./cmd/prata/
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
| `cmd/prata/` | Main push-to-talk application (`--install`, `--uninstall`, `--set-key`, daemon). |
| `internal/installer/` | Machine-wide `--install` (elevation, copy, Task Scheduler XML). |
| `internal/ui/` | Win32 `MessageBox` for subcommands (no console in windowsgui builds). |
| `internal/audio/` | WASAPI microphone capture via `malgo` (16 kHz mono PCM). |
| `internal/transcribe/` | Multi-backend transcription client + PCM→WAV encoder. |
| `internal/hotkey/` | Global `RegisterHotKey` listener for F1 (PTT) and F8 (dictionary quick-fix). |
| `internal/inject/` | Hybrid text injection — SendInput Unicode for allowlisted (Chromium/Electron) windows, clipboard paste with preservation otherwise. |
| `internal/dict/` | Word-boundary correction dictionary (embedded baseline + per-user override). |
| `internal/sanity/` | Degenerate-output guard: discards Whisper repetition loops via gzip compression ratio. |
| `internal/auth/` | DPAPI key encryption (`crypt32.dll`). |
| `internal/single/` | Single-instance named-mutex guard. |
| `internal/cue/` | In-process audio cue tones (winmm `PlaySoundW`). |
| `internal/tray/` | System-tray icon, backend selector, update check, and Avsluta menu (P/Invoke `shell32`/`user32`). |
| `internal/icon/` | Embedded application icon (`//go:embed Prata.ico`). |
| `internal/dict/dictionary-corrections.txt` | Baseline dictionary source (embedded at build time). |

The `cmd/*-test/` directories (`hotkey-test`, `f8-test`, `record-test`,
`inject-test`, `popup-test`, `transcribe-test`, `wav-roundtrip-test`,
`sanity-test`, `tray-test`, `regkey-test`) are isolated smoke-test and
calibration utilities for individual subsystems. `sanity-test` prints gzip
compression ratios for built-in example strings to calibrate the
degenerate-output threshold; `regkey-test` is the `RegisterHotKey` canary
(see ADR 2026-06-09).

## Releasing

Pushing a `v*` tag triggers `.github/workflows/release.yml`, which builds
`prata.exe` on `windows-latest` and publishes it together with the
`Installera-Prata.bat` and `Avinstallera-Prata.bat` USB wrappers as a GitHub
release. The correction dictionary ships embedded in the binary (from
`internal/dict/dictionary-corrections.txt`) — no separate dictionary file is
published. A deferred Authenticode signing step (gated on a `CODE_SIGN_PFX`
secret) is wired in but a no-op until a code-signing certificate exists.

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
[`PRATA-GPU-SERVER.md`](PRATA-GPU-SERVER.md) documents how to run a local
KB-Whisper GPU server on the LAN (home via Tailscale, clinic via LAN-only)
as an alternative to Berget Ai.
