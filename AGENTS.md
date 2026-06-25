# AGENTS.md — meta-document for AI agents

> Read this at the start of every session. It describes **how to work on Prata**.
> The companion document `PRATA-MASTER.md` describes **what Prata is** (curated truth).
> Read both before changing anything.

---

## Current Truth

- **Two operations**: `F1` (push-to-talk dictation) and `F8` (dictionary quick-fix popup). Nothing else is a user workflow operation.
- **Dictation pipeline**: `F1` held → record 16 kHz mono PCM → on release, send to the selected backend → normalize the response to running prose → apply dictionary corrections → inject into the window that was active when `F1` was pressed (class-based routing: Chromium/Electron → `SendInput` Unicode; other windows → clipboard paste). Transcription runs asynchronously in a FIFO worker so a slow round never blocks the next `F1`.
- **Backends**: two local whisper.cpp GPU servers (no auth) — **Rngv GPU-server (Tailscale)** (`Hemma`) and **LAN GPU-server** (`Jobb`) — plus **Berget Ai** (`Berget`, Bearer key, DPAPI-encrypted). Selection is persisted as a *stable ID* in `%LOCALAPPDATA%\Prata\backend.txt`; display names can change without breaking saved choices. Default on first run is **LAN GPU-server** (`Jobb`). No silent failover — a dead backend produces an error cue, not a fallback.
- **Prata is the active development project.** Diktell is finished and frozen; all new development happens here. Prata targets machines *without* a dedicated GPU (Diktell needs a local CUDA GPU). They are sibling tools, not versions.
- **Distribution**: single binary. `prata.exe --install` / `--uninstall` / `--set-key`, machine-wide Task Scheduler autostart, USB install. Updates are **notify-only** (tray "Sök efter uppdatering…"); the upgrade is a manual re-run of `--install` from the new binary on USB. The binary never replaces itself.
- **Near-zero dependencies**: the only external Go dependency is `malgo` (audio capture). Everything else — hotkeys, tray, clipboard, text injection, DPAPI, the F8 popup, MessageBox, single-instance — is implemented with direct Win32 calls via `syscall`/`unsafe`. Treat this minimal footprint as a feature.
- **`PRATA-MASTER.md` is hand-maintained, not generated.** It only stays correct if the agent keeps it in sync (see §2).

## 1. Documentation map

Each document has a distinct job. Know which one to update.

| File | Purpose | Language |
| --- | --- | --- |
| `PRATA-MASTER.md` | Curated single source of truth — *what is built and how it was reasoned*, at a glance. **Hand-maintained.** | English |
| `README.md` | Public entry point / overview. | English |
| `PRATA-GPU-SERVER.md` | Backend & server setup: network topology, Tailscale vs LAN, firewall, deployment. | English |
| `PRATA-DESIGN-LOG.md` | Design decisions and Win32 traps — the *"how I reasoned"* log (dated entries). | English |
| `CHANGELOG.md` | Release / work history (Keep a Changelog). | English |
| `AGENTS.md` | This file — process and policy for agents. | English |
| `CONTRIBUTING.md` | Developer setup and contribution workflow. | English |
| `PRATA-REVIEW.md` | Self-contained snapshot for soliciting external AI feedback. **Derived, not a source of truth** — regenerated on demand, not kept perfectly in sync. | English |

## 2. The single most important rule — keep `PRATA-MASTER.md` fresh

`PRATA-MASTER.md` is **not** auto-generated. Unlike a mechanical concatenation, it is curated synthesis, so it drifts the moment behavior changes and no one updates it. Prevent that:

- **AI ownership of doc updates**: when you implement a feature or change documented behavior, update the relevant docs in the **same agent run** as the code change. Never defer to a separate task and never ask the user to do it later.
- Route the update to the right file:
  - Behavior / feature / user-flow change → `PRATA-MASTER.md` (+ `README.md` if it is publicly visible).
  - Design decision or a Win32 gotcha worth remembering → `PRATA-DESIGN-LOG.md` (dated entry).
  - Backend / server / network change → `PRATA-GPU-SERVER.md`.
  - Any user-visible change → a `CHANGELOG.md` entry under `[Unreleased]`.
