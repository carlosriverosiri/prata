# Prata — Complete Overview for External Review

> **Purpose.** This is a *self-contained* document meant to be pasted into various
> AI models to gather feedback, criticism, and new ideas. A reviewer should be able
> to understand the entire app — features, technology, design choices, and open
> questions — without access to the code or the other documents.
>
> **Status.** Snapshot **2026-06-23** (v0.4.0). This document is a *distillation* — the running truth lives
> in `PRATA-MASTER.md`, `PRATA-DESIGN-LOG.md`, `PRATA-GPU-SERVER.md`, `README.md`,
> and `CHANGELOG.md`. It is not generated automatically; update it when you want a
> fresh round of review.
>
> **At the very bottom** there is a section *"Questions for the reviewer"* — feel
> free to start there if you are an AI being asked to give feedback.

---

## TL;DR

Prata is a minimal, Windows-native push-to-talk app for **Swedish medical
dictation**. You hold **F1**, speak, release — the audio is transcribed with
`KBLab/kb-whisper-large` against a chosen backend (a local whisper.cpp GPU server
over the network, or Berget Ai in the cloud), run through a correction dictionary,
and typed into the window that was active when you pressed F1. A second operation,
**F8**, is a dictionary quick-fix. The app has no window of its own — just a tray
icon. It is written in **Go** with **a single external dependency** (`malgo` for
audio); everything else is direct Win32 via `syscall`. It is built to be *installed
and forgotten* ("see and forget") on ~12 shared clinic computers, and the entire
design is steeped in **patient confidentiality** (patient audio never to disk,
dictated medical-record text never to the clipboard/cloud clipboard; only
metadata — backend, timings, error codes, never the transcribed text — is ever
written to disk).

Prata is a sibling tool to **Diktell** (the same developer's existing, frozen
dictation app in Rust with local CUDA Whisper). Diktell requires a dedicated GPU;
Prata fills the gap on machines without a GPU.

---

## 1. Context and problem

- **The user** is an orthopedist/physician who builds AI tools but does not write
  code himself — all code is driven via AI assistants. High architectural
  understanding, reads code at a high level.
- **The environment** is a hospital where the user often **switches computers
  during the day**. Many of these are mini-PCs **without a GPU**, where Diktell
  (local CUDA Whisper) cannot run. That is the problem Prata solves.
- **The text lands in a patient record** (web-based, "Webdoc"). That raises the
  bar: a wrong injection is a patient-safety risk, and patient data must not leak.
- **Scale:** ~10–12 clinic computers, logged-in clinicians have local admin,
  distribution via USB stick (not Intune/SCCM yet, but the design is prepared for
  it).

---

## 2. Design principles

1. **"See and forget"** — installed once, should work for years without
   supervision. This drives the choice of Go (Go 1 compatibility promise), a
   self-contained binary with no runtime, and no configuration files.
2. **Minimalism / stdlib-only** — a single external dependency (`malgo`).
   Everything else (HTTP, DPAPI, clipboard, hotkeys, audio, tray, installer) is
   direct Win32 P/Invoke. No packaging tools (MSI/Inno/WiX), no frameworks.
3. **Patient safety is a hard invariant** — several design choices (see §5, §8,
   §9) are locked so that dictated text can never end up in the wrong place or
   leak.
4. **Windows-only for now** — no cross-platform abstraction is paid for before it
   is needed. Mac/Linux is "maybe later".
5. **No app-initiated workflow UI** — only audio cues in the flow, a passive tray
   icon, and a user-initiated F8 popup. The app never "interrupts".

---

## 3. Features

### 3.1 F1 — dictation (the main flow)

1. The user holds **F1** down (global hotkey via `RegisterHotKey`).
2. Prata captures the foreground window (the injection target) and records the
   microphone (16 kHz mono PCM via WASAPI/`malgo`). A start tone (880 Hz) plays.
3. On release (stop tone 587 Hz): PCM → WAV → POST (multipart,
   OpenAI-compatible) to the chosen backend.
4. The response is normalized to running prose (segment joining — see §7.4) and
   run through the correction dictionary.
5. The target window is restored and the text is typed in via **class-based
   routing** (see §8). If the window cannot be restored safely → the injection is
   aborted with an error tone (better no text than the wrong place).
