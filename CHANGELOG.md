# Changelog

All notable changes to Prata are documented in this file.
Format follows [Keep a Changelog](https://keepachangelog.com/).
Development is organised in numbered phases; the phase entries below
record that history. Tagged releases bundle the phases completed up to
that point.

## [Unreleased]

Hardening from a multi-model external-review triage (the council ran against the
v0.5.0 doc; each finding was verified against the code — most were already handled
or misframed, two were genuinely worth acting on):

### Added

- `cmd/prata` — **"see and forget" health signal, first slice** (PRATA-REVIEW
  §15 #14). Two things a freshly-started daemon does that used to be invisible
  under `-H windowsgui`: (1) a durable **startup log anchor** —
  `daemon started version=… backend=…` — so the daemon log shows whether, and
  when, Prata last came up (a tool that keeps restarting or has not started in
  days now shows it); (2) the **darkest silent failure made visible** — if the F1
  push-to-talk hotkey cannot be registered (almost always another program already
  owns F1), the daemon used to exit with no cue, no balloon, nothing; it now logs
  `FATAL listener stopped` and shows a modal box ("Prata kunde inte starta —
  dikteringstangenten F1 …") so the clinician sees why dictation is dead. The
  rest of the designed health signal (a startup mic probe, a Task Scheduler
  restart-on-failure for the crash class, and possibly a persistent degraded tray
  state / F1 self-healing) is scoped as a follow-up pending a few decisions — see
  PRATA-DESIGN-LOG and §15 #14.

- `internal/audio` + `cmd/prata` — **silent-capture guard.** A capture that is
  long enough but carries no sound (a muted, disconnected, or wrong-default
  microphone) is now dropped with the error cue instead of sent to Whisper, which
  hallucinates a short phrase ("Tack för att ni tittade") on silence that would
  otherwise land in the journal with no feedback. New `audio.Peak` returns the
  loudest 16-bit sample; below `silencePeakFloor` (512, ~1.5% of full scale —
  far below real speech, so a genuine quiet dictation is never dropped) the
  capture is treated as silence. Best-effort and conservative; the dropped peak
  is logged so the floor can be retuned if ever needed. `audio.Peak` is unit-tested.
  On this path Prata also shows a tray balloon **"INGET LJUD — KONTROLLERA
  MIKROFONEN"** (uppercase, readable at a glance): the generic error cue is the
  same double-pulse for five failure paths, so naming this specific, actionable
  one helps. (A spoken-TTS version via SAPI was tried first but the Swedish voice
  was hard to understand, so it was dropped in favour of the written balloon.)

### Fixed

- `cmd/prata` — **panic recovery on the long-running goroutines** so a bug can no
  longer silently kill the "see and forget" daemon. The transcription worker now
  runs each job through `transcribeSafely`, turning a panic into an ordinary
  error (error cue + log, worker keeps running); the F8 worker and the processor
  goroutine recover, log, and cue rather than crashing the process.

## v0.5.0 — 2026-06-25

### Added

- `internal/failover` (new) + `cmd/prata` — an explicit, notify-only backend
  failover hint. When the active local GPU backend fails to respond on two
  consecutive dictations, the tray shows a one-time balloon ("… svarar inte
  upprepade gånger. Byt backend i menyn vid behov (t.ex. Berget Ai).") so the
  user can tell a backend outage from a bad dictation and switch in the menu.
  Prata still has **no silent failover**: nothing switches automatically and
  patient audio is never auto-routed to the cloud — the switch stays a
  deliberate menu action. The hint fires at most once per outage streak and
  resets on the first successful response. The decision logic lives in the
  stdlib-only `internal/failover` package (unit-tested); `cmd/prata` wires it to
  the transcription error path and `tray.Notify`, and records the event in the
  daemon log.
- `cmd/dict-foldin` (new) — the build-time tool that folds valuable per-user
  dictionary override entries into the embedded baseline ahead of a release, so
  clinic corrections (domain knowledge, not personal preference) ship to every
  user. `dict-foldin --override <path> [--baseline …] [--dry-run]`: per key it
  adds a new rule or replaces an existing one in place, preserving the baseline's
  comments, blank lines, and order; empty/identity rules are skipped and baseline
  rules are never removed (idempotent). The merge lives in `internal/dict`
  (`FoldIn`) so it stays identical to the runtime `mergeRules`; the CLI only does
  file I/O and a short added/replaced/skipped report, edits only the baseline
  file (never the user's override), and is run manually by the developer — never
  in the daemon hot path or in CI. Implements the contract specified in
  PRATA-DESIGN-LOG.

### Fixed

- `cmd/prata` — **a very late transcription no longer injects into whatever the
  user is now doing.** Found in the §9 failure-mode review: injection is async (a
  Berget hiccup or queue backlog can delay a result up to 30s), and a result that
  returns long after the user finished dictating would land mid-sentence in text
  they have since started typing by hand. New `maxInjectAge` (8s): past it, the
  result is dropped with an error cue + tray hint ("Dikteringen tog för lång tid
  …") instead of injected. The backend still counts as reachable. Normal
  dictation (sub-second to ~2.7s) is unaffected. (The related cross-patient
  framing was investigated and set aside: Webdoc's window title is static across
  patients and patients share one browser HWND, so Prata cannot detect an in-app
  patient switch; the user's workflow makes it unlikely. See PRATA-DESIGN-LOG.)
- `cmd/prata` — a **too-short capture** (`len(pcm) < minCaptureBytes`) now plays
  the error cue instead of skipping with only the stop cue. A real dictation
  clipped by a slow device start would otherwise vanish silently (the same
  symptom as the paste race). An accidental F1 tap now beeps too — honest
  feedback that nothing was recorded.
- `internal/inject` — **silent paste loss on slow clipboard targets** (found via
  Notepad++). Dictation into Notepad++ produced no text and *no error cue* — the
  dictation just vanished. Root cause: the paste path waited only 50 ms after
  Ctrl+V before restoring the user's clipboard (which calls `EmptyClipboard`).
  Notepad++'s Scintilla editor reads the clipboard slower than that, so the
  dictated text was wiped before it landed. (The history/cloud exclusion markers
  were initially suspected but exonerated: manual Ctrl+V of marked text pastes
  fine in Notepad++.) Fix: `pasteSettleDelay` 50 ms → **400 ms**, deliberately
  generous because silent dictation loss in a patient journal is far worse than
  an imperceptible restore delay — this hardens the whole clipboard-paste path,
  not just Notepad++. Notepad++ is additionally routed through `SendInput`
  (clipboard-free, race-immune; verified live with multi-line text and digit
  strings). Classic Notepad (`Notepad`), Word (`OpusApp`), and PowerPoint read
  fast enough that the old 50 ms already worked.
- `cmd/prata/rsrc_windows_amd64.syso` (new) — `prata.exe` now carries a Windows
  icon resource, so Explorer and the taskbar show the Prata icon instead of the
  generic default. The `//go:embed Prata.ico` in `internal/icon/` only feeds the
  runtime tray icon; the executable's file icon comes from a linked `.syso`,
  which the binary lacked. The committed resource is generated from
  `internal/icon/Prata.ico` with `akavel/rsrc` and `go build` links any `*.syso`
  in the main package automatically, so the CI release build on `windows-latest`
  picks it up too. Regeneration is documented in the README build section.

### Changed

- `internal/sanity` — strengthened the degenerate-output guard with a second,
  complementary signal (PRATA-REVIEW §15 #7). The gzip ratio only catches
  HIGH-repetition token loops; a sentence repeated ~4x compresses to ~1.9 and
  slipped through. New `looksRepeated` flags a multi-word phrase repeated
  back-to-back ≥4 times (anywhere in the text, so an end-of-output loop after
  real dictation is caught too). It is false-positive-safe by construction: four
  identical 2+-word phrases in a row never occur in real clinical dictation,
  whereas legitimate repetition repeats a *word* across *varied* content ("ingen
  X, ingen Y, ...") and so never matches. Analysis backing the gzip threshold
  (token loops score 8–12; the worst legitimate repetitive dictation tops out at
  ~1.8, so the threshold must not be lowered) is locked in by regression tests.
  Accepted gaps: a phrase repeated only 2–3x, and short single-word runs, are
  left alone — ambiguous with legitimate speech, short, and visible to the user.
- `internal/inject` — the clipboard paste path now keeps dictated text out of
  clipboard history (Win+V), the cloud clipboard, and clipboard monitors. After
  placing the text it sets the `CanIncludeInClipboardHistory`,
  `CanUploadToCloudClipboard`, and `ExcludeClipboardContentFromMonitorProcessing`
  marker formats in the same clipboard session (new `setDictatedClipboardText`).
  Every clipboard write Prata makes is marked the same way — the dictated text
  and the restore of the user's prior clipboard alike — so Prata never adds an
  entry to the user's clipboard history, not even a duplicate of the user's own
  earlier copy when a paste restores it; the user sees only what they copied
  themselves. The markers are best-effort, so a failure reverts to the prior
  behavior and never fails the paste. Closes the paste-path confidentiality gap
  (SendInput targets never had it). Win+V exclusion verified live on Windows.
- `internal/daemonlog` — the daemon now prunes its own logs on startup: per-day
  `prata-YYYY-MM-DD.log` files older than 30 days are deleted when the log is
  opened. A "see and forget" daemon that runs for years would otherwise leave one
  small file per active day forever. Best-effort and stdlib-only: an unreadable
  directory or undeletable file is ignored, the date is read from the filename
  (not the mtime), and only files matching the dated pattern are touched, so an
  unrelated file beside the logs is never removed. Skipped when `PRATA_DAEMON_LOG`
  overrides the path (tests).
- `.github/workflows/release.yml` — bumped the three GitHub Actions to their
  Node 24 runtimes (`actions/checkout@v7`, `actions/setup-go@v6`,
  `softprops/action-gh-release@v3`), clearing the "Node.js 20 is deprecated"
  warning GitHub now emits on every release run. Release behaviour is unchanged —
  the same gofmt/vet/build/test gates run and the same assets (`prata.exe` plus
  the USB `.bat` wrappers) ship. The pinned major tags were verified to exist.

## v0.4.0 — 2026-06-23

### Added

- `internal/daemonlog` — a minimal, append-mode per-day daemon log at
  `%LOCALAPPDATA%\Prata\logs\prata-YYYY-MM-DD.log`. Under `-H windowsgui` the
  daemon has no console, so the existing stderr diagnostics are discarded; this
  gives a durable record of each dictation. `cmd/prata` now mirrors every
  per-dictation stderr event (capture start, too-short/queue-full drops,
  transcribe/empty/degenerate/inject outcomes) to this file, stamped with the
  active backend ID and elapsed time. Lines carry metadata only — backend,
  timings, char counts, errors — never the transcribed text, so the file is
  safe by construction. Best-effort and stdlib-only: a log that cannot be opened
  or written falls back to stderr and never disrupts dictation. `PRATA_DAEMON_LOG`
  overrides the full path (test isolation, mirroring `PRATA_INSTALL_LOG`). The
  `logs/` directory and `prata-*.log` files are gitignored.

### Changed

- `cmd/prata/main.go` — the tray tooltip base now carries the build version:
  `tray.New(icon.ICO, "Prata "+version, …)`. On hover it reads `Prata dev` for a
  local build or `Prata <tag>` for a release, and `Prata <tag> — <backend>` once
  a backend is selected (the version slots in ahead of the existing
  ` — <backend>` suffix from `tooltipText`). This is the same string stamped via
  `-ldflags "-X main.version=…"` and reported by the "Sök efter uppdatering…"
  check, so the running release is now visible at a glance without opening that
  menu item. Display-only; no behaviour change.
- `internal/inject/inject.go`, `cmd/prata/main.go` — the async injection path
  now validates the target window with `inject.IsWindow` before attempting focus
  restoration. If the window that was foreground when `F1` was pressed was closed
  during a slow transcription (e.g. switching from patient A's record to patient
  B's), the result is dropped with a distinct "target window gone" diagnostic and
  error cue, instead of failing implicitly inside `RestoreForeground`. The
  failure was already safe; this makes it explicit, faster, and clearer in the
  log. The pre-existing "no target window" case (no foreground window at press
  time) keeps its own distinct message.
- `internal/icon/Prata.ico` — replaced the red Prata tray icon with a yellow
  microphone badge (regenerated from `internal/tray-icon.svg` via ImageMagick).
- `PRATA-GPU-SERVER.md` — Step 2c documents per-machine GPU-server autostart,
  `.bat` launcher, port watchdog, BIOS power-on-after-AC-loss, and cold-boot
  verification on rum-ett (2026-06-22).
- `internal/transcribe/client.go` — renamed two tray backend labels (display
  only; the stable IDs `Hemma`/`Jobb` and existing `backend.txt` are unchanged):
  "Rngv GPU-server" → "Rngv GPU-server (Tailscale)", and "Rum1 GPU-server" →
  "LAN GPU-server".
- `internal/popup/popup.go` — the F8 popup now casts a distinct system drop
  shadow that follows its rounded corners. It is created with `WS_CAPTION` (not a
  bare `WS_POPUP`) so DWM treats it as framed and shadows it, while the visible
  frame is removed by returning 0 from `WM_NCCALCSIZE`. `DwmExtendFrameIntoClientArea`
  (1px bottom margin) keeps the DWM frame and its shadow alive as the client
  fills the whole window; `SetWindowPos(SWP_FRAMECHANGED)` forces the recalc, and
  `WS_VISIBLE` is dropped from creation so `ShowWindow` reveals the reshaped
  window without a title-bar flash. Replaces the removed `CS_DROPSHADOW`, whose
  rectangular shadow clashed with the rounded corners.
- `internal/popup/popup.go` — the F8 popup field now vertically centers its text.
  The EDIT gains `ES_MULTILINE` (still used as one line — Enter is caught by the
  modal loop, so no newline is inserted) so that `EM_SETRECT` actually moves the
  line; a true single-line EDIT ignores the formatting rect's top/bottom and
  stays top-pinned. A multiline EDIT drops its window region on `WM_SETFONT`, so
  the rounded field corners are re-applied (`roundEdit`) after the font is set —
  the same trap the chip hit.
- `internal/popup/popup.go` — dropped the F8 popup's `CS_DROPSHADOW`: its
  rectangular legacy shadow poked a sharp corner past the DWM-rounded window edge
  (worst at the bottom-right). The popup now relies on the DWM rounded corners and
  the teal border for definition; a shadow that follows the rounded contour would
  need the `DwmExtendFrameIntoClientArea` + `WM_NCCALCSIZE` custom-frame approach.
- `internal/popup/popup.go` — the F8 chip is now a rounded badge with roughly
  double the padding around the "F8" text (`baseChipW` 26→38, `baseChipH` 14→20,
  rounded via a `SetWindowRgn` region at `baseChipRadius` 7px @96dpi). The caption
  strip grows (`baseCaptionH` 18→22) to host the taller chip and the window grows
  (`baseHeight` 100→104) to keep the field height.
- `internal/popup/popup.go` — roomier F8 popup spacing (Variant B): the layout
  constants roughly double (`baseMargin` 8→16, `baseGap` 6→14, `baseHeight`
  72→100, `baseTextMargin` 8→12, `baseChipGap` 6→12). Constants only — all layout
  math, brushes, fonts, the DWM border, and region rounding are unchanged.
- `internal/popup/popup.go` — the F8 popup field now has rounded corners via a
  `CreateRoundRectRgn` clipping region (radius 6px @96dpi, DPI-scaled), no
  square `WS_BORDER`, and inner text padding (`EM_SETMARGINS`) so text clears
  the curves. The region is owned by the system after `SetWindowRgn` and is
  never freed by the app.
- `internal/popup/popup.go` — the F8 popup now layers like the approved mockup:
  a thin teal edge via `DwmSetWindowAttribute(DWMWA_BORDER_COLOR)` (Win11;
  harmless no-op on Windows 10), a soft-teal tint panel as the window
  background, and a white field floating in it with more padding (margin/gap/
  height bumped). Three owned brushes (teal chip / tint panel / white field),
  all freed on teardown.
- `internal/popup/popup.go` — the F8 popup caption and chip now use their own
  10pt semibold font (`createFont` generalized to take point size and weight),
  distinct from the 11pt regular field font. The owned font is freed on teardown.
- `internal/popup/popup.go` — the F8 popup caption now carries a small teal
  "F8" chip (white text) at the right of the strip, as a second STATIC child;
  `WM_CTLCOLORSTATIC` branches on the control to give the chip a teal
  background (reusing the frame brush, now promoted onto the popup struct) and
  the caption the tint background. No new GDI objects.
- `internal/popup/popup.go` — the F8 popup now shows a caption label
  "Lägg till i lexikon" above the field, as a STATIC child on the field tint
  with teal text (`WM_CTLCOLORSTATIC`, reusing the field brush). The window
  grew to 62px @96dpi to fit an 18px caption strip; the field height is
  effectively unchanged. Fourth step of the F8 popup restyle (Variant 1).
- `internal/popup/popup.go` — the F8 popup's EDIT field is now tinted a soft
  teal (#F4FBF8) with dark ink text, via a `WM_CTLCOLOREDIT` handler that
  returns a persistent owned brush (created once in `run()`, freed on teardown —
  never per-message). Third step of the F8 popup restyle (Variant 1).
- `internal/popup/popup.go` — the F8 popup now paints a Prata-profile teal
  frame (#0F6E56) by setting the window background to a solid brush and dropping
  the outer `WS_BORDER`; the EDIT control's existing margin exposes the teal as a
  border, which also removes the `WS_BORDER` / rounded-corner clipping artifact.
  Second step of the F8 popup restyle (Variant 1).
- `internal/popup/popup.go` — the F8 quick-fix popup opts into Windows 11
  rounded corners via
  `DwmSetWindowAttribute(DWMWA_WINDOW_CORNER_PREFERENCE, DWMWCP_ROUND)`,
  guarded with `.Find()` so it is a harmless no-op on Windows 10 and earlier.
  (This step originally also added a `CS_DROPSHADOW` shadow, later dropped — see
  above — because it clashed with the rounded corners.) First step of the F8
  popup restyle (Variant 1).

### Fixed

- `internal/transcribe/client.go` — Berget transcriptions dropped the space
  after a sentence-ending period ("förluster.Ungdomarna", "haft.Vi"). The
  earlier särskrivning fix dropped segment newlines unconditionally, which is
  correct for the local whisper.cpp servers (untrimmed segments — the spacing is
  already in the text) but wrong for Berget, which trims each segment line so the
  newline is the only sentence separator. `normalizeTranscript` now takes a
  `trimmedSegments` flag and `Backend.TrimmedSegments` (true only for Berget)
  turns those newlines into spaces, while local backends still concatenate to
  keep a mid-word compound split joined ("Tyd"+"lighet"). Regression-tested; see
  PRATA-DESIGN-LOG (2026-06-21).

## v0.3.0 — 2026-06-21

### Added

- `internal/installer` + `prata --install` — machine-wide, self-elevating
  install (clean-install happy path). Checks token elevation
  (`OpenProcessToken`/`GetTokenInformation`); if not elevated, re-launches
  itself with `--install` via `ShellExecuteW` verb `runas` (a declined UAC
  prompt shows a Swedish message box and exits). Once elevated it copies the
  running binary into `%ProgramFiles%\Prata\prata.exe` (with a source==dest
  guard for idempotent repair) and registers a machine-wide Task Scheduler
  logon task from generated XML (`schtasks /Create /XML`, UTF-16LE BOM): a
  `LogonTrigger` with no `UserId` (all users), principal `GroupId`
  `S-1-5-32-545` (BUILTIN\Users) with implicit interactive logon (no explicit
  `LogonType`, which the v1.2 schema would require before `GroupId`) / **RunLevel
  LeastPrivilege** (the UIPI/medium-IL invariant — never `Highest`),
  `MultipleInstancesPolicy Parallel`, `ExecutionTimeLimit PT0S`. The daemon is
  started post-install via `schtasks /Run` (medium IL in the user session),
  never exec'd from the elevated installer; a failed on-demand start is
  non-fatal ("starts at next sign-in"). Scope: clean install only — migration
  of a per-user install, `--uninstall`, and overwrite-while-running/update are
  later phases. Install mechanics and the medium-IL daemon start were
  hardware-verified 2026-06-20 (copy, task registration, RunLevel Limited,
  daemon from %ProgramFiles%, F1 injection into a non-elevated window; a clean
  clinic-machine run and the UAC-cancel message box are still pending — see
  PRATA-DESIGN-LOG); unit tests cover the task XML, `installDir`,
  `samePath`, and the UTF-16 encoding. Each install step and any error is also
  appended to `%TEMP%\prata-install.log` (timestamped, best-effort) so the
  console-less, message-box-only install path leaves a durable diagnostic trail
  shared by the non-elevated parent and the elevated child.
- `prata --install` migration step — before copying, the elevated install
  terminates any other running `prata.exe` (snapshot via
  `CreateToolhelp32Snapshot`, self PID excluded) to free the session-scoped
  single-instance mutex and unlock the target binary; the copy then retries a
  transiently locked target (10 × 200 ms) and aborts with a message box rather
  than continuing silently if the lock persists. After the daemon is
  (re)started, stale per-user `prata.exe` / `prata-setkey.exe` left by the
  legacy `install.ps1` path are removed from every user profile (only those two
  binaries — user data is preserved). The task XML, RunLevel, and medium-IL
  start path are unchanged. Hardware-verified 2026-06-20 (dirty-state: stale
  daemon terminated, copy retried through the lock, new daemon up at medium
  integrity, user data preserved).
- `prata --uninstall` — machine-wide uninstall mirroring `--install`. Self-elevates
  (`relaunchElevated` was parameterized to take the subcommand; the install path is
  behaviour-identical), terminates running instances, deletes the machine-wide
  `Prata` task, and removes `%ProgramFiles%\Prata` (with a retry for the transient
  post-termination lock). Task deletion is classified locale-safely via
  `schtasks /Query` post-state rather than parsing the (localized) delete output.
  Best-effort teardown: "already absent" counts as success and a genuine leftover
  yields a soft warning message box, not a crash. **Per-user data in
  `%LOCALAPPDATA%\Prata` is left in place** (API key, dictionary, backend choice);
  `PrataWhisperServer` is never touched. Known limit (Option A): running
  `--uninstall` from the installed binary cannot delete its own running `.exe`, so
  the message box tells the user to run it from the USB/original copy.
  Hardware-verified 2026-06-20 (uninstall run from an external copy: running
  daemon terminated, `Prata` task gone, `%ProgramFiles%\Prata` removed, and
  `%LOCALAPPDATA%\Prata` left intact).
- `internal/ui` — minimal Win32 `MessageBox` helper (user32 `MessageBoxW` via
  syscall, UTF-16 strings) for GUI feedback in windowsgui builds that have no
  console. Reusable by later maintenance subcommands; kept off the dictation
  hot path.
- `prata --set-key <key>` — folds the standalone `prata-setkey` flow into the
  main binary as a subcommand. Manual `os.Args` parsing dispatched before any
  daemon setup; reuses `auth.SaveAPIKey` (user-scope DPAPI,
  `%LOCALAPPDATA%\Prata\apikey.dat`, no elevation) and reports success/failure
  via the new message box. `cmd/prata-setkey` stays buildable as a thin wrapper
  over the same `auth` function (removed in a later phase).
- Dictionary **embedded baseline + per-user override**. The baseline
  (`dictionary-corrections.txt`) is now `go:embed`-ed into the binary as an
  immutable layer that always loads. A per-user override is layered on top
  (`dict.LoadDefault`): an override rule adds to, or replaces by key, a
  baseline rule (`mergeRules`). The override path is `PRATA_DICT_PATH` (dev) or
  `%LOCALAPPDATA%\Prata\dictionary-corrections.txt`. `internal/dict/dict_test`
  covers merging/override-wins, missing-override fallback, the embedded
  baseline parsing, and `Save` creating the `%LOCALAPPDATA%` override.

### Changed

- **Phase 6 — update messaging.** The update-available tray balloon now points at
  the supported upgrade path (re-running the Prata install from the USB stick)
  instead of the vague "rerun the installation command"; the
  error/local-build/up-to-date branches are unchanged. Stale `install.ps1`
  references in the `internal/update` package doc and in the `version` /
  `checkForUpdate` comments were corrected to the `--install` re-run flow — the
  installer is a separate process that terminates the daemon before overwriting
  the binary, so there is no self-overwrite and no rename dance. No update-flow
  code changed: `--install` already performs the overwrite-while-running upgrade
  (terminate → retry-copy → re-register → restart), proven by the phase-5b smoke
  test. Verified 2026-06-20 (gates + diff review; string-only change, no new
  mechanic to hardware-test).
- The "no API key" tray warning now says `Kör prata --set-key` instead of the
  stale `Kör prata-setkey` (the key tool was folded into the main binary in
  phase 2).
- Documentation synced (`README.md`, `PRATA-MASTER.md`, `PRATA-GPU-SERVER.md`,
  `PRATA-DESIGN-LOG.md`) for Fas 2–5a: `prata --set-key`, embedded dictionary
  baseline + per-user override, default backend Rum1 GPU-server (`Jobb`),
  machine-wide `prata.exe --install`, and dual install paths (new vs legacy
  `install.ps1`).
- Default backend changed from Berget to **Work** (the local "Jobb" GPU
  server). When `backend.txt` is missing, unreadable, or names an unknown
  backend, `loadBackendPref` now returns `transcribe.Work` instead of
  `transcribe.Berget`. A fresh install with no preference lands on a local GPU
  server that needs no API key, rather than Berget-without-a-key surfacing as
  an error cue on the first F1. An existing valid `backend.txt` is still
  honored unchanged (per-user override wins). Hard-coded default — one binary,
  no separate build, no ldflags.
- `dict.resolvePath` and `cmd/prata`'s `loadDict` no longer compute the
  dictionary path independently: `loadDict` delegates to `dict.LoadDefault`, so
  the daemon, `dict.Save`, and `dict.Reload` always agree on the override
  location. Resolution no longer looks next to the executable (ProgramFiles is
  read-only once installed); F9/`dict.Save` writes only to the override file
  (creating `%LOCALAPPDATA%\Prata` if needed) and never touches the baseline.
  Side effect: this also fixes the `go run` case where the dictionary was
  disabled because no file sat next to the build-cache executable.

- Renamed tray backend labels: Hemma→Rngv GPU-server, Jobb→Rum1 GPU-server,
  Berget→Berget Ai (display only, backend mapping unchanged).
- `Backend` struct split: `Name` → stable `ID` (persisted in `backend.txt`)
  + `DisplayName` (tray menu, tooltip, user-facing messages). Existing
  `backend.txt` files with `Hemma`/`Jobb`/`Berget` continue to work.
- Documentation synced across `PRATA-MASTER.md`, `PRATA-GPU-SERVER.md`, and
  `README.md` for multi-backend support and the new display names.
- `PRATA-GPU-SERVER.md` — verified clinic deployment (2026-06-16): topology
  (rum-ett/rum4), firewall as root cause when server works locally but not
  from client, LAN verification rum4→rum-ett, ~1.4 s latency, KB-Whisper
  verification, and expanded troubleshooting section.

### Removed

- **Phase 7 — legacy install path retired.** Deleted `install.ps1` (root
  PowerShell installer), `cmd/prata-setkey` (folded into `prata --set-key` in
  phase 2), and the duplicate root `dictionary-corrections.txt` (the embedded
  baseline in `internal/dict/dictionary-corrections.txt` is the single source).
  `release.yml` now ships exactly one binary (`prata.exe`) plus the
  `Installera-Prata.bat` / `Avinstallera-Prata.bat` USB wrappers, and no longer
  publishes `prata-setkey.exe`, the root dictionary, or `install.ps1`. A
  deferred Authenticode signing step (gated on a `CODE_SIGN_PFX` secret) is
  wired into `release.yml` as a no-op until a code-signing certificate exists.

### Fixed

- Swedish särskrivningar (mid-word spaces) in dictated text, e.g. "tydlighet" →
  "tyd lighet" and "kärnenergifrågan" → "kärnenergifrå gan". whisper sometimes
  places a timing-segment boundary inside a long compound word; the GPU server
  serializes each segment on its own line in the JSON `text` field
  (`"Tyd\nlighet"`), and `normalizeTranscript` turned every such newline into a
  space via `strings.Fields`/`Join(" ")`. It now drops the segment newlines and
  concatenates with no separator (mirroring Diktell): a real word boundary
  already carries a leading space on the next segment, a mid-word boundary does
  not. The root cause is the client-side assembly, not the whisper.cpp version —
  the same audio reproduces the identical split byte-for-byte on both `v1.8.6`
  and the later HEAD build (see PRATA-DESIGN-LOG 2026-06-21). Unit-tested on the
  real captured server output; live-verified in Swedish dictation.

- F8 dictionary quick-fix failed silently on the first tap in Chromium/Webdoc
  because `CopySelection` read the clipboard after a fixed 50 ms sleep — too
  short for async copy handlers. Holding F8 worked only because RegisterHotKey
  auto-repeat fired many attempts until one won the race. `CopySelection` now
  gates on `GetClipboardSequenceNumber` (captured after `clearClipboard`, then
  polled until it changes, ~300 ms timeout) before reading `CF_UNICODETEXT`.
  Empty or failed captures now play the error cue instead of returning silently.

### Added

- `internal/transcribe` — selectable transcription **backends**. A `Backend`
  (name, URL, `RequiresKey`) and three predefined ones — **Hemma** and **Jobb**
  (local whisper.cpp GPU servers over the LAN/Tailscale, no auth) and **Berget**
  (cloud fallback, Bearer-authenticated). `Client.SetBackend`/`ActiveBackend`
  switch at runtime under a mutex; `Transcribe` posts the same OpenAI-compatible
  multipart form to the active backend's URL and sends `Authorization` only when
  the backend requires it. A backend with no configured URL, or Berget without a
  key, fails before going on the wire. Endpoint URLs are hardcoded constants
  (`HomeURL`/`WorkURL`/`BergetURL`); `WorkURL` is empty until the work server is
  deployed. See `PRATA-GPU-SERVER.md` Steg 5.
- `internal/tray` — `SetBackends(names, active)` adds a row of radio items at the
  top of the right-click menu (bulleted via `CheckMenuRadioItem`), and
  `SetOnSelectBackend` fires on a deliberate switch. The active backend is shown
  in the tooltip ("Prata — Hemma") and refreshed on change.
- `cmd/prata` — wires the tray backend selector to the client: switching updates
  the tooltip, shows a Swedish balloon ("Aktiv transkribering: …", with a caveat
  when Berget lacks a key or Work is unconfigured), and persists the choice to
  `%LOCALAPPDATA%\Prata\backend.txt` (state, not config; default Berget).
- `internal/transcribe/client_test.go` — covers conditional auth and routing:
  Berget sends the Bearer header and form fields, a local backend sends no auth
  even when a key is present, an empty URL fails, Berget without a key fails, and
  `BackendByName` round-trips.
- `PRATA-GPU-SERVER.md` Steg 2b — autostart for the home GPU server. A scheduled
  task (`PrataWhisperServer`) runs `whisper-server.exe` as **SYSTEM at boot**
  (`AtStartup`, `ServiceAccount`/`Highest`, no time limit, restart-on-failure),
  so the Hemma backend behaves like the Tailscale service: it comes up at
  startup without anyone logging in and survives reboots/power loss. Verified on
  the home PC that CUDA works for SYSTEM in session 0 (port listening + a real
  transcription returned correct JSON). Also documents the sleep caveat
  (`standby-timeout-ac`/`hibernate-timeout-ac = 0`) and management commands.
- `PRATA-GPU-SERVER.md` — a copy-paste **install prompt for the work PC**. Drop
  the repo on the clinic machine, paste the prompt into Cursor/Claude, and an
  agent runs the whole work-scenario server setup (GPU/arch detection, build,
  model, LAN firewall, SYSTEM-at-boot autostart, verification, set `WorkURL` +
  rebuild) autonomously, pausing only for the single UAC approval and IT-policy
  decisions. Explicitly LAN-only: never a Tailscale rule, patient audio stays on
  the network.

### Changed

- `internal/transcribe` — `WorkURL` is now set to the clinic GPU server's fixed
  LAN IP (`http://10.64.3.60:8080/v1/audio/transcriptions`) instead of empty, so
  the "Jobb" backend is configured. It is only reachable inside the clinic
  network; selecting it off-site fails with an error cue (no silent fallback).
  `PRATA-GPU-SERVER.md` records the work network (GPU server IP `10.64.3.60`,
  subnet mask `255.255.255.192`, shared DNS) and warns that the `LocalSubnet`
  only covers that small subnet — if dictation workstations sit elsewhere the
  rule must be widened.
- `internal/transcribe` — `Transcribe` now joins the per-segment line breaks
  the backend returns in the `text` field into one flowing prose block instead
  of a poem. Whisper (whisper.cpp server and Berget alike) serializes each
  timing segment on its own line; those breaks land on time-window cuts, not
  sentence boundaries. `normalizeTranscript` mirrors Diktell, concatenating
  segments without a separator — see the Fixed entry and PRATA-DESIGN-LOG
  2026-06-21 for why inserting a space instead caused särskrivningar. The
  end-of-dictation newline (added in `cmd/prata`) is unchanged.
- `cmd/prata` now loads the Berget API key **best-effort** instead of refusing to
  start without one: the local GPU backends need no key, so a missing key only
  fails the Berget backend (with an error cue) rather than blocking startup. The
  HTTP client is no longer Berget-only — it routes to the active backend, and the
  active backend is never switched silently (no automatic failover).

- `internal/update/update.go` — `Check(current)` asks GitHub's
  "latest release" API for the newest published tag and compares it to the
  version stamped into the running binary, returning whether a newer release
  exists and its release-page URL. It is notify-only: it never downloads or
  installs anything (the upgrade still runs through `install.ps1`). This
  deliberately keeps Prata clear of the download-and-execute behaviour that
  behavioural AV/EDR products flag — the same concern as the unsigned-binary
  ADR (2026-06-15). Numeric `vX.Y.Z` comparison ignores any `-`/`+` suffix; a
  non-numeric `current` (a plain `go build`/`go run`, which leaves
  `version = "dev"`) is reported as a local build and never nags.
- `internal/tray` — `SetOnCheckUpdate` adds a **Sök efter uppdatering…**
  item above Avsluta (only when a handler is registered, so `cmd/tray-test`
  keeps just Avsluta), and `Notify(title, text)` shows a tray balloon. Notify
  is goroutine-safe: it stashes the text under a lock and posts a private
  message to the icon's message-loop thread, which owns `Shell_NotifyIconW`.
- `cmd/prata` now embeds a `version` string (stamped via
  `-ldflags "-X main.version=…"`) and wires the tray's update item to
  `update.Check`, reporting the outcome — newer version available, up to
  date, or "local build" — as a Swedish tray balloon. The network call runs
  on its own goroutine so the tray UI thread is never blocked.
- `internal/hotkey/listener.go` — `SetOnF8` registers a callback that
  fires once per F8 tap, on the physical key-up transition: a poll
  goroutine detects release via `GetAsyncKeyState` (20 ms interval) so
  F8 is not physically held when the callback later synthesizes
  Ctrl+C/Ctrl+V. F8 is registered as a system hotkey (`RegisterHotKey`)
  only when a handler is set — without a handler, F8 is not registered
  and passes through untouched globally. A failed F8 registration is
  non-fatal (soft-degrade with a warning to stderr).
- `internal/inject/inject.go` — `CopySelection` grabs the foreground
  window's current selection by synthesizing Ctrl+C and reading the
  clipboard, and is clipboard-neutral: it saves the prior clipboard,
  clears it, copies, settles, reads the selection, then restores the
  prior contents. Clearing first makes "empty after copy" reliably mean
  "nothing was selected". The paste chord helper was generalized to
  `sendChord(vk)` so Ctrl+C reuses it.
- `internal/popup/popup.go` — `Prompt(initial)` shows a small modal
  text-input popup for quick edits: borderless, always-on-top, anchored
  *over the text selection* and opening upward so it lands on the edited
  word rather than the text below it, pre-filled with `initial`
  (select-all), returning the edited text on Enter and cancelling on Esc /
  click-away / close. The anchor is resolved in `anchorPoint` from three
  sources in order: the selection's bounding rectangle via UI Automation
  (`internal/popup/uia.go`), the legacy system caret (`GetGUIThreadInfo`),
  and finally the mouse cursor.
- `internal/popup/uia.go` — UI Automation lookup of the focused element's
  text-selection rectangle (IUIAutomation → focused element → TextPattern →
  GetSelection → GetBoundingRectangles), used to anchor the quick-fix popup
  reliably in Chromium/Electron (the web journal and editor) where the
  legacy caret is reported inconsistently. Pure COM via syscall, run on an
  apartment-isolated goroutine with a 500 ms timeout so an unresponsive
  window can never hang the popup; any failure falls through to the caret
  and mouse fallbacks.
  DPI-aware (per-monitor font scaling via `GetDpiForMonitor` +
  `CreateFontW`). Direct Win32 P/Invoke, stdlib only.
- `cmd/f8-test` — isolated harness wiring the F8 hotkey to `CopySelection`
  and printing the grabbed selection (or "no selection") to stderr.
- `internal/inject/inject.go` — experimental `TypeUnicode`, a clipboard-free
  alternative to `Type`. It synthesizes the whole string as Unicode
  character input (`KEYEVENTF_UNICODE`) and sends it in a *single*
  `SendInput` call; newlines become `Shift+Enter` soft breaks (never a bare
  Enter, which would send the message in chat apps). The single batched
  call is the deliberate difference from the per-rune Phase 4 attempt, which
  autorepeated characters in Electron/Chromium and modern Notepad — the same
  atomic approach the Diktell Rust app uses via enigo. The production
  dictation path (`Type`, clipboard + Ctrl+V) is unchanged. Evaluation of
  clipboard-free injection, parallel to Diktell's ADR 2026-05-24.
- `cmd/inject-test` — `-mode` flag selecting `clipboard` (default,
  `inject.Type`, the existing behavior) or `unicode` (`inject.TypeUnicode`).
  A `-nl` flag (default off) replaces literal `\n` in the argument with a
  real newline before injection, for testing line breaks where the shell
  does not interpret the escape.
- `internal/inject/inject.go` — `ForegroundWindowClass` helper
  (`GetForegroundWindow` + `GetClassNameW`) reporting the foreground
  window's class, and `cmd/inject-test` now logs that class before
  injecting — diagnostics ahead of class-based injection routing. The
  package doc comment now describes both injection paths (clipboard paste
  and SendInput Unicode).
- `internal/inject/inject.go` — class-based injection routing: a hardcoded
  allowlist (`sendInputSafeClasses`) of SendInput-verified window classes,
  `IsSendInputSafeClass`, and `TypeAuto`, which routes to `TypeUnicode`
  (SendInput) for allowlisted foreground classes and to `Type` (clipboard
  paste) for everything else. `cmd/inject-test` gains `-mode auto`
  (`inject.TypeAuto`) and logs the chosen route in that mode.
- `internal/dict/dict.go` — `Save(wrong, correct)` writes a correction
  rule to the dictionary file (same location as loading: `PRATA_DICT_PATH`,
  else `dictionary-corrections.txt` next to the executable), and a `Reload`
  method re-reads the file into a running `Dict`. `Save` trims both fields
  and writes nothing — `(false, nil)` — for an empty field or an identity
  rule (`wrong == correct`). It deduplicates on write by replacing an
  existing key's line in place (matching is first-match-wins, so a trailing
  duplicate would be dead) and otherwise appends, preserving comments,
  blank lines, and unrelated rules verbatim; a missing file is created.
  `Load`/`Apply` and their `cmd/prata` caller are unchanged. Stdlib only.
- F8 step C1 — primitives ahead of the quick-fix orchestrator (no
  orchestrator yet). `internal/inject` exposes `ForegroundWindow` (the
  foreground HWND; `ForegroundWindowClass` now goes through it, unchanged
  behavior) and `RestoreForeground`, which reattaches input to the target
  window's thread (`AttachThreadInput`), calls `SetForegroundWindow`, and
  confirms the window actually became foreground — the safety gate the
  orchestrator uses to abort paste-back on a failed focus restore. (The
  injected-event hook filtering originally added here — `LLKHF_INJECTED` →
  `CallNextHookEx` passthrough — is obsolete under the `RegisterHotKey`
  rewrite below, which cannot self-trigger from synthesized
  Ctrl+C/Ctrl+V/Unicode input.)
- F8 step C2 — the `cmd/prata` quick-fix orchestrator that wires the
  primitives together (no device test yet). A global F8 tap grabs the
  foreground selection (`inject.CopySelection`), splits off its leading/
  trailing whitespace (`splitEnvelope`, rune-aware), shows the trimmed core
  in `popup.Prompt`, and on Enter: hands the rule to the processor
  goroutine over a channel (that goroutine owns the `*dict.Dict` and runs
  `dict.Save` + `Reload`, so no lock is needed), restores focus to the
  source window (`inject.RestoreForeground`, a hard gate — paste-back is
  aborted if it fails so a correction never lands in the wrong window), and
  pastes the correction back over the selection via `inject.TypeAuto` with
  NO trailing newline (unlike the dictation path). An `atomic.Bool`
  single-flight drops overlapping F8 taps, and the channel send is
  non-blocking so the worker never blocks or leaks if the processor is busy
  or gone. The rule persists even if paste-back is aborted; when the
  dictionary was disabled at startup the rule is still saved but the running
  session is not reloaded. `processEvents` is restructured from a `range`
  loop to a `for`/`select` that keeps the existing shutdown semantics
  (a closed `events` channel still returns).
- `internal/cue` — `PlayError()`, an audible error cue: a double low
  pulse (2 × 330 Hz, 110 ms each, 70 ms gap), distinct from the single
  start (880 Hz) and stop (587 Hz) tones in both pitch and rhythm. Same
  0.07 amplitude and the same in-memory winmm `PlaySoundW` mechanism;
  playback is best-effort and can never take the dictation loop down.
  `cmd/prata` plays it on previously *silent* failure paths in the release
  chain — audio start/stop failure, transcribe error/timeout, empty
  transcription, degenerate-transcription discard, and injection error.
  Rationale: the
  production build (`-H windowsgui`) has no console, so these failures
  were completely invisible — the user heard the press/release cues but
  got no text and no indication why (surfaced by the Berget outage
  2026-06-10/11). The stderr lines remain for terminal runs. The
  deliberate "no audio captured" skip (an accidental brief tap) stays
  cue-free so accidental taps are not punished with an alarm.

### Changed

- `internal/hotkey` rewritten from `WH_KEYBOARD_LL` to `RegisterHotKey`
  (ADR 2026-06-09 in PRATA-DESIGN-LOG.md). PTT gesture changes from
  **Ctrl+Win-hold** to **F1-hold**; F8 (dictionary quick-fix) moves from
  the low-level hook to a conditional `RegisterHotKey` registration. The
  `WH_KEYBOARD_LL` failure class — silent unhook on 300 ms callback
  timeout, hook invalidation across sleep/resume, AV/EDR keylogger
  signature — leaves the codebase entirely. The public `Listener`
  interface (`NewListener`, `SetOnF8`, `Run`, `Stop`) is unchanged;
  `cmd/prata` is untouched except user-facing strings and stale comments.
- Dictionary quick-fix hotkey moved from **F9** to **F8** (ADR 2026-06-15
  in PRATA-DESIGN-LOG.md). Diktell owns F9 (and consumes it via its
  low-level hook before Prata's `RegisterHotKey` can match), so on a machine
  running both apps Prata's F9 quick-fix never fired. F8 is unclaimed,
  giving each app its own key: F9 = Diktell, F8 = Prata. Public API renamed
  `SetOnF9` → `SetOnF8`; the test harness `cmd/f9-test` is now `cmd/f8-test`.
  The quick-fix never shipped to users on F9, so this is the first released
  key for the feature.
- `cmd/prata` transcription is now asynchronous: the processor goroutine
  hands each finished capture to a dedicated transcription worker over a
  buffered FIFO channel instead of calling Berget inline, then applies the
  dictionary and injects the result when it comes back. Each capture carries
  the foreground HWND from the F1 press; before injection the processor
  restores and verifies that same window via `inject.RestoreForeground`, and
  aborts with an error cue if focus cannot be restored, so a slow response
  cannot land in a later-focused patient field or chat box. A single worker
  keeps injected text in dictation order. Previously a slow Berget response
  (~24s observed during a hiccup) blocked the whole loop, so F1 appeared
  dead until it returned; now capture stays responsive and the text lands,
  in order, when ready. If the queue fills under a sustained outage the
  capture is dropped with an error cue rather than stalling F1.
- `cmd/prata` serializes all foreground/clipboard/SendInput work with an
  input gate shared by PTT injection and F8 quick-fix. This prevents an async
  transcription result from interleaving with F8's Ctrl+C/Ctrl+V sequence or
  stealing focus from the popup. F8 now also plays the error cue on copy,
  popup, restore, paste, missing-foreground, and rule-queue failures.
- F8 rule persistence is no longer silently lossy under load. `dictAdds` is
  buffered to the same bounded depth as transcription jobs, and a confirmed
  F8 edit aborts paste-back with an error cue if the rule cannot be queued
  within 500 ms. Showing corrected text while losing the persisted rule is
  treated as unsafe.
- `install.ps1` now preserves an existing
  `%LOCALAPPDATA%\Prata\dictionary-corrections.txt` on install/upgrade and
  writes the release copy to `dictionary-corrections.default.txt` instead.
  F8 quick-fix rules are user data; updating Prata must not overwrite them.
- `.github/workflows/release.yml` and `install.ps1 -Local` now stamp the
  binary with a version via `-ldflags "-X main.version=…"` — the release
  workflow uses the pushed git tag (`github.ref_name`); the local installer
  uses `git describe --tags --always`, falling back to `dev`. This feeds the
  in-app "Sök efter uppdatering…" check; previously the binary carried no
  version at all.
- Dictation now routes on the foreground window's class.
  `Chrome_WidgetWin_1` — the whole Chromium/Electron family plus the
  verified web-based journal system, which reports the same class — goes via
  SendInput (`TypeUnicode`): the clipboard is left untouched and the dictated
  text never enters Win+V / cloud-clipboard history. All other windows keep
  clipboard paste (`Type`); an unknown or unreadable foreground window
  defaults to clipboard paste (the safe default). `TypeAuto` deliberately
  does NOT fall back to clipboard paste if SendInput fails — SendInput may
  already have sent characters, so a paste would double-inject (a hazard in a
  patient journal). Modern Notepad (class `Notepad`) is intentionally
  excluded: SendInput fails there on realistic, multi-line text (a short
  test can hide the failure).
- `dictionary-corrections.txt` — corrected the misleading header note that
  claimed a duplicated misspelling lets "the latest line win". Matching is
  first-match-wins and `dict.Save` deduplicates in place, so the first
  occurrence wins; the header now states this.
- `internal/dict` — word-boundary matching is now Unicode-aware (a word
  character is `[\p{L}\p{N}_]`) instead of Go's ASCII `\b`. This fixes
  prior under-matching (a key starting or ending in å/ä/ö never matched)
  and over-matching (e.g. "sken" inside "påsken"); existing rules whose
  keys touch non-ASCII letter boundaries may now behave differently — by
  design, this is the fix. Matching no longer uses `regexp` (literal scan
  plus a rune-aware boundary check) and replacements are inserted verbatim.

## Phase 9 — 2026-05-29

System tray. Prata now puts a small red icon in the notification area with
a single right-click item, **Avsluta** (Quit). This matters because the
production build runs under `-H windowsgui` with no console, so `Ctrl+C`
is never delivered — until now there was no graceful way to quit a
login-started Prata. Avsluta shares the exact Ctrl+C shutdown path.

### Added

- `internal/tray/tray.go` — Windows notification-area icon with a
  right-click "Avsluta" menu. A hidden top-level window pumps the message
  loop on its own OS thread (mirrors `internal/hotkey`); direct P/Invoke
  against `shell32.dll` (`Shell_NotifyIconW`) and `user32.dll`, stdlib
  only, no cgo. The HICON is built from the embedded `.ico` and sized to
  the DPI-scaled `SM_CXSMICON` metric, picking the smallest frame ≥ the
  target so scaling is downward (crisp), never upward (blurry).
  `SetProcessDPIAware` opts the process into per-monitor-v2 awareness. The
  window registers for the shell's `TaskbarCreated` broadcast and
  (re-)adds the icon when the shell becomes ready or Explorer restarts; a
  failed initial `NIM_ADD` is non-fatal and `Run` returns an error only for
  fundamental setup failures (class/window/icon).
- `internal/icon/icon.go` — embeds the red Prata application icon via
  `//go:embed Prata.ico` as `icon.ICO`, so binaries carry the icon with no
  runtime file dependency. The `.ico` has frames at
  16/20/24/32/40/48/64/128/256 px for crisp rendering at every display
  scale.
- `cmd/tray-test/` — isolated smoke test for the tray icon in isolation
  (no audio, no Berget): shows the icon and quits on Avsluta or Ctrl+C.
- `cmd/prata/main.go` (modified) — calls `tray.SetProcessDPIAware()` first,
  then starts the tray after the single-instance guard so a blocked second
  instance never adds an icon. Avsluta and Ctrl+C share one `shutdown`
  closure (stop listener → drain processor → stop tray). A tray that fails
  to start degrades gracefully: it is logged as `tray disabled` and
  dictation keeps running — the same soft-degrade policy already used for
  the correction dictionary, so a notification-area hiccup never takes the
  core push-to-talk loop down.

### Verified

- `gofmt -w`, `go vet ./...`, `go build ./...`, and the production
  `go build -ldflags="-s -w -H windowsgui" -o prata.exe ./cmd/prata/`
  all clean.
- `Prata.ico` validated as a real multi-frame icon (frames at
  16/20/24/32/40/48/64/128/256 px, 32bpp); `pickIconFrame` selects the
  smallest frame ≥ the DPI-scaled target.

### To confirm on device

- Right-click → **Avsluta** quits cleanly with the icon removed (no ghost
  icon), and Ctrl+C still quits in a dev terminal — both via the shared
  shutdown path.
- The icon appears at login and reappears after an Explorer restart (the
  `TaskbarCreated` path); these need a real shell and cannot be verified
  headless.

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
