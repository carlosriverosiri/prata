# Contributing to Prata

Prata is a push-to-talk Swedish dictation utility for Windows: hold **F1**,
speak, release, and the transcription is typed into the active window. It is a
bilingual project — developer artifacts (code, comments, commits, `README.md`,
`CHANGELOG.md`, `AGENTS.md`) are in English; product strings (tray menu,
tooltips, dialogs, the correction dictionary) and the design docs
(`PRATA-MASTER.md`, `PRATA-GPU-SERVER.md`, `PRATA-DESIGN-LOG.md`) are in Swedish.
See [AGENTS.md §6](AGENTS.md#6-language-policy) for the full policy.

---

## Prerequisites

| Requirement | Notes |
|---|---|
| **Go** | Version pinned in [`go.mod`](go.mod). |
| **Windows 10/11** | The only supported target; the app is built on Win32. |
| **C toolchain** | Required: audio capture uses [`malgo`](https://github.com/gen2brain/malgo) (cgo). `CGO_ENABLED=1`. TDM-GCC 10.3.0 is used on the dev machine; any compatible MinGW-w64 GCC works. |
| **Berget Ai API key** | Optional — only for the cloud backend. Local GPU backends need no key. |

End users installing a release need **none** of this — the release ships a
prebuilt `prata.exe`.

---

## Quick start

```powershell
git clone https://github.com/carlosriverosiri/prata.git
cd prata

# Optional: install the non-blocking pre-commit reminder to keep
# PRATA-MASTER.md in sync (see "Documentation requirement" below).
.\scripts\install-hooks.ps1

# Build the daemon (no console window; -X stamps the version the in-app
# update check compares against — use "dev" for throwaway builds).
go build -ldflags="-s -w -H windowsgui -X main.version=dev" -o prata.exe ./cmd/prata/

go test ./...
```

### Running from the working tree

```powershell
# go run executes from the Go build cache, which behavioural AV (e.g. Webroot)
# tolerates — unlike a freshly built unsigned prata.exe in a dev folder.
$env:PRATA_DICT_PATH = "internal\dict\dictionary-corrections.txt"  # dev dictionary
$env:BERGET_API_KEY  = "your-key"                                   # only for the cloud backend
go run ./cmd/prata/
```

Why `PRATA_DICT_PATH`: under `go run`, `os.Executable()` resolves to the build
cache, so the per-user override path does not exist; pointing the env var at the
repo dictionary is the dev workaround (not a bug). See
[PRATA-DESIGN-LOG.md](PRATA-DESIGN-LOG.md) (2026-06-15).

---

## Where to read first

1. **[AGENTS.md](AGENTS.md)** — how to work on Prata (stack, language, verification gate, git, forbidden actions).
2. **[PRATA-MASTER.md](PRATA-MASTER.md)** — the curated source of truth: what Prata is and how it is meant to work.
3. **[PRATA-DESIGN-LOG.md](PRATA-DESIGN-LOG.md)** — the decision log and the Win32 traps behind the current design.
4. **[PRATA-GPU-SERVER.md](PRATA-GPU-SERVER.md)** — how to run a local KB-Whisper GPU server (Tailscale at home, LAN-only at the clinic).

---

## Workflow

1. Branch from `master`: `git checkout -b feature/<slug>` or `fix/<slug>`.
2. Make small, logical commits.
3. Run the **verification gate** before every push (it mirrors CI):

```powershell
gofmt -l .          # must print nothing
go vet ./...
go build ./...
go test ./... -count=1
```

4. Commit using [Conventional Commits](https://www.conventionalcommits.org/) in
   English (`feat`, `fix`, `docs`, `refactor`, `test`, `chore`, `perf`, `style`).
5. Open a PR against `master`. CI (`.github/workflows/ci.yml`) runs the same
   gate on `windows-latest`.

> **Shell note.** The dev shell is PowerShell, not bash. `&&` chaining,
> heredocs, and `$(cat …)` do not work. For a multi-line commit message, write
> it to a temp file and `git commit -F <file>`.

---

## Code style

- Go (version in `go.mod`). See [AGENTS.md §7](AGENTS.md#7-code-style-and-conventions).
- **Errors as values** — anything that can fail returns `error`. No `panic` in
  production paths (except unrecoverable startup in `main`).
- Small, single-purpose functions; small files.
- Exported identifiers carry a `//` doc comment explaining *purpose*.
- `gofmt` and `go vet` clean before every commit.
- **No new dependency for trivial functionality.** Prata's only external
  dependency is `malgo`; everything else is stdlib + direct Win32 via `syscall`.
  Adding or swapping a dependency is a deliberate decision — state why first, and
  never swap/major-bump `malgo` without confirmation.

---

## Documentation requirement

`PRATA-MASTER.md` is **hand-maintained, not generated** — it only stays correct
if you keep it fresh. Any change in behavior requires a documentation update **in
the same commit** as the code change; do not defer it.

- Behavior / feature / flow change → `PRATA-MASTER.md` (+ `README.md` if public).
- Design decision or Win32 trap → `PRATA-DESIGN-LOG.md` (dated entry).
- Backend / server / network change → `PRATA-GPU-SERVER.md`.
- Any user-visible change → `CHANGELOG.md` under `[Unreleased]`.

The optional pre-commit hook (`scripts/install-hooks.ps1`) prints a **non-blocking**
reminder when a notable document changed but `PRATA-MASTER.md` was not staged. It
never blocks a commit. See [AGENTS.md §2](AGENTS.md#2-the-single-most-important-rule--keep-prata-mastermd-fresh).

---

## Test harnesses

`internal/*` packages have unit tests (`go test ./...`). The `cmd/*-test/`
directories are isolated, manually run smoke-test and calibration utilities for
individual subsystems:

| Harness | Purpose |
|---|---|
| `cmd/regkey-test` | `RegisterHotKey` canary (F1/F9 delivery, repeat suppression) — see ADR 2026-06-09. |
| `cmd/hotkey-test` | End-to-end hotkey listener. |
| `cmd/record-test` | WASAPI capture via `malgo`. |
| `cmd/transcribe-test` | Backend HTTP client against a real endpoint. |
| `cmd/wav-roundtrip-test` | PCM→WAV encoding. |
| `cmd/inject-test` | Hybrid injection with route logging (`-mode auto`). |
| `cmd/popup-test`, `cmd/f8-test` | F8 popup rendering and quick-fix flow. |
| `cmd/sanity-test` | Prints gzip compression ratios to calibrate the degenerate-output guard. |
| `cmd/tray-test` | Tray icon, menu, balloon. |

---

## Reporting issues

Open a [GitHub issue](https://github.com/carlosriverosiri/prata/issues) with
reproduction steps, the Windows version, the active backend, and the relevant
log: for install problems the contents of `%TEMP%\prata-install.log`, and for
dictation/runtime problems today's daemon log at
`%LOCALAPPDATA%\Prata\logs\prata-YYYY-MM-DD.log` (metadata only — backend,
timings, char count, sanity ratio, error strings — never the transcribed text).

> **Antivirus / EDR.** A freshly built, unsigned `prata.exe` may be blocked at
> launch by behavioural security products (e.g. Webroot SecureAnywhere) because
> it registers global hotkeys, captures the microphone, and synthesizes
> keystrokes. Symptoms are loader-level rejections ("not a valid Win32
> application" / "Access denied") with no crash logged. For development use
> `go run ./cmd/prata/`; for deployment, allowlist the install folder or
> code-sign the binary. See PRATA-DESIGN-LOG.md (2026-06-15).