- **Match the existing language of the file you edit** (see the table in §1). Do not translate a Swedish design doc to English or vice versa.

### What does NOT require a `PRATA-MASTER.md` update

- Pure refactor with no behavior change.
- `gofmt` / `go vet` fixes.
- Comment or typo fixes.
- Test-only changes.

## 3. The user

The user is a domain expert — a clinician who builds AI-assisted tools but does not write Go directly. He drives all code through AI assistants and reads it at a high level.

- Often communicates via **dictated Swedish** — expect spoken language, reordered words, half-finished sentences. Interpret charitably.
- Prefers **concise, structured Swedish** in chat. Code, comments, and commit messages stay English (see §5) — chat language ≠ code language.
- Values direct communication and motivated decisions: when recommending A over B, say why in one line.
- High confidence at the project level — do not hold back challenging objections if a design seems weak.
- High-autonomy posture: make normal engineering calls and report them; stop only for destructive, irreversible actions (see §10).

## 4. Tech stack

| Area | Choice | Notes |
| --- | --- | --- |
| Language | **Go 1.26** (`go.mod`) | Windows-only; `CGO_ENABLED=1` (malgo uses cgo). |
| Audio capture | **gen2brain/malgo** | miniaudio/WASAPI binding — the *only* external dependency. |
| Hotkeys, tray, clipboard, injection, DPAPI, popup, MessageBox, single-instance | **stdlib `syscall` + direct Win32** | No third-party libraries; bindings are hand-rolled in `internal/`. |
| HTTP | **stdlib `net/http`** | multipart POST to OpenAI-compatible transcription endpoints. |

Adding a dependency is a deliberate decision — Prata's near-zero-dependency posture is intentional. State the rationale before adding one, and never swap or major-bump `malgo` without confirmation.

## 5. Project structure

```
prata/
├── AGENTS.md            # This file
├── PRATA-MASTER.md      # Curated source of truth (hand-maintained)
├── README.md            # Public overview
├── PRATA-GPU-SERVER.md  # Backend/server/network setup
├── PRATA-DESIGN-LOG.md  # Design decisions + Win32 traps
├── CHANGELOG.md         # Release/work history
├── CONTRIBUTING.md      # Developer setup and contribution workflow
├── PRATA-REVIEW.md      # Self-contained snapshot for external AI review (derived)
├── go.mod / go.sum      # Single external dep: malgo
├── Installera-Prata.bat / Avinstallera-Prata.bat
├── scripts/
│   └── install-hooks.ps1   # Installs the optional pre-commit stale-warning
├── .github/workflows/      # ci.yml (fmt+vet+build+test), release.yml
├── cmd/
│   ├── prata/              # The daemon + --install / --uninstall / --set-key
│   ├── dict-foldin/        # Build-time tool: fold override entries into the baseline
│   └── *-test/             # Manual test harnesses (f8, hotkey, inject, popup, …)
└── internal/
    ├── audio/      # malgo capture
    ├── auth/       # DPAPI key storage
    ├── cue/        # audio feedback tones
    ├── daemonlog/  # durable per-dictation file log (%LOCALAPPDATA%\Prata\logs, metadata only)
    ├── dict/       # correction dictionary (embedded baseline + per-user override)
    ├── failover/   # notify-only backend-failure hint (never auto-switches)
    ├── hotkey/     # global F1/F8
    ├── icon/       # tray icon asset
    ├── inject/     # SendInput / clipboard text injection
    ├── installer/  # --install / --uninstall logic
    ├── popup/      # F8 popup window (Win32)
    ├── sanity/     # startup self-checks
    ├── single/     # single-instance guard
    ├── tray/       # system tray menu + tooltip
    ├── transcribe/ # HTTP client, backends, WAV encoding, response normalization
    ├── ui/         # MessageBox helpers
    └── update/     # update-check (notify-only)
```