6. Transcription happens **asynchronously in a FIFO worker** — a slow backend
   round does not block the next F1 recording.

### 3.2 F8 — dictionary quick-fix

1. The user selects a mistranscribed word/phrase and presses **F8**.
2. Prata copies the selection and shows a small popup (DWM shadow, rounded
   corners, F8 chip) anchored over the selection.
3. The user types the correct form and presses Enter (Esc/click outside cancels).
4. The rule is saved to the per-user override file, the dictionary is reloaded,
   the source window is restored, and the corrected text is pasted back.

F8 and F1 injections are **serialized** so that their clipboard/keystroke
operations cannot interleave with each other.

### 3.3 Other

- **Backend selector** in the tray menu (radio buttons) — see §7.
- **Audio cues** are synthesized in-process (winmm `PlaySoundW`), no audio files:
  start (high tone), stop (low tone), error (double low pulse on all silent error
  paths).
- **Tray icon** (small yellow microphone badge): backend selection, "Sök efter
  uppdatering…", "Avsluta". The primary way to exit when the app runs at login
  without a console. The tooltip shows the running build version and the active
  backend — e.g. `Prata v0.4.0 — LAN GPU-server` (`Prata dev` on a local build).
- **Update check** — notifying, never self-updating (see §9.3).
- **Single-instance guard** — a named, session-bound mutex (`Local\`) → one
  instance per session on a shared PC.
- **Autostart** via a machine-wide Task Scheduler task (see §9).

---

## 4. Architecture and technology

| Area | Choice | Note |
|---|---|---|
| Language | **Go** (1.26, see `go.mod`) | Go 1 compat, self-contained binary, ~150 MB toolchain. |
| Audio capture | **gen2brain/malgo** (miniaudio/WASAPI, cgo) | The only external dependency; `CGO_ENABLED=1`. |
| Hotkeys, tray, clipboard, injection, DPAPI, popup, MessageBox, single-instance, installer | **stdlib `syscall` + direct Win32** | No third-party libraries; the bindings are hand-written in `internal/`. |
| HTTP | **stdlib `net/http`** | multipart POST to OpenAI-compatible transcription endpoints. |

**Thread model:** the hotkey listener runs on the message queue (`RegisterHotKey`
→ `WM_HOTKEY` for the press; 20 ms `GetAsyncKeyState` polling for the release,
started on press and stopped on release = zero idle cost). Transcription runs in
**a FIFO worker** separate from the recording.

**Package map (`internal/`):** `audio` (malgo capture), `transcribe` (multi-backend
HTTP client + WAV encoder + normalization), `hotkey` (F1/F8 via RegisterHotKey),
`inject` (hybrid text injection), `dict` (dictionary: embedded baseline +
override), `sanity` (degenerate-output guard via gzip ratio), `auth` (DPAPI),
`single` (mutex guard), `cue` (audio cues), `daemonlog` (per-day metadata-only
file log), `tray` (icon/menu/balloon/update
check), `icon` (`go:embed` of the icon), `installer` (machine-wide
`--install`/`--uninstall`), `ui` (`MessageBox` helper), `update` (notifying version
check), `popup` (the F8 popup, Win32/DWM). `cmd/prata/` is the daemon + the
subcommands; `cmd/*-test/` are isolated smoke-test/calibration tools.

---

## 5. Transcription and backends

### 5.1 The three backends

| Display name | Stable ID | Points to | Auth |
|---|---|---|---|
| Rngv GPU-server (Tailscale) | `Hemma` | Home GPU (whisper.cpp) over Tailscale | None |
| LAN GPU-server | `Jobb` | The clinic's GPU on the LAN | None |
| Berget Ai | `Berget` | Berget Ai cloud API | Bearer key (DPAPI) |

- All run **the same model** (`KBLab/kb-whisper-large`) → the same error patterns,
  and Diktell's `dictionary-corrections.txt` is directly reusable.
- **The endpoint URLs are hardcoded constants** in the binary (the backend
  *selection* is state, not configuration).
- **Berget:** `https://api.berget.ai/v1/audio/transcriptions`, multipart,
  zero retention, servers in Stockholm (data does not leave Sweden — for a
  physician probably the only *legitimate* cloud service for dictated medical
  text). ~50 öre/month at the user's volume; ~2.6 s latency (measured).

### 5.2 Selection, persistence, default

- The selection is saved as a **stable ID** in `%LOCALAPPDATA%\Prata\backend.txt`
  → the display name can change without breaking saved selections.
- **Default on first run: `Jobb` (LAN GPU-server)** — internal GPU without a key.
  (Otherwise a new user would have hit Berget-without-a-key on F1 → error tone.)
- **No silent failover** — if the chosen server is down you get an error tone, not
  a silent switch. A switch happens only when the user selects in the menu.

### 5.3 Network topology and confidentiality

- **The clinic's GPU is NEVER exposed over Tailscale.** Patient audio must not
  leave the clinic's network. The firewall is scoped to LocalSubnet/Domain.
- **The home GPU** is always reached externally over Tailscale (Tailscale IP,
  CGNAT range `100.64.0.0/10`). Own machines, not patient audio.
- The GPU server runs as its own SYSTEM task (`PrataWhisperServer`) — separate
  from the Prata client's task.

### 5.4 Text normalization (an instructive bug)

whisper servers serialize each time segment on its own line in the `text` field.
whisper **sometimes puts a segment boundary in the middle of a long word**. That
determines how the lines should be joined — and it differs per backend:

- **Local whisper.cpp** leaves the segment text **untrimmed**: a genuine word
  boundary carries its own leading space on the next segment; only a boundary
  *inside* a word lacks it. Therefore: **drop the line break without a separator**
  → "Tyd"+"lighet" = "Tydlighet" (correct), and genuine word boundaries keep their
  space.
- **Berget** **trims** each segment line → the line break is then the *only* thing
  separating sentence from sentence. Therefore: **let the line break become a
  space** → otherwise "förluster.Ungdomarna".

The solution is a `Backend.TrimmedSegments` flag (true only for Berget).
A punctuation heuristic was rejected: "få"+"skriva" (should be separated) and
"Tyd"+"lighet" (should be glued) are both letter+letter without a space —
impossible to distinguish without token data.

### 5.5 Degenerate-output guard (`internal/sanity`)

whisper can get stuck in repetition loops (the same phrase over and over). Prata
measures the **gzip compression ratio** of the output and discards degenerate
output before it is typed in. The threshold is calibrated with `cmd/sanity-test`.

---

## 6. The correction dictionary

Two layers:

1. **Baseline** — `go:embed`-ed into the binary at build time
   (`internal/dict/dictionary-corrections.txt`). Always loaded → cannot be
   "silently disabled" because a file is missing.
2. **Per-user override** — `%LOCALAPPDATA%\Prata\dictionary-corrections.txt`
   (created on the first F8 save). The override **adds** or **replaces per key**
   baseline entries (first-match-wins).

- Matching is case-sensitive with **Unicode-aware word boundaries**
  (`[\p{L}\p{N}_]`), literal indexing (no regex), rules in file order.
- **Build-time fold-in (implemented):** a small CLI (`cmd/dict-foldin`) folds
  valuable override entries into the embedded baseline ahead of a release
  (clinic corrections = domain knowledge, not personal preference). Per key it
  adds or replaces in place, preserving comments/blank lines/order; baseline
  rules are never removed; `--dry-run` previews. Run manually by the developer.

---

## 7. Text injection (class-based routing)

This is one of the most safety-sensitive decisions.

- **Routing on the foreground window's class**
  (`GetClassNameW(GetForegroundWindow())`):
  - `Chrome_WidgetWin_1` (the whole Chromium/Electron family + the web-based
    medical record, confirmed to be the same class) and `Notepad++` (its
    Scintilla editor reads the clipboard too slowly for the paste path's restore
    timing — see §15 #3) → **SendInput Unicode**, the whole string in *one* call.
    The clipboard is never touched.
  - All other windows → **clipboard paste** (`CF_UNICODETEXT`, save/restore the
    previous clipboard). Every clipboard write Prata makes — the dictated text
    and the restore of the user's prior clipboard alike — is marked to stay out
    of clipboard history (Win+V), the cloud clipboard, and clipboard monitors, so
    Prata never adds an entry (not even a duplicate of the user's own copy) to
    their Win+V.
- **Invariants (patient safety — must not change):**
  - **Safe default:** all uncertainty (no foreground window, a failed class read,
    an unknown class) → clipboard paste.
  - **No execution fallback:** the path is chosen once. On a SendInput failure it
    *never* falls back to paste — SendInput may already have sent characters, and
    a subsequent paste would double-inject (in a medical record a safety risk).
    Lost text → the user re-dictates (safe).
  - **Allowlist, not denylist:** untested apps default to the proven paste path.
    Nothing gets SendInput until the class has been verified with realistic,
    multi-line text. **Exact** class matching, not prefix.
  - **Dead-target fast-fail:** before focus is restored the target HWND is
    re-validated (`inject.IsWindow`). If the window that was foreground at F1
    press was closed during a slow transcription (e.g. switching from patient
    A's record to patient B's), the result is dropped with a distinct "target
    window gone" diagnostic rather than injected into whatever now holds focus.
- **Why:** (1) in AI chats you should be able to copy a screenshot, dictate, and
  then Ctrl+V the image in — dictation must not touch the clipboard; (2) patient
  confidentiality: medical-record text should not linger in Win+V or sync to the
  cloud clipboard.
- **History:** an early Unicode path dropped key-up events in Chromium/modern
  Notepad → OS autorepeat. It was solved by sending the entire transcription in
  *one* SendInput call. Modern Notepad is deliberately *not* allowlisted (SendInput
  fails there in a length-/content-dependent way).

---

## 8. Hotkeys

- **F1 = PTT dictation, F8 = dictionary quick-fix.** Via `RegisterHotKey`
  (`MOD_NOREPEAT`), not a `WH_KEYBOARD_LL` hook.
- **Why not a hook:** low-level keyboard hooks have a documented failure class
  (silent uninstallation on a >~300 ms callback, invalidation on sleep/resume, and
  AV/EDR suspicion — hooks pattern-match keyloggers). Diktell carries that class
  with a watchdog; Prata does not want to inherit it. A canary
  (`cmd/regkey-test`) proved that F-keys via `RegisterHotKey` *do not* reach the
  focused app (an earlier counter-observation turned out to be a crate artifact).
- **F8 (not F9):** Diktell owns F9 (and Ctrl+Win). By having Prata take F8, both
  apps can run in parallel on the same machine: **F9 = Diktell, F8 = Prata
  quick-fix, F1 = Prata PTT**. It also enables A/B comparison of the two pipelines
  on the same dictation.
- F1's native Help function is consumed system-wide while Prata runs; it is
  restored on exit.

---

## 9. Distribution and lifecycle

### 9.1 One binary, machine-wide install

- The delivery is **a single `prata.exe`** + the USB wrappers
  `Installera-Prata.bat` / `Avinstallera-Prata.bat`. The same binary runs the
  daemon, `--install`, `--uninstall`, `--set-key`.
- `prata.exe --install` (self-elevating via UAC): copies the binary to
  `%ProgramFiles%\Prata\` (read-only for non-admin → the daemon cannot modify its
  own image), registers a **machine-wide Task Scheduler task** (`Prata`, all users
  via SID `S-1-5-32-545`), and starts it in the session via `schtasks /Run`.
- **All writable state is per-user** in `%LOCALAPPDATA%\Prata\` (`apikey.dat`,
  `backend.txt`, dictionary override). **No machine-wide writable data** → no
  `%ProgramData%`, no ACL/multi-session write races.
- `--uninstall` stops the daemon, removes the task + `%ProgramFiles%\Prata`, but
  **leaves the per-user data** (expensive to recreate; symmetry — install never
  created it).

### 9.2 The hard elevation invariant (UIPI)

The daemon runs at **medium IL** (Task Scheduler RunLevel Limited). **Only** the
install action elevates. An elevated daemon would **silently** break SendInput
injection into a non-elevated Webdoc (UIPI blocks low-level input from high IL →
medium IL). Therefore the daemon is never started directly from the elevated
installer; the post-install start happens via `schtasks /Run` (medium IL).

### 9.3 Update — notifying, not self-updating

- The binary is stamped with a version via `-ldflags "-X main.version=…"`.
  `internal/update.Check` queries GitHub's latest-release API and compares
  `vX.Y.Z`. The tray item "Sök efter uppdatering…" reports in a balloon.
- **The upgrade is manual:** re-run `--install` from the *new* binary on USB.
  A `samePath` guard makes the already-installed binary only repair the task (no
  version bump) — an update must come from the USB copy.
- **Why not self-update:** a binary that downloads and runs a replacement of
  itself is exactly the download-and-execute pattern that behavior-based AV/EDR
  flags for an unsigned exe (see §10). It would also add a silent error path to
  the one operation that must not fail on a clinical tool.

---

## 10. The big open problem: unsigned binary vs AV/EDR

- A freshly built, unsigned `prata.exe` is blocked at launch by behavior-based AV
  (confirmed: **Webroot SecureAnywhere**). Symptom: loader rejection ("not a valid
  Win32 application" / "Åtkomst nekad"), no crash logged. Cause: an unknown,
  zero-prevalence binary that registers hotkeys, captures the microphone, and
  synthesizes keystrokes = the textbook "suspicious unknown".
- **`go run` works** (runs from the Go build cache, which Webroot tolerates) → it
  is the verified dev path.
- **Current handling (scale ~12 machines):** USB-copied exes lack the
  Mark-of-the-Web → SmartScreen does not trigger; **per-machine allowlisting**
  replaces public signing. Windows Defender exclusions are set by `--install`
  itself (`Add-MpPreference`); third-party EDR is allowlisted in the console
  (documented in the USB runbook).
- **The lasting fix — Authenticode signing — is prepared but deferred:** a no-op
  hook in `release.yml` (gated on the `CODE_SIGN_PFX` secret) until a cert exists.
  A public EV cert only becomes critical at IT-driven scaling (Intune/SCCM).

---

## 11. Security and privacy (summary)

- **Patient audio is never written to disk** — buffered in memory, discarded after
  the transcription round.
- **Durable logging is metadata-only.** The daemon writes a per-day log
  (`%LOCALAPPDATA%\Prata\logs\prata-YYYY-MM-DD.log`, via `internal/daemonlog`)
  carrying backend ID, timings, character counts, and error strings — **never the
  transcribed text or audio**. It is best-effort: a log that cannot be opened or
  written falls back to stderr and never disrupts dictation (`PRATA_DAEMON_LOG`
  overrides the path for tests).
- **Dictated medical-record text never leaves the clipboard** in Chromium/the
  record (the SendInput path) → neither Win+V nor the cloud clipboard. On the
  paste path (other windows) the dictated text is placed with the
  history/cloud/monitor exclusion markers, so it is kept out of Win+V and the
  cloud clipboard there too. The restore of the user's own prior clipboard is
  marked the same way, so Prata never adds an entry to their Win+V — not even a
  duplicate of their own copy. (Verified live 2026-06-25. The markers do NOT
  break the paste — a slow clipboard *read* did: see §15 #3 and the design log.)
- **The Berget key is DPAPI-encrypted** per user/machine
  (`%LOCALAPPDATA%\Prata\apikey.dat`) — unreadable for other accounts/machines.
  *No* machine-scope DPAPI (it would expose the key to everyone on a shared PC).
- **The clinic's GPU is never exposed over Tailscale.**
- The repo is **private**.

---

## 12. Key design decisions in brief (with rationale)

| Decision | Rationale | Rejected alternative |
|---|---|---|
| Go, not Rust | "See and forget", stdlib covers most of it, small toolchain, self-contained binary | Rust (the same stack as Diktell, but a heavier toolchain) |
| One external dependency (`malgo`) | Minimalism, long-term stability | Libraries for tray/hotkey/clipboard (dropped in favor of direct Win32) |
| Hardcoded endpoints, no config | "See and forget" | `config.toml` |
| F1/F8 via `RegisterHotKey` | Avoids the hook failure class + AV suspicion | `WH_KEYBOARD_LL` (Diktell's path) |
| Class-based hybrid injection | Patient confidentiality + robustness in Chromium/the record | Unconditional SendInput (broke Notepad/the record); denylist (unsafe default) |
| Backend-specific segment joining | Local whisper untrimmed, Berget trimmed | Punctuation heuristic (impossible without token data) |
| Notifying update | Self-update = AV-flagged download-and-execute | Full self-update; silent auto-check at start (deferred) |
| Machine-wide install, per-user state | Shared PCs, switching computers; no machine-wide writable data | `%ProgramData%`-shared dictionary (ACL/race); MSI/Inno/WiX (breaks one-file) |
| Medium IL via Task Scheduler | UIPI: an elevated daemon breaks injection silently | HKLM\Run (no RunLevel control) |
| Signing deferred | USB + allowlisting is enough at ~12 machines | Blocking all delivery on EV-cert lead time |

---

## 13. What works well (verified)

- **Phase 0 validation (2026-05-27):** Berget gives an *identical error pattern* to
  local Diktell (the same model); Diktell's dictionary directly reusable; latency
  mean 2.61 s (min 2.56 / max 2.77) over 5 calls, no cold-start effect.
- **Live dictation verified** on a secondary machine (4 dictations, ~2.1–2.7 s
  round-trip).
- **Hybrid injection** verified clean in Chrome, Cursor, Claude Desktop (multi-line
  text) and in the medical-record system via `cmd/inject-test` (class confirmed).
  Paste path verified live in **Word (`OpusApp`)**, **PowerPoint**, and **classic
  Notepad (`Notepad`)**. **Notepad++ (`Notepad++`)** lost dictation silently on
  the paste path — root cause was the clipboard restore race (`pasteSettleDelay`
  too short), now fixed (50 ms → 400 ms); Notepad++ also routed to SendInput as
  the race-immune path (2026-06-25).
- **Backend failover hint (`internal/failover`) verified end-to-end (2026-06-25):**
  with the active keyless GPU made unreachable, two consecutive dictation failures
  raised the one-time tray balloon and logged `failover hint shown`; a third
  failure raised none (once-per-streak).
- **Win+V hygiene verified (2026-06-25):** dictated text never enters clipboard
  history, and the paste path no longer leaves a duplicate of the user's own copy
  there.
- **Machine-wide install/uninstall/update hardware-verified (2026-06-20):**
  overwrite-while-running (kill the old daemon → retry-copy → re-registration →
  restart), medium-IL injection into a non-elevated window, user data preserved.
- **The word-splitting / Berget-spacing bugs** are solved and live-verified, with
  unit tests on real server output.
- **The F8 popup** restyled (DWM shadow, rounded corners, centered text) and
  live-verified.

---

## 14. Deliberate limitations and non-goals

- **Not cross-platform** (Windows-only; Mac/Linux "maybe later").
- **Not configurable** — change a constant + recompile.
- **Not commercial** — personal + collegial use.
- **Not cloud-first** — local-first with a cloud fallback (Berget) for
  transcription.
- **No framework** — one tool.
- **No AI post-processing** of the text (unlike some dictation tools) — only
  deterministic dictionary correction.

---

## 15. Questions for the reviewer (open threads)

If you are an AI being asked to give input: here are the points where feedback and
ideas are most valuable.

1. **Signing / delivery.** Is "USB + per-machine allowlisting + a deferred
   signtool hook" the right trade-off at ~12 machines? Which signing path (OV/EV,
   Azure Trusted Signing, an internal cert in the EDR) gives the best
   benefit/cost before IT-driven scaling? Is there a way to reduce the AV friction
   *without* a cert?
2. **Injection coverage.** Class-based allowlist with exact
   `Chrome_WidgetWin_1`. Which realistic window classes (Win32-native records,
   Java/Qt apps, RDP/Citrix sessions, virtual desktops) risk ending up on the
   safe-but-clipboard-leaking paste path, and how would you verify new classes
   safely? Is "no execution fallback" right even when SendInput *guaranteed* did
   not manage to send anything?
3. **Patient confidentiality on the paste path.** On non-Chromium windows the
   clipboard is used (save/restore). The dictated text is now marked with the
   clipboard-history / cloud / monitor exclusion formats so it stays out of Win+V
   and the cloud clipboard; a short window where the text sits in the clipboard
   still exists (for the paste itself). Is this the right approach, and is the
   residual window a concern? (Verified live 2026-06-25: text stays out of Win+V.
   Newly found risk — the paste can fail **silently** (no text, no error cue) if
   the target reads the clipboard slower than `pasteSettleDelay`, which then wipes
   the text on restore: Notepad++'s Scintilla did exactly this. Mitigated by
   raising the delay 50 ms → 400 ms, but a fixed delay is a guess — high-latency
   targets (RDP/Citrix clipboard redirection) could still lose the race. Should
   the paste path *confirm* the insert landed rather than trusting
   `SetClipboardData` + Ctrl+V success? Win32 has no clean signal short of
   delayed-rendering — an open design question.)
4. **Multi-session on a shared PC.** `--install`/update kills *everyone else's*
   `prata.exe`. Is "update when no one is dictating" a sustainable operational
   rule, or should the update be session-aware?
5. **Dictionary fold-in.** The interface (`cmd/dict-foldin`) is now built (folds
   per-user override entries into the embedded baseline ahead of a release). Is
   manual fold-in ahead of a release right, or should clinic corrections be
   synchronized in some smarter way across ~12 machines?
6. **Backend robustness.** No silent failover (deliberate). An *explicit*,
   notify-only hint is now implemented (`internal/failover`): after two
   consecutive failures on a local backend the tray suggests a manual switch,
   but nothing switches on its own and patient audio is never auto-routed. Is
   the threshold right, and is a balloon the best surface — or should the hint
   be more (or less) prominent?
7. **The degenerate-output guard.** *Resolved 2026-06-25.* Two complementary
   signals now. The gzip ratio (2.4) catches HIGH-repetition token loops —
   analyzed against a corpus of realistic Swedish clinical phrases: token loops
   score 8–12, while the worst *legitimate* repetitive dictation ("ingen X, ingen
   Y, ..."; bilateral findings) tops out at ~1.8, so there are no false positives
   and the threshold must NOT be lowered. `looksRepeated` now catches the
   LOW-repetition phrase loops the ratio missed (a sentence repeated ≥4x): it
   flags a multi-word phrase repeated back-to-back, which legitimate repetition
   never does (it repeats a *word* across *varied* content). Both backed by
   regression tests. Remaining accepted gaps: a phrase repeated only 2–3x and
   short single-word runs are left alone — ambiguous with legitimate speech,
   short, and visible to the user, so not worth risking a false positive.
8. **Ergonomics.** F1 (PTT) + F8 (quick-fix). Risk of an Fn layer on mini-PC
   keyboards (requires Fn+F1). A better key choice, or is this right?
9. **General ideas.** What is missing for this to be a robust clinical tool for
   years without supervision? Which failure modes have we not thought of?

---

## 16. Technical fact appendix

- **Model:** `KBLab/kb-whisper-large` (GGUF, locally on GPU; the same via Berget).
- **Berget endpoint:** `https://api.berget.ai/v1/audio/transcriptions`.
- **Home GPU endpoint (example):**
  `http://100.87.6.56:8080/v1/audio/transcriptions` (Tailscale IP).
- **Audio:** 16 kHz mono PCM, WASAPI via `malgo`.
- **Hotkeys:** F1 (PTT), F8 (dictionary quick-fix), via `RegisterHotKey` +
  `MOD_NOREPEAT`.
- **Latency:** ~2.6 s mean against Berget (measured); local GPU faster on repeated
  dictation, ~1.85 s model load on a local cold start.
- **Per-user paths:** `%LOCALAPPDATA%\Prata\{apikey.dat, backend.txt,
  dictionary-corrections.txt, logs\prata-YYYY-MM-DD.log}` (the daemon log is
  per-day and metadata-only).
- **Install path:** `%ProgramFiles%\Prata\prata.exe` (read-only), Task Scheduler
  task `Prata` (medium IL, all users).
- **Build:** `go build -ldflags="-s -w -H windowsgui -X main.version=<tag>" -o
  prata.exe ./cmd/prata/`, `CGO_ENABLED=1`.
- **CI:** `gofmt -l .` → `go vet` → `go build` → `go test` on `windows-latest`.
- **Dependency:** `github.com/gen2brain/malgo` (the only external one).
