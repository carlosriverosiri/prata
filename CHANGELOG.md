# Changelog

All notable changes to Prata are documented in this file.
Format follows [Keep a Changelog](https://keepachangelog.com/).
Development is organised in numbered phases; the phase entries below
record that history. Tagged releases bundle the phases completed up to
that point.

## [Unreleased]

### Added

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

### Fixed

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
- `internal/transcribe` — `Transcribe` now collapses the per-segment line
  breaks the backend returns in the `text` field into single spaces, so an
  injected dictation reads as one flowing prose block instead of a poem.
  Whisper (whisper.cpp server and Berget alike) serializes each timing segment
  on its own line; those breaks land on time-window cuts, not sentence
  boundaries. The new `normalizeTranscript` mirrors Diktell, which concatenates
  segments without a separator. The end-of-dictation newline (added in
  `cmd/prata`) is unchanged.
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