## 6. Language policy

Prata separates developer/documentation language (English) from product-facing strings (Swedish). **All documentation is English** — including the design docs, which are the author's working documents.

- **English** — all code (identifiers, `//` comments, log/panic messages), commit messages, branch names, and **every document**: `README.md`, `CHANGELOG.md`, `AGENTS.md`, `CONTRIBUTING.md`, `PRATA-MASTER.md`, `PRATA-GPU-SERVER.md`, `PRATA-DESIGN-LOG.md`, `PRATA-REVIEW.md`.
- **Swedish (product strings only)** — UI text (tray menu, tooltip, `MessageBox`, F8 popup), user-facing error messages, `dictionary-corrections.txt` content, and **Swedish dictation examples inside docs** (e.g. `tydlighet → tyd lighet`). These illustrate real Swedish ASR/UI behavior and stay verbatim even in an English doc.

When writing a new document, write it in English.

## 7. Code style and conventions

- **Errors as values**: anything that can fail returns `error`. No `panic` in production paths (except unrecoverable startup in `main`).
- **Small, single-purpose functions** and small files. Split when a file grows unwieldy.
- **Exported identifiers** get a `//` doc comment explaining *purpose*, not mechanics.
- **`gofmt` clean** and **`go vet` clean** before every commit.
- **No new dependency** for trivial functionality — prefer stdlib + a few lines of Win32.

## 8. Verification gate (before declaring a task done)

Mirror CI (`.github/workflows/ci.yml`). Run, and fix anything that fails before reporting done:

```
gofmt -l .          # must print nothing
go vet ./...
go build ./...
go test ./... -count=1
```

## 9. Git routine (Windows / PowerShell)

- **Conventional commits in English**: `feat`, `fix`, `docs`, `refactor`, `test`, `chore`, `perf`, `style`.
- **Commit often**, one logical unit at a time. Branch per feature: `feature/<slug>` or `fix/<slug>`. `master` is production.
- **Never force-push** without explicit confirmation.
- **Shell gotcha**: the dev shell is PowerShell, not bash. `&&` chaining, heredocs (`<<'EOF'`), and `$(cat …)` do **not** work. For multi-line commit messages, write the message to a temp file and use `git commit -F <file>`; chain commands with `;` or run them separately.
- Before each commit, verify with `git status` / `git diff --cached` that no secrets (API keys) or build artifacts are staged.

## 10. What the agent must NEVER do without explicit confirmation

1. Change the hotkey bindings (`F1`, `F8`) — discuss ergonomics first.
2. Write the audio buffer to disk (privacy: it may be patient audio — keep it in memory only).
3. Make the repository public, or push anything that would expose it to a public audience.
4. Force-push, or rewrite shared git history.
5. Delete source files/directories the user did not mention (build artifacts like `*.exe` and `dist/` are fine).
6. Add, replace, or major-bump dependencies (especially swapping `malgo`).
7. Edit per-user state files or anything under `%LOCALAPPDATA%\Prata\`.

## 11. Security

- The Berget Ai API key is **DPAPI-encrypted** at `%LOCALAPPDATA%\Prata\apikey.dat` (user scope) — never commit it, never log it. Local GPU backends need no key.
- **Patient audio is never written to disk** — buffered in memory and discarded after the transcription round.
- Logs may contain transcribed text — keep `logs/` gitignored. (The daemon log written by `internal/daemonlog` to `%LOCALAPPDATA%\Prata\logs\` is metadata-only by design — backend, timings, char counts, error strings, never the transcribed text — but treat that as a property to preserve, not a licence to relax this rule for other log sources.)
- The repository is private. Before any push that would expose it publicly, request explicit confirmation.

---

*Last updated: 2026-06-25.*
