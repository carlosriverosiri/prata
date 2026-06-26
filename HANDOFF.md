# HANDOFF — state of play after v0.6.0 (2026-06-25)

> **What this is.** A self-contained brief to continue Prata on **another
> machine** with a fresh session that has no memory of the prior work. It
> reflects the repo as of **v0.6.0**. Everything described below is already
> committed, pushed, merged, and released — there is nothing in flight.

---

## 0. Read first (the docs are the source of truth)

- `ROADMAP.md` — the curated status + prioritised backlog (start here).
- `CHANGELOG.md` — the per-change record; `## v0.6.0 — 2026-06-25` is the latest.
- `PRATA-REVIEW.md` **§15** — the open-question discussion (each item numbered).
- `PRATA-DESIGN-LOG.md` — the "why" behind each decision (dated entries).
- `PRATA-MASTER.md` — the hand-curated overview; `AGENTS.md` §1–§2 — the doc map
  and the rule to keep MASTER fresh in the same change as behaviour changes.

## 1. State — clean and released

- `master` @ latest, **working tree clean, fully pushed, no open PRs, no stray
  branches** (only `master` exists locally and on the remote).
- **v0.6.0 is released** (GitHub release with `prata.exe` + `Installera-Prata.bat`
  + `Avinstallera-Prata.bat`): <https://github.com/carlosriverosiri/prata/releases/tag/v0.6.0>.
- The **"see and forget" health signal (§15 #14) is complete** — three slices
  shipped:
  1. a durable startup log anchor (`daemon started version=… backend=…`);
  2. a Task Scheduler **restart-on-failure** (bounded 3× / PT1M) **and** a
     **persistent degraded tray state** (`Tray.SetDegraded`/`ClearDegraded`, a
     non-fading tooltip suffix; `SVARAR INTE` on a backend outage);
  3. **F1 self-heal** — a busy F1 no longer exits the daemon; it stays alive,
     cues + balloons + shows a persistent `F1 UPPTAGEN` badge, and re-probes
     every 3 s, reclaiming F1 the instant it frees, no restart.
- The **developer's primary machine is already running v0.6.0** (installed via
  `--install`, verified by hash + `daemon started version=v0.6.0` in the log).

## 2. Environment — building/testing on the new machine

- Confirm the toolchain: `go version` (Go 1.2x) and `gcc --version` (MinGW-W64 —
  cgo needs a C compiler; `cmd/prata` and `internal/audio` use `malgo`).
- CI gate (also enforced by `.github/workflows/release.yml` on a `v*` tag):
  `gofmt -l .` clean → `CGO_ENABLED=1 go vet ./...` → `go build ./...` →
  `go test ./...`.
- Production build:
  `CGO_ENABLED=1 go build -ldflags="-s -w -H windowsgui -X main.version=<tag>" -o prata.exe ./cmd/prata`.
- **Redeploy notes** (gotchas hit before): a running tray-app daemon is *not*
  killed by `schtasks /End`; use `taskkill /F /IM prata.exe`. `schtasks /Run` is
  Access-Denied from a non-elevated shell (machine-wide task), so to start the
  daemon at the required **medium** integrity outside the installer, launch the
  binary directly (`Start-Process "C:\Program Files\Prata\prata.exe"`). The clean
  install path is `Start-Process prata.exe -ArgumentList '--install' -Verb RunAs
  -Wait` (one UAC + a final "installerad och startad" dialog).

## 3. Candidate next steps (none urgent — pick what you want)

- **Fleet rollout of v0.6.0** — the other ~12 clinic machines still run an older
  version. Update each via USB: `Installera-Prata.bat` from the v0.6.0 download
  (the same `--install` flow). This is where the health-signal robustness matters
  most.
- **Startup mic probe** (§15 #14, the *only* remaining health-signal item) —
  low value; the silent-capture guard already names a dead mic on the first
  dictation. Optional.
- **Next backlog items** (`ROADMAP.md` → 🔭): transport security / response
  authenticity for the LAN GPU (§15 #13) is the next "higher value" candidate;
  then the medium/niche items.

## 4. Git & docs discipline

- Branch from `master` (e.g. `feat/<thing>`), commit per logical change. **End
  every commit message with:** `Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>`.
- Repo pattern: feature branch → PR → merge `--merge --delete-branch`; small
  doc/fix commits can go straight to `master` (confirm with the user).
- Keep the doc set in sync **in the same change** as behaviour: `CHANGELOG.md`
  `[Unreleased]`, `PRATA-DESIGN-LOG.md`, `PRATA-REVIEW.md` §15, `ROADMAP.md`, and
  `PRATA-MASTER.md` (AGENTS.md §2).
- Cutting a release: finalise `CHANGELOG [Unreleased] → vX.Y.Z`, bump the ROADMAP
  status line, commit (`release: cut vX.Y.Z`), then `git tag -a vX.Y.Z` and push
  the tag — the `v*` push triggers the build-and-publish workflow.

## 5. Delete me

This brief is transient — delete it (or rewrite it) once you have moved on from
the state above, so it never goes stale and misleads a fresh session.
