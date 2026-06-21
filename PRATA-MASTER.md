# Prata — Master Document

> **Hand-curated overview — not generated.** This is the project's collected truth in
> distilled form. It is kept current by hand: update it in the same change as the
> behavior changes (see `AGENTS.md` §2). It is deliberately *not* a concatenation of the other
> docs — the value lies in the synthesis.

## What Prata is

A minimal Windows-native push-to-talk dictation app for Swedish medical dictation with
`KBLab/kb-whisper-large`. Transcription runs against a selected backend: a local whisper.cpp GPU server
over the network (**Rngv GPU-server (Tailscale)** / **LAN GPU-server**) or **Berget Ai** as a cloud fallback.
Designed as a complement to Diktell on machines without a dedicated GPU. The backend setup
is described in `PRATA-GPU-SERVER.md`.

## User flow

### F1 — dictation

1. Carlos holds `F1` down
2. Prata records microphone audio (16 kHz mono PCM)
3. When `F1` is released: send the audio to the selected backend (Rngv GPU-server (Tailscale) / LAN GPU-server / Berget Ai)
4. Normalize the response into running prose (join Whisper's per-segment lines **without** a separator, like Diktell, so that long compound words are not split apart) and apply dictionary corrections to the text
5. Restore the window that was active when `F1` was pressed and write the text there via class-based routing (SendInput Unicode in Chromium/Electron windows, otherwise clipboard paste — see Decision 6 in the design log). If the window cannot be restored, injection is aborted with an error tone instead of letting text land in the wrong place.
6. Transcription runs asynchronously in a FIFO worker, so a slow backend round does not block the next `F1` recording.

### F8 — dictionary quick-fix

1. Carlos selects a mistranscribed word or expression
2. Presses `F8`
3. Prata copies the selection and shows a small popup over the selection
4. Carlos types the correct form and presses Enter (Esc/clicking outside cancels)
5. The rule is saved to the per-user override file (`%LOCALAPPDATA%\Prata\dictionary-corrections.txt`), the dictionary is reloaded, the source window is restored, and the corrected text is pasted back

## Components

- **Hotkey** — global F1 (PTT) and F8 (dictionary quick-fix) via `RegisterHotKey`
- **Audio capture** — 16 kHz mono PCM via WASAPI (`malgo` Go binding for miniaudio)
- **HTTP client** — POST multipart to the selected backend; OpenAI-compatible form (`file`, `model`, `language`, `response_format`)
- **Backend selector** — Rngv GPU-server (Tailscale) / LAN GPU-server / Berget Ai as radio buttons in the tray menu; the active backend is shown in the tooltip + balloon. The choice is saved as a stable ID (`Hemma` / `Jobb` / `Berget`) in `%LOCALAPPDATA%\Prata\backend.txt` — the display names can change without breaking saved choices. **Default on first start (missing or invalid `backend.txt`): LAN GPU-server (`Jobb`)** — internal GPU without an API key. Conditional auth (only Berget Ai sends Bearer). No silent failover. See `PRATA-GPU-SERVER.md`.
- **Dictionary** — two layers: (1) a **baseline** embedded in the binary at build time (`go:embed` of `internal/dict/dictionary-corrections.txt`); (2) a **per-user override** in `%LOCALAPPDATA%\Prata\dictionary-corrections.txt` (F8 writes here). The override is layered on top of the baseline (replacing per key). Unicode-aware word-boundary replacements (literal `strings.Index`, no regexp).
- **Text injection** — class-based routing: Chromium/Electron (class `Chrome_WidgetWin_1`, including the web-based medical record) → `SendInput` Unicode, the whole string in a single call, the clipboard is never touched; other windows → clipboard paste (`CF_UNICODETEXT`, save/restore). See Decision 6.

## Berget AI — API-detaljer

- **Endpoint**: `https://api.berget.ai/v1/audio/transcriptions`
- **Model**: `KBLab/kb-whisper-large`
- **Format**: multipart/form-data
- **Auth**: Bearer token, DPAPI-encrypted locally
- **Price**: €3 per 1000 minutes of audio = ~50 öre / month for Carlos's usage
- **Latency** (measured 2026-05-27 on the main machine):
  - Mean: 2.61 sec
  - Min: 2.56 sec
  - Max: 2.77 sec
  - Spread: 0.21 sec across 5 calls (very consistent)
  - No cold-start effect observed

## Phase 0 measurements

- The model produces an **identical error pattern** to local Diktell (the same KB-Whisper-Large)
- `dictionary-corrections.txt` from Diktell is **directly reusable** without modification
- Berget AI is ~1.5–2 sec slower than a local RTX GPU for repeated dictation
- For one-off dictations the difference is smaller (local has 1850 ms model loading on cold start)

## Distribution

Machine-wide install via USB — one binary, no separate tools:

| Path | Target | Autostart | Status |
|---|---|---|---|
| **`prata.exe --install`** (double-click `Installera-Prata.bat`) | `%ProgramFiles%\Prata\prata.exe` | Machine-wide Task Scheduler (`Prata`, all users, RunLevel Limited) | Implemented (install/uninstall/update, Phases 5–7) |

- **Uninstallation:** `prata.exe --uninstall` (double-click `Avinstallera-Prata.bat`) — removes the task + `%ProgramFiles%\Prata`, leaves per-user data.
- **Key:** `prata --set-key <key>` (user-scope DPAPI → `%LOCALAPPDATA%\Prata\apikey.dat`). The standalone `prata-setkey` has been **removed (Phase 7)** — folded into `prata --set-key`.
- **Writable state** always lives per user in `%LOCALAPPDATA%\Prata\` (`apikey.dat`, `backend.txt`, dictionary override). No machine-wide writable data in `%ProgramData%`.
- **Update:** notify-only (not automatic). The tray menu has "Sök efter uppdatering…". The upgrade itself is done manually — a **USB re-run of `--install`** from the new binary. The binary never replaces itself.
- The version is stamped in via `-ldflags "-X main.version=<tag>"` in the release build.
- **Hard invariant:** the daemon is never started directly from the elevated installer (HIGH IL → UIPI blocks SendInput). Post-install start happens via `schtasks /Run` (medium IL). See the design log 2026-06-17.

## What Prata IS

- Two operations: `F1` PTT dictation and `F8` dictionary quick-fix
- Fully local except for the HTTP call to the selected transcription backend (a local GPU server on the network, or Berget Ai)
- API key DPAPI-encrypted on the machine (only needed for the Berget Ai backend)
- Audio feedback via short tones: a start tone (880 Hz) at recording start, a stop tone (587 Hz) on release, and an error tone (a double 330 Hz pulse) on the silent failure paths in the release chain
- Single binary (daemon + `--install` + `--uninstall` + `--set-key`), no runtime, no model file
- Hard-coded endpoint constants; the backend *choice* is saved as state (not config) in `backend.txt`
- Maintenance subcommands (`--install`, `--uninstall`, `--set-key`) report via `MessageBoxW` (windowsgui = no console)

## What Prata is NOT

- Not cross-platform (Windows-only for now — Mac/Linux may come later)
- Not configurable (change a constant + recompile)
- Not commercial — personal + collegial use
- Not a cloud-first system — local-first with a cloud fallback for transcription
- Not a framework — it is a tool

## Phases

_Original plan from Phase 0. The actual phases and status — including work after Phase 7
(hybrid injection, tray icon, F8 dictionary additions) — are documented in the CHANGELOG._

- **Phase 0** — verify Berget AI (done 2026-05-27)
- **Phase 1** — HTTP client + WAV encoding in isolation
- **Phase 2** — audio capture with malgo
- **Phase 3** — hotkey handling (WH_KEYBOARD_LL)
- **Phase 4** — text injection (SendInput / KEYEVENTF_UNICODE)
- **Phase 5** — dictionary corrections
- **Phase 6** — DPAPI API key + Task Scheduler autostart
- **Phase 7** — GitHub Actions + install.ps1 (original plan)

### Installer ADR (2026-06-16 — in progress)

| Phase | Contents | Status |
|---|---|---|
| 0 | Delivery branch (Branch A: USB, ~12 machines, local admin) | ✅ Answered |
| 1 | Signtool hook (deferred) + AV allowlisting in the runbook | ⏳ Hook not coded; Defender exception in `--install` deferred |
| 2 | `--set-key` + `MessageBoxW` | ✅ |
| 3 | Dictionary embed + per-user override | ✅ |
| 4 | Default backend Jobb | ✅ |
| 5a | `--install` happy path (clean machine) | ✅ |
| 5b | Migration of old per-user install (kill instances → retry-copy → legacy binary cleanup) | ✅ Hardware-verified 2026-06-20 |
| 5c | `--uninstall` (self-elevate → kill instances → remove task + `%ProgramFiles%\Prata`; leaves per-user data) | ✅ Hardware-verified 2026-06-20 |
| 6 | Update = `--install` re-run from USB (the mechanics already exist; Phase 6 = notice text + docs) | ✅ Verified 2026-06-20 |
| 7 | Release.yml → one binary + `Installera-Prata.bat`/`Avinstallera-Prata.bat`; legacy `install.ps1`/`prata-setkey`/root dictionary removed; `PRATA_INSTALL_LOG` override | ✅ 2026-06-20 — code + docs done; .bat hardware smoke-tested (launch + å/ö + pause on network drive); release.yml review-verified (full validation on the first `v*` tag) |

## Relationship to Diktell

Diktell is "finished" and frozen. Only security and crash fixes will be applied. All new
development happens in Prata. Diktell runs on machines with a GPU; Prata runs everywhere else. They are
sibling tools, not versions.
