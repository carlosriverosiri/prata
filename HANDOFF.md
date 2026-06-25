# HANDOFF — finish the "see and forget" health signal (§15 #14)

> **What this is.** A self-contained brief to continue Prata's "see and forget"
> health-signal work on a **different machine** (a home Windows PC, via Claude
> Desktop / local Claude Code). It is transient — delete it once the work lands.
> You have no memory of the prior session; everything you need is here or in the
> repo docs it points to.

---

## 0. Read first (the design already exists — don't re-derive it)

- `PRATA-DESIGN-LOG.md` → the dated entry **"2026-06-25: 'See and forget' health
  signal — design, and the first slice shipped"** (the full design, what shipped,
  what's deferred, the open decisions, and the honest limits).
- `PRATA-REVIEW.md` **§15 #14**, and `ROADMAP.md` item **#1**.
- `AGENTS.md` §1 (documentation map) and §2 (keep `PRATA-MASTER.md` fresh).

## 1. State

- Repo `master` @ **c06ea5e**, clean, fully pushed. v0.5.0 is released; new work
  lives in `CHANGELOG.md` "[Unreleased]".
- The health signal's **first slice already shipped** (c06ea5e): a startup log
  anchor (`daemon started version=… backend=…`) and a **visible F1-registration
  failure** — a modal box instead of a silent exit under `-H windowsgui`.
- This handoff is the **remaining 4 items**.

## 2. Environment — you are LOCAL on a Windows machine

This is the simple path: you run on the home machine's disk, so you do the **full
build and the live tests yourself** — no cloud split. First confirm the toolchain:

```
go version          # expect Go 1.2x
gcc --version        # expect MinGW-W64 (cgo needs a C compiler)
```

- Both present → proceed. CI gate: `CGO_ENABLED=1 go vet ./...` /
  `CGO_ENABLED=1 go build ./...` / `CGO_ENABLED=1 go test ./...`.
  Production build: `CGO_ENABLED=1 go build -ldflags="-s -w -H windowsgui -X main.version=dev" -o prata.exe ./cmd/prata`.
- `gcc` missing → install MinGW-W64 (e.g. MSYS2: `pacman -S mingw-w64-ucrt-x86_64-gcc`) — `cmd/prata` and `internal/audio` need cgo (`malgo`). Without cgo you can only build/test the pure packages (`internal/hotkey`, `internal/installer`, `internal/tray`) and `GOOS=windows` cross-compile.
- To run/live-test: stop any running instance, then launch the freshly built
  `prata.exe`. (PowerShell: `Get-Process prata | Stop-Process -Force; Start-Process .\prata.exe`.)

## 3. The 4 items (recommendation + the OPEN DECISION — confirm with the user)

### 1) F1 self-healing vs fatal — *biggest; a behaviour change*
Today: `internal/hotkey/listener.go` `Run()` registers F1 **fatally** → returns an
error → `cmd/prata/main.go` (`case err := <-listenerDone:`) logs `FATAL`, shows a
modal box, and exits. **Option:** keep the daemon **alive** and re-probe
`RegisterHotKey(F1)` on a timer until it succeeds (self-heals when the program that
owns F1 closes), surfacing a degraded state (item 3). This is the meatiest change —
it touches the listener lifecycle **and** `main`'s shutdown (which waits on
`listenerDone`). `internal/hotkey` is pure syscall. **Live-test:** have another app
claim F1 (or simulate the failure), start Prata, confirm it survives + recovers when
that app closes. *Decision for the user: stay fatal (current) or self-heal.*

### 2) Task Scheduler restart-on-failure — *touches the INSTALL path*
`internal/installer/installer.go` → `taskXML(exePath)` (~line 661) builds the task
XML. Add to the `<Settings>` block:
`<RestartOnFailure><Interval>PT1M</Interval><Count>3</Count></RestartOnFailure>`.
**Caveat (see the comment ~lines 645-657): `schtasks /XML` rejects out-of-order
`<Settings>` children — place it in the schema-correct position.** Update
`installer_test.go`. *Decision: bounded (3×, recommended — self-heals a crash without
a crash-loop) vs unbounded.* Note it only fires on a non-zero **exit**; with the
panic recovery already in place the daemon rarely exits, so this mainly catches a
hard process death. **Verify by running `prata.exe --install` (admin) on Windows**
and checking `schtasks /Query /TN Prata /XML` shows the restart settings; optionally
kill the daemon and confirm Task Scheduler restarts it.

### 3) Persistent degraded tray state — *so it's seen, not just a dismissable balloon*
`internal/tray/tray.go` — the tooltip is built by `tooltipText()`/`updateTooltip()`.
Add a way to set a degraded tooltip (e.g. `Prata — F1 UPPTAGEN`) and/or a distinct
icon state, called from `cmd/prata` when degraded. Pure syscall. **Live-test** the
visual on Windows. (Pairs with item 1's self-heal so the degraded state is visible
while F1 is unavailable.)

### 4) Startup mic probe — *LOW priority; the silent-capture guard already covers it*
`internal/audio/capture.go` — add a best-effort `DeviceAvailable()` (init a capture
device without `Start`, then `Uninit`; false if `InitDevice` fails); wire into
`cmd/prata` startup, balloon if no mic. CGO (`malgo`). **Live-test:** mute/disconnect
the mic, start Prata, confirm the balloon. Lower value since a missing mic already
surfaces on the first dictation (error cue + the silent-capture balloon).

## 4. Suggested order

**#2 + #3 first** (self-contained, fully buildable/testable on their own), then
**#1** after the user confirms keep-alive-vs-fatal, then **#4** (lowest value).

## 5. Git & docs

- Branch from `master` (e.g. `feat/health-signal-rest`). Commit per item. **End every
  commit message with:** `Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>`.
- This repo's pattern: small slices committed to `master` (the v0.5.0 work was a
  feature branch merged `--no-ff`, then doc/fix commits went directly to `master`).
  Confirm with the user whether to merge via a branch or commit to `master`.
- Keep these in sync as you go: `CHANGELOG.md` [Unreleased]; `PRATA-DESIGN-LOG.md`
  (extend the 2026-06-25 health-signal entry); `PRATA-REVIEW.md` §15 #14;
  `ROADMAP.md` #1; and `PRATA-MASTER.md` if behaviour changes (AGENTS.md §2).
- Run the full cgo gate (§2) and live-test before committing patient-safety-relevant
  changes — never let a dictation fail silently; that is the whole point of this work.
