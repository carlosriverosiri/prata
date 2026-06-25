# Prata — Design Journey

A distilled summary of the key decisions that led to Prata. Not the entire
conversation history — just the decisions and their rationales.

## Background

Diktell is Carlos's existing dictation app written in Rust with local CUDA Whisper. It works
excellently on the main machine (RTX 5070 Ti, 9800X3D) but cannot run on mini-PCs without a GPU.

Carlos frequently switches computers during the workday at the hospital and has grown used to
dictating. Mini-PCs make Diktell unusable — which is the problem Prata solves.

## Decision 1 — Berget AI as the transcription backend

Berget AI hosts exactly the same model that Diktell uses locally (`KBLab/kb-whisper-large`),
via an OpenAI-compatible API. The servers are in Stockholm, data does not leave Sweden, zero retention.

For a physician this is not a "cloud option with a GDPR compromise" — it is probably the
only cloud service that can *legitimately* handle dictated medical text.

Competitor: Whisper Flow (commercial). Berget wins on Swedish quality (KB-Whisper) + GDPR.

## Decision 2 — Windows-only (for now)

Mac + Linux were initially considered as cross-platform targets. Then revised: a cross-platform
abstraction is paid for before it is used. Mac/Linux are "maybe in a year" scenarios. Can be ported
later when Carlos is actually sitting in front of such a machine.

## Decision 3 — Go (not Rust)

Starting point: Carlos's initial impulse was Rust because it is the same stack as Diktell.

Revised after "see and forget" was formulated as the primary design principle:

- **Go 1 compatibility promise** — code written today compiles in five years
- **The standard library covers most of it** — `net/http`, `encoding/json`, `mime/multipart` without dependencies
- **The toolchain is 150 MB** versus Rust + VS Build Tools at 4–6 GB
- **A single self-contained binary** without a runtime
- **AI is genuinely fluent in Go**

Trade-off: Go's audio stack on Windows is less mature (`malgo` is less battle-tested than `cpal`).
Manageable for a simple push-to-talk app.

## Decision 4 — No cross-platform layer, no configuration files

Consequences of being Windows-only and "see and forget":

- No platform/ modules — direct Win32 P/Invoke
- No `config.toml` — hardcoded constants
- API key via a DPAPI-encrypted file, not an environment variable in the long run
- No tray menu — possibly no UI at all

## Decision 5 — Diktell is "finished"

Diktell is considered finished. Only security and crash fixes are allowed. Everything experimental
and new happens in Prata.

Reason: without this discipline Carlos oscillates between improving Diktell and building Prata,
and both projects suffer.

## Decision 6 — Hybrid text injection: class-based routing (2026-05-31)

Status: Accepted and implemented. internal/inject (TypeAuto, IsSendInputSafeClass,
the sendInputSafeClasses allowlist); the production dictation path in cmd/prata calls TypeAuto.

Background: Phase 7 switched injection from KEYEVENTF_UNICODE to clipboard paste
(CF_UNICODETEXT + Ctrl+V), because the Unicode path lost key-up events in Chromium/Electron
and modern Notepad → the OS auto-repeated characters. Clipboard paste is robust but touches the
clipboard on every dictation: a copied screenshot is overwritten, and dictated text lands in
the Win+V history and syncs to the cloud clipboard (patient data leaves the machine).

Driving goals: (1) in AI chats (Claude Desktop, Cursor, Chrome) you should be able to copy a
screenshot, dictate, and then Ctrl+V the image in — dictation must not touch the clipboard;
(2) patient confidentiality: medical-record text must not linger in Win+V or sync to the cloud clipboard.

Decision: route injection on the foreground window's class (GetClassNameW(GetForegroundWindow())):
- Chrome_WidgetWin_1 (the entire Chromium/Electron family plus the web-based medical-records system,
  confirmed to be the same class) → SendInput Unicode. The clipboard is never touched.
- All other windows → clipboard paste (proven path; saves and restores any CF_UNICODETEXT).

What made SendInput usable again: the entire transcription is sent in ONE SendInput call
(Phase 4 batched per rune → autorepeat in Electron). A trailing line break becomes Shift+Enter on
the SendInput path and \r\n on the clipboard path.

Verification (as of 2026-05-31): SendInput verified clean in Chrome, Cursor, and Claude Desktop
with multi-line text, and in the medical-records system via cmd/inject-test (class confirmed
Chrome_WidgetWin_1). The live production-path verification in the medical record — real PTT through
cmd/prata with realistic multi-line text — remains and is the gate before clinical use.

Invariants (patient safety — must not change):
- Safe default: all uncertainty (no foreground window, failed class read, unknown class)
  → clipboard paste.
- No execution fallback: the path is chosen once and called. On a SendInput error it never falls
  back to clipboard paste — SendInput may already have sent characters, and a subsequent paste
  would double-inject (in a medical record a safety risk). Lost text → the user re-dictates (safe).
- Allowlist, not denylist: untested apps default to the proven path. Nothing gets SendInput until
  its class has been verified with realistic, multi-line text.
- Exact class matching, not prefix.

A deliberate engine-level scope: allowlisting Chrome_WidgetWin_1 trusts the entire Chromium/Electron
engine (including Slack, VS Code, Discord, and others), not just the tested apps — justified because
the autorepeat bug is engine-level and SendInput has been verified across several distinct Chromium hosts.

Modern Notepad left out: the "Notepad" class is deliberately not allowlisted — SendInput fails there
in a content-/length-dependent way (a short "test" went through, "rad ett\nrad två" did not). Its own
class automatically routes it to clipboard paste.

Rejected alternatives:
- Unconditional SendInput everywhere — broke Notepad, risked the medical record.
- Full clipboard snapshot/restore of all formats — TOCTOU race; rejected in Diktell too
  (ADR 2026-05-24).
- Denylist — defaults untested apps to the risky path.

Privacy win: in Chromium (including the medical record) dictation never touches the clipboard →
patient text in neither Win+V nor the cloud clipboard. Same outcome as Diktell's ADR 2026-04-21, different mechanism.

Follow-up:
- Extend the allowlist: verify the new class with realistic, multi-line text before adding it.
- The production path does not log the chosen route; route logging is available in cmd/inject-test -mode auto.

Update (2026-06-25): `Notepad++` joined the allowlist — not for an autorepeat
bug, but because its Scintilla editor silently rejected the claim_009 clipboard
exclusion markers on the paste path. See the 2026-06-25 entry below.

## Reuse from Diktell

Directly reusable:

- **`dictionary-corrections.txt`** — the same model produces the same error patterns
- **Hotkey design** — Ctrl+Win for PTT, possibly F9 for the dictionary
- **The text-injection principle** — VK_PACKET / Unicode (applies to the Go implementation too; later revised — see Decision 6)
- **Audio-feedback design** — possibly toned down in Prata

Discarded from Diktell:

- The Rust code itself (50 lines of Cargo + transcribe.rs that validated the Berget API)
- The entire whisper-rs / whisper.cpp layer (replaced by HTTP calls)
- The mode system (already gone after Diktell Phase 4)
- Tokio (Go has its own primitives)

## Validation — Phase 0 done 2026-05-27

1. **API-key sanity check** via Llama 3.3 70B chat completion → ✓
2. **Audio transcription** via curl with an m4a file → ✓ with an error pattern identical to local Diktell
3. **Latency measurement, 5 calls**: mean 2.61 s, min 2.56, max 2.77 s on the main machine
4. **Mini-PC test**: done later when Carlos is at one, judged non-blocking

## Phases ahead

_Original plan from Phase 0. The actual phases after Phase 7 (tray icon, F9, hybrid injection, etc.)
are documented in the CHANGELOG._

| Phase | Content | Estimated number of Cursor sessions |
|-----|----------|----------------------------------|
| 1 | HTTP client + WAV encoding | 1 |
| 2 | Audio capture (malgo) | 1 |
| 3 | Hotkey (WH_KEYBOARD_LL) | 1 |
| 4 | Text injection (SendInput) | 1 |
| 5 | Dictionary corrections | 1 |
| 6 | DPAPI + Task Scheduler | 1–2 |
| 7 | GitHub Actions + install.ps1 | 1 |

## Possible future paths (not decisions, just open doors)

- Mac port for the wife's sermons (kb-whisper-medium-q5_0 on M2)
- Linux port if Carlos eventually moves to Unix
- Eliminate audio feedback if Carlos finds it unnecessary after use


### 2026-06-09: PTT moves from Ctrl+Win (WH_KEYBOARD_LL hook) to F1-hold (RegisterHotKey + reconciliation loop)

**Context:**

Prata's `internal/hotkey` was ported conceptually from Diktell: a custom
`WH_KEYBOARD_LL` hook detecting modifier-only Ctrl+Win, with selective Win
suppression and injected-event filtering. The hook approach carries a
documented failure class from six months of Diktell operation: silent
uninstallation when the callback exceeds Windows' ~300 ms
LowLevelHooksTimeout, invalidation across sleep/resume cycles, and AV/EDR
suspicion (keyboard hooks pattern-match keyloggers — relevant on managed
office machines, Prata's primary deployment target). Diktell absorbed this
class with a watchdog thread, generation counters, and recovery machinery;
Prata has none of that safety net.

Separately, distinct PTT gestures let both apps run in parallel on the
development machine: F1 for Prata, Ctrl+Win for Diktell — direct A/B
benchmarking of the two pipelines on the same dictation.

A Diktell observation from April 2026 (ADR 2026-04-21) — bare F-keys via
RegisterHotKey reaching the focused app anyway — argued against this design.
Kanary-tested 2026-06-09 with direct Win32 calls in Go (`cmd/regkey-test`):
WM_HOTKEY is delivered, MOD_NOREPEAT suppresses repeats, 20 ms
GetAsyncKeyState polling detects release with ms precision, and the focused
app (Notepad, browser) never sees F1/F9. Conclusion: the April observation
was a crate-level (`global-hotkey`) artifact, not an API property.

**Alternatives considered:**

- **Keep Ctrl+Win via WH_KEYBOARD_LL (status quo):** identical muscle memory
  to Diktell, zero migration cost. Rejected: inherits the hook failure class
  without Diktell's recovery machinery; porting that machinery contradicts
  Prata's simplification goal; AV/EDR risk on managed office machines;
  blocks parallel A/B with Diktell.
- **Ctrl+Win+Space via RegisterHotKey (MOD_CONTROL | MOD_WIN + VK_SPACE):**
  hook-free, preserves the modifier feel. Rejected for ergonomics (see
  Diktell ADR 2026-04-22) — kept as documented fallback if F1 proves
  Fn-layered on a mini-PC keyboard.
- **F1-hold via RegisterHotKey + reconciliation loop.** Chosen.

**Decision:**

`internal/hotkey` is rewritten around RegisterHotKey: id 1 = VK_F1
(MOD_NOREPEAT) for PTT, id 2 = VK_F9 (MOD_NOREPEAT) for the dictionary
quick-fix. Press = WM_HOTKEY on the registering thread's message queue;
release = 20 ms GetAsyncKeyState polling, started on press, terminated on
release (zero idle cost). The public Listener interface (`NewListener`,
`SetOnF9`, `Run`, `Stop`) is unchanged — `cmd/prata/main.go` is untouched
except user-facing strings. The PTT key is a single const; an env-var
override is deferred until a real Fn-layer problem appears.

**Consequences:**

- The WH_KEYBOARD_LL failure class (silent unhook, sleep/resume
  invalidation, AV signature) leaves the codebase entirely; no watchdog
  will ever be needed.
- The 300 ms callback constraint disappears — callbacks still return fast
  to keep the message loop responsive, but there is no OS-enforced death
  penalty.
- The Ctrl/Win state machine, Win suppression, and LLKHF_INJECTED filtering
  are deleted — RegisterHotKey cannot self-trigger from Prata's own
  SendInput (Ctrl+C/Ctrl+V/VK_PACKET never match F1/F9).
- F1's native function (Help) is consumed system-wide while Prata runs;
  restored on exit. Accepted.
- Prata and Diktell can run simultaneously for A/B benchmarking.
- Different gestures per app/machine — the machine context is itself
  the cue.
- Mini-PC confirmation pending (Fn layer, EDR) with the same
  `regkey-test.exe`; non-blocking, fallback documented above.

  - F9 coexistence with Diktell on the development machine: Diktell's
  WH_KEYBOARD_LL hook consumes F9 before RegisterHotKey matching, so while
  both apps run, F9 deterministically opens Diktell's dictionary popup and
  Prata's quick-fix never fires. Accepted — Prata's F9 is effectively
  office-scoped. Optional refinement: point PRATA_DICT_PATH at Diktell's
  dictionary file on that machine so both apps share one rule set there.

### 2026-06-15: Webroot SecureAnywhere blocks the freshly-built unsigned binary; `go run` is the verified test path

**Context:**

End-to-end dictation was re-verified on a secondary machine: F1-hold →
WASAPI capture → Berget → injection works (four consecutive dictations,
~2.1–2.7 s round-trip, in line with the documented baseline). The earlier
"context deadline exceeded" failures were the Berget outage of
2026-06-10/11, not an app defect — and they are now audible via the
`PlayError` cue added the same week.

Separately, a real deployment obstacle surfaced. A locally built
`prata.exe` placed in the working tree (`C:\Dev\prata`) refuses to launch:
PowerShell reports "not a valid Win32 application" and `cmd.exe` reports
"Access denied" (Swedish: "Åtkomst nekad"). Both are launch-time loader
rejections, not crashes — the Windows Application event log records no
fault from Prata.

The binary itself is sound. The PE was validated by hand: correct `MZ` and
`PE\0\0` signatures, machine type `0x8664` (amd64), full length with
non-zero data through the final bytes (not truncated/gutted). A copy of the
exe under a *different name* is blocked identically, the file carries no
Mark-of-the-Web (no `Zone.Identifier` stream) and no deny ACL, and
`Get-MpThreatDetection` shows nothing.

Root cause: the active security product is **Webroot SecureAnywhere**
(confirmed via the `root\SecurityCenter2` `AntiVirusProduct` class; Windows
Defender is present but passive, and `Get-MpPreference` fails with
`0x800106ba` because WinDefend is not the primary engine). Webroot's
behavioural/journaling model blocks unknown, unsigned, zero-prevalence
executables until it decides they are safe — and a brand-new Go binary that
registers global hotkeys, captures the microphone, and synthesizes
keystrokes is a textbook "suspicious unknown". This is the concrete
materialization of the AV/EDR-suspicion risk anticipated in the
2026-06-09 ADR (one of the motivations for leaving `WH_KEYBOARD_LL`),
except here Webroot blocks the *executable image at launch*, independent of
the hotkey mechanism.

Key asymmetry: `go run ./cmd/prata/` runs fine, because it executes the
compiled binary from the Go build cache under `%LOCALAPPDATA%\go-build`,
which Webroot tolerates, whereas an unsigned exe in a user dev folder is
blocked. (`go run` resolves the dictionary via `os.Executable()` to the
cache directory, so the dictionary soft-degrades to "disabled" unless
`PRATA_DICT_PATH` is set — that env var is the dev workaround, not a bug.)

**Decision (interim):**

For development and testing on Webroot-managed machines, run via
`go run ./cmd/prata/` with `PRATA_DICT_PATH` pointing at the repo's
`dictionary-corrections.txt`. No code change is warranted — the binary is
correct; the obstacle is host policy.

**Options for production deployment (not yet chosen):**

- **Webroot folder/file allowlist (override):** add `%LOCALAPPDATA%\Prata`
  (or the published exe hash) to Webroot's allow list. Simplest for a
  single known machine; does not scale to "see and forget" distribution and
  may require admin/console access on managed hospital machines.
- **Authenticode code signing:** sign `prata.exe` (and `prata-setkey.exe`)
  with a real certificate. The durable fix — a signed, reputable publisher
  identity is what lets Webroot (and SmartScreen, and Defender) trust the
  binary without per-machine overrides. Cost: a code-signing certificate
  (OV/EV) and a signing step in `release.yml`. This is the path that aligns
  with the installer-based "see and forget" goal.
- **Reputation seasoning:** unsigned binaries eventually gain prevalence/age
  and unblock themselves. Unreliable and unacceptable for a clinical tool —
  rejected.

**Consequences:**

- The dictation pipeline is verified working on this machine; the only
  blocker to running the *installed* build is host AV policy, not Prata.
- Code signing is now the leading candidate for the next deployment-
  hardening task and should be folded into the release workflow before
  wider rollout.

### 2026-06-15: Dictionary quick-fix hotkey moved F9 → F8 (Diktell owns F9)

**Context:**

The 2026-06-09 ADR accepted that on a machine running both Diktell and
Prata, Diktell's `WH_KEYBOARD_LL` hook consumes F9 before Prata's
`RegisterHotKey` can match, so Prata's F9 quick-fix never fires there —
"Prata's F9 is effectively office-scoped". In practice the user drives
Diktell with F9 as their primary dictation tool, so the collision is not
theoretical: wherever Diktell runs, Prata's quick-fix is dead on F9.

**Decision:**

Move Prata's dictionary quick-fix from **F9** to **F8**. F8 is unclaimed
by Diktell (which owns F9 and Ctrl+Win) and by Prata's own PTT (F1), so the
two apps get one key each: **F9 = Diktell, F8 = Prata quick-fix, F1 = Prata
PTT**. This supersedes the "office-scoped, accepted" disposition above — the
quick-fix is now expected to work alongside Diktell rather than yield to it.

The change is `vkF8 = 0x77 (VK_F8)` in `internal/hotkey/listener.go`, with
the public API renamed `SetOnF9` → `SetOnF8` and all internal identifiers
(`onF8`, `f8Held`, `f8Busy`, `f8Worker`) and comments following. The test
harness `cmd/f9-test` is now `cmd/f8-test`. `cmd/regkey-test` is left as the
dated F1/F9 RegisterHotKey canary from the 2026-06-09 migration — it records
that diagnostic as run, and is unrelated to the production key choice. The
quick-fix never shipped to users on F9, so no released behavior changes.

**Consequences:**

- On the mini-PC, F8 may sit behind the keyboard's Fn layer (same caveat as
  F1); verified at the next on-device test. `RegisterHotKey` binds the base
  VK_F8, so an Fn-layered keyboard would require Fn+F8.
- The earlier optional refinement (point `PRATA_DICT_PATH` at Diktell's
  dictionary so both apps share one rule set) still stands and is now more
  useful, since Prata's quick-fix can actually run on the shared machine.

### 2026-06-15: Update mechanism — notify-only check, not self-update

**Context:**

Prata installs and upgrades through `install.ps1` (GitHub Releases →
`%LOCALAPPDATA%\Prata`, dictionary preserved). Upgrading therefore already
needs no USB stick — re-running the one-liner does it — but nothing tells the
user a new version exists, and the binary carried no version string to
compare against. The question was whether to add an in-app updater, and if so
how much it should do. Cadence is roughly annual; the audience is a handful
of clinical machines; output lands in a patient journal.

**Decision:**

Add a **notify-only** update check, not a self-updater. Three pieces:

1. The binary is stamped with a version via `-ldflags "-X main.version=…"`
   (release workflow uses the git tag; `install.ps1 -Local` uses
   `git describe`; plain `go build`/`go run` stays `"dev"`).
2. `internal/update.Check` queries GitHub's latest-release API and compares
   numeric `vX.Y.Z` versions.
3. A tray item, **Sök efter uppdatering…**, runs the check off the UI thread
   and reports the result in a tray balloon. The actual upgrade is still manual
   (re-running the installer — today `install.ps1`; transitioning to
   `prata.exe --install` on USB, see installer-ADR 2026-06-16).

**Alternatives considered:**

- **Full self-update** (download new exe, rename the running one via
  `MoveFileEx`, write the replacement, restart). Rejected: a binary that
  downloads and executes a replacement of itself is precisely the
  download-and-execute pattern behavioural AV/EDR flags — and the
  unsigned-binary ADR above already documents Webroot blocking Prata at
  launch. Self-update would worsen that surface, add a silent-failure path
  into the one operation that must not go wrong on a clinical tool, and buy
  little for an annual cadence.
- **Silent auto-check on startup.** Reasonable, and easy to add later (the
  `update.Check` + `tray.Notify` plumbing already supports it). Deferred:
  for an annual cadence a constantly-polling background check is overkill,
  and an explicit user action keeps control with the user.
- **Do nothing in the app, document re-running the installer.** Honest
  baseline, but leaves the user with no signal that an update exists.

**Consequences:**

- Once code signing lands (the leading deployment-hardening candidate from
  the ADR above), the notify-only stance can be revisited — a signed binary
  removes the main argument against self-update.
- The check needs network and GitHub's unauthenticated API (60 req/h per IP);
  fine for a manual, occasional click. Failures degrade to a "could not
  check" balloon, never a crash.

### 2026-06-16: Single-file machine-wide installation (Branch A: USB + local admin), signing prepared but deferred

**Status:** Accepted. **Phase 0 answered (2026-06-16): Branch A in small-scale form** —
~10–12 clinic computers, logged-in clinicians have local admin (UAC works),
distribution via USB stick manually per machine (not Intune now). The design remains
prepared for IT-driven distribution (Intune/SCCM) later. Phases 2–4 are clean,
unsigned refactors that can run right away; Phase 5+ are unblocked — signing is
no longer a gate (see decision 1).

**Background**

Prata was originally installed per user: `install.ps1` copies the
binaries to `%LOCALAPPDATA%\Prata` and registers a Task Scheduler task
`"Prata"` for a single user. **Phase 5a (2026-06-17)** added
machine-wide install via `prata.exe --install` → `%ProgramFiles%\Prata\` + a
logon task for all users. Both paths exist in parallel until Phase 7
(which removes `install.ps1` and legacy files). At a clinic with shared PCs, where
users switch computers, the per-user model is wrong — every user has to
reinstall, and separate files (`prata-setkey.exe`, `dictionary-corrections.txt`,
`install.ps1`) make the package fragile. The goal: **one file** that installs everything, and
**one installation that applies to all users** on the machine.

An architectural consequence of the decisions below (per-user key + per-user
dictionary): there is **no machine-wide writable data**. Therefore **no
`%ProgramData%`** is needed — the binary sits read-only in `%ProgramFiles%\Prata`, all
writable state is per-user in `%LOCALAPPDATA%\Prata`. This eliminates the entire
ACL/multisession-write problem.

**Phase 0 — delivery branch (answered 2026-06-16: Branch A, small-scale)**

Who runs the elevated installation determines the outer conditions, not the
`--install` logic (which is **identical** in both branches):

- **Branch A — the clinician has local admin. ← CHOSEN NOW.** Self-elevating binary:
  double-click → `ShellExecute "runas"` → UAC → machine-wide install. Scale: ~10–12
  clinic computers, distribution via **USB stick**, manually per machine. No
  public cert is required at this scale (see below + decision 1).
- **Branch B — no clinician admin (future scaling, not now).** IT runs the same
  `--install` once per machine, elevated, via their tooling (SCCM/Intune/GPO),
  with **IT allowlisting** (hash/path, or IT's own internal cert in
  EDR/AppLocker) instead of public signing. The design remains prepared for
  this, but it is not the goal now.

**Why signing can be deferred now.** USB-copied exe files normally lack the
Mark-of-the-Web → SmartScreen does not trigger. At this scale (~12 machines) +
local admin + USB, **per-machine allowlisting** (decision 9) replaces public
signing entirely. A public EV cert (which usually requires a registered organization)
only becomes relevant when scaling to IT-driven distribution.

**Decisions**

1. **Signing = a prepared, deferred step (Phase 1) — not a gate.** At the
   chosen scale (Branch A, USB, local admin) **no public EV cert is needed to
   ship**: USB binaries lack the Mark-of-the-Web (no SmartScreen) and
   per-machine allowlisting (decision 9) covers AV/EDR. Signing is therefore
   implemented as a **prepared hook in `release.yml` that is a no-op until a cert
   exists**. This reassesses (but does not tear down) the update ADR (2026-06-15):
   self-update stays off until a trusted publisher identity exists; the
   runnable distribution now is USB + per-machine allowlisting. A public cert
   only becomes a requirement at IT-driven scaling (Branch B).
2. **Installation location.** Binary in `%ProgramFiles%\Prata` (read-only for
   non-admins — the daemon cannot modify its own image). All writable state
   per-user in `%LOCALAPPDATA%\Prata`. **No `%ProgramData%`.**
3. **Berget key.** Keep per-user, user-scope DPAPI (status quo) via
   `prata --set-key`. **No** `CRYPTPROTECT_LOCAL_MACHINE` — that would
   expose the key to everyone on a shared PC. Not required for Jobb/Hemma.
4. **Dictionary.** `go:embed` of a shared baseline + per-user override in
   `%LOCALAPPDATA%\Prata`. Sidesteps both write permission in ProgramFiles and
   the multisession-write race against a shared file. F8 writes to the override.
   `resolvePath` (dict.go) **and** `loadDict` (main.go) currently compute the path
   independently and **must be changed together**. A **build-time routine** is designed
   to fold valuable override additions into the baseline at release time
   (clinic corrections are domain knowledge, not personal preference);
   the implementation may be phased, but the interface is designed.
5. **Default backend Jobb.** The `loadBackendPref` default was changed Berget → Jobb
   (implemented in Phase 4); a per-user `backend.txt` overrides it. Otherwise a
   new user hits Berget-without-key on F1 → error tone.
6. **Autostart.** One machine-wide Task Scheduler task, trigger AtLogon for
   **all** users (Principal `BUILTIN\Users` with implicit interactive logon,
   RunLevel Limited), starts Prata in each user's session. **Task Scheduler
   > HKLM\Run** is justified by RunLevel control (medium IL, see invariant),
   start conditions, and robustness; HKLM\Run is mentioned as a simpler fallback.
7. **Migration (spans all profiles; data only for the installing user).**
   `--install` detects and cleans up an earlier per-user install. Cleanup
   of old autostarts **must span all user profiles** — admin can
   enumerate and remove old `"Prata"` tasks and `%LOCALAPPDATA%` exe copies
   across all users. But **per-user DATA cannot be migrated across
   users:** `apikey.dat` is user-scope DPAPI and is unreadable to the
   installer. Only the **installing user's** data is migrated (Branch
   A: preserve `apikey.dat`/`backend.txt`, migrate any old dictionary →
   override). Other users get **fresh defaults on first run** —
   acceptable, since Jobb requires no key and the dictionary baseline is
   embedded. `--uninstall` removes the ProgramFiles folder + the machine-wide
   task. (Does **not** touch `PrataWhisperServer` — that is the GPU server's task, a
   different thing.)
8. **One (1) binary.** The deliverable is **a single binary** with the Jobb default built in +
   per-user `backend.txt` override — **not** separate named builds per
   site or per branch. The same `prata.exe` runs the daemon, `--install`,
   `--uninstall`, and `--set-key`.
9. **AV/EDR allowlisting (part of the install routine).** The design log documents
   that Webroot blocks unsigned binaries at launch (ADR 2026-06-15). Two
   paths are designed, so the installation works regardless of which protection the machine runs
   (which AV is confirmed with IT):
   - **Windows Defender:** the elevated `--install` adds the exclusion itself —
     `Add-MpPreference -ExclusionPath "%ProgramFiles%\Prata"` — under the
     existing UAC elevation, no extra prompt.
   - **Third-party EDR (Webroot and the like):** the exclusion cannot be set
     programmatically; it is done in the EDR console and documented as a step in
     the **USB runbook**.

**Invariants (patient safety — must not change)**

- **UIPI / medium IL.** The daemon runs at medium IL (Task Scheduler RunLevel
  Limited). **Only** the install action elevates. An elevated daemon breaks
  SendInput injection into a non-elevated Webdoc **silently** — a hard invariant.
- **windowsgui = no console.** All installer/update feedback via `MessageBoxW`
  (incl. "UAC avbruten", errors, done). `--set-key` as a **pure argument form**
  (`--set-key <key>`), no interactive prompt.
- **The single-instance mutex is already session-bound** (an unprefixed name in
  `single.Acquire` = `Local\`). Verified and documented — not changed. This
  is what gives Prata one instance *per session* on a shared PC.

**Rejected alternatives**

- **HKLM\Run** instead of Task Scheduler — no RunLevel/condition control,
  can be disabled per user in Task Manager. Kept only as an
  emergency fallback.
- **Machine-scope DPAPI** for the Berget key — exposes the secret to everyone on
  the machine; unnecessary since Berget is deprioritized.
- **A shared writable dictionary in `%ProgramData%`** — requires widening ACLs + atomic
  write + cross-process locking against the multisession race; a lot of machinery against
  minimalism. The per-user override gives the same benefit without the race.
- **MSI/Inno/NSIS/WiX** — external packaging tools break the
  single-file/stdlib-only principle.
- **Separate named builds per site/branch** — break the single-binary principle;
  replaced by the Jobb default + per-user override (decisions 5 and 8).

**Consequences**

- **Signing is not on the critical path now.** With Branch A/USB/allowlisting, Phases
  2–4 are built unsigned and Phase 5+ are unblocked. A cert becomes critical-path only at
  IT-driven scaling (Branch B). The EV-cert lead time stalls no codeable work.
- The `%ProgramFiles%` placement means a running exe cannot overwrite
  itself → an update (manual USB re-run) must stop the task + all instances,
  copy, re-register, restart (Phase 6). No download — not a network
  self-update.
- **Post-install start is interactive-only.** "Start in the current session after
  install" applies to the chosen **Branch A** (interactive UAC elevation) — works now.
  Should `--install` later run as SYSTEM via SCCM (**Branch B**), there is no
  interactive session, and Prata then starts only at the next logon via the task.
  The start step is guarded so it does not error under the SYSTEM context (expected,
  non-fatal).
- **Multisession:** the machine-wide task starts Prata in each session at
  logon. Already logged-in sessions update/start only at the next
  logon.
- **Blast radius:** `release.yml` (today ships `prata.exe`, `prata-setkey.exe`,
  `dictionary-corrections.txt`, `install.ps1`), the update ADR, and the tray string
  ("Kör om installationskommandot") must be updated in step (Phases 1/6/7).
- "See and forget" and minimalism are preserved by keeping the install/update code path
  **strictly separate** from the daemon hot path — the runtime stays minimal even
  when the binary gains an install mode.

**Phased plan (summary)**

- **Phase 0** — ANSWERED (2026-06-16): Branch A, ~12 machines, USB, local admin
  in place. Unblocked.
- **Phase 1** — Signtool hook in `release.yml` (**deferred, no-op until a cert exists**)
  + USB install routine/runbook with AV allowlisting (Defender via
  `Add-MpPreference` in `--install`; third-party EDR in the console). **No longer a
  gate** for Phase 5+.
- **Phase 2** — `--set-key` as a subcommand (pure argument form) + `MessageBoxW` helper.
  ✅ Implemented.
- **Phase 3** — Dictionary: `go:embed` baseline + per-user override. ✅
  Implemented.
- **Phase 4** — Default backend Berget → Jobb. ✅ Implemented.
- **Phase 5a** — `--install` happy path (clean machine, self-elevating). ✅
  Implemented (2026-06-17).
- **Phase 5b** — Migrating an old per-user install. In the elevated
  `installElevated`, **before** copyFile: (1) `terminateOtherInstances` kills every
  running `prata.exe` except its own PID (`CreateToolhelp32Snapshot` →
  `Process32FirstW/NextW`, `OpenProcess(PROCESS_TERMINATE)` + `TerminateProcess`)
  — clearing both the session-bound single-instance mutex and any file lock on the
  target binary; (2) `copyFileWithRetry` (10 × 200 ms) tolerates a transient lock after
  the termination and aborts the install with an error box if the lock persists (never a silent
  continuation); (3) after `schtasks /Run`, `cleanupLegacyUserBinaries` removes
  **only** `prata.exe` + `prata-setkey.exe` in each `C:\Users\*\AppData\Local\
  Prata\` (best-effort; user data preserved). The task XML, RunLevel, and the medium-IL
  start are **untouched** → the invariant intact. Self-PID exclusion is mandatory.
  Status: **✅ hardware-verified 2026-06-20** — dirty-state smoke test:
  `terminateOtherInstances` killed the old daemon (PID 82948),
  `copyFileWithRetry` absorbed the file lock (attempt 1 → 2), the new daemon (PID 99272)
  from `%ProgramFiles%` remained, F1 injection into a non-elevated window worked
  (medium IL), and user data was preserved. Known limitation:
  the binary you run `--install` *from* cannot be deleted by the cleanup (in use) →
  logged, harmless since the machine-wide binary in `%ProgramFiles%` is authoritative.
- **Phase 5c** — `--uninstall` (mirrors `--install`). `Uninstall()` self-elevates
  via `relaunchElevated("--uninstall")` (the helper was parameterized; `Run` passes
  `"--install"` — behavior-preserving). Elevated, in order: (1)
  `terminateOtherInstances` stops the daemon + unlocks the binary (self-PID
  excluded); (2) `schtasks /Delete /TN "Prata" /F`, classified **locale-safely**
  via the post-state `schtasks /Query` (`taskAbsent`) — never error-string parsing; (3)
  `removeInstallDirWithRetry` removes `%ProgramFiles%\Prata` (10 × 200 ms against the
  transient image-section lock after termination). **Per-user data in
  `%LOCALAPPDATA%\Prata` is left untouched** (API key, dictionary, backend choice — expensive
  to recreate, and `--install` never created them; symmetry). `PrataWhisperServer`
  (the whisper server's SYSTEM task) is never touched — only `"Prata"` is addressed. Best-
  effort teardown: "already gone" = success, a genuine remnant → a soft warning
  MessageBox (no crash). **Running-binary limitation (Option A):** if
  `--uninstall` is run from the *installed* `%ProgramFiles%\Prata\prata.exe`, the folder
  cannot be fully emptied (Windows does not allow deleting a running `.exe`); `runningFromInstallDir`
  detects this and shows "kör --uninstall från USB-/originalkopian". A more robust
  temp-copy relaunch (Option B) is not built now. Status: **✅ hardware-verified
  2026-06-20** — smoke test (`--uninstall` from an external `C:\Dev` copy, not the
  installed binary): `terminateOtherInstances` killed the running daemon
  (PID 99272), `schtasks /Query` confirmed the task gone, `%ProgramFiles%\Prata`
  was removed, and `%LOCALAPPDATA%\Prata` was left intact (6 files remaining: apikey.dat,
  backend.txt, the dictionary + `.default` + `.bak`, prata.exe.bak). Since uninstall
  ran outside installDir, the success path applied, not the Option-A warning.
- **Phase 6** — Update flow. **The mechanics already exist in `--install`** and require
  no new code: `installElevated` runs `terminateOtherInstances` → `copyFileWithRetry`
  → `registerTask` (`schtasks /Create /F`, so the XML is regenerated and a changed
  task definition is applied) → `runTask` (restart in the session, medium IL). Proven by
  the 5b smoke test (overwrite-while-running: killed PID 82948 → retry-copy → re-registration
  → restart). An update = **re-run `--install` from the NEW binary on the USB stick**;
  the `samePath(src,dst)` guard means that if you run the already-installed
  `%ProgramFiles%\Prata\prata.exe --install` the copy is skipped (only task repair +
  restart, no version bump) — an update must therefore happen from the USB copy, not
  from the installed binary (same model as uninstall Option A). Phase 6 is therefore
  **just text + docs**: the tray notice (`res.Newer`) now points to the USB re-run instead
  of the vague "installationskommandot", and the stale `install.ps1` comments in
  `internal/update` + `main.go` are corrected (the installer is a separate process that kills the
  daemon *before* the copy → no self-overwrite, no rename dance). The version check
  (`update.Check`) is unchanged. **Multi-session caveat:** `terminateOtherInstances`
  kills *all* other `prata.exe` by name (self-excluded), so on a shared clinic computer
  an `--install` also terminates another logged-in user's daemon — update when no one
  is dictating (full USB-runbook line = Phase 7). Status: **✅ Verified 2026-06-20** (gates + diff review; a pure
  string change, no new mechanics to hardware-test — the update mechanism is
  already 5b-verified).
- **Phase 7** — Packaging + legacy cleanup. `release.yml` now ships **ONE binary**
  (`prata.exe`) + the USB wrappers `Installera-Prata.bat`/`Avinstallera-Prata.bat`;
  `prata-setkey.exe`, the root `dictionary-corrections.txt`, and `install.ps1` are dropped from
  the release bundle. Removed from the repo: `install.ps1`, `cmd/prata-setkey/` (folded into
  `prata --set-key` since Phase 2), and the root duplicate of the dictionary (the embed source
  `internal/dict/dictionary-corrections.txt` is the single truth). Signing is a
  **prepared, deferred hook** in `release.yml` (gated on the `CODE_SIGN_PFX` secret,
  no-op without a cert). The `logf` path is now env-controllable via `PRATA_INSTALL_LOG`
  (test isolation + dev). Docs synced (README, PRATA-MASTER, GPU-SERVER, CHANGELOG).
  The legacy-cleanup comment in `installer.go` (`cleanupLegacyUserBinaries`) is kept —
  it cleans up already-deployed `prata-setkey.exe` on clinic disks, not the current method.
  Status: **✅ 2026-06-20** — code + docs done; `.bat` hardware smoke-tested (launch +
  å/ö + pause safety net); `release.yml` review-verified (full validation on the first `v*` tag).

### 2026-06-16: Dictionary — embedded baseline + per-user override (Phase 3 implemented)

**Status:** Implemented. Carries out Phase 3 of the installer plan above.

**What was done**

- The baseline (`dictionary-corrections.txt`) is now `go:embed`-ed into the binary as an
  **immutable** layer (`internal/dict/dictionary-corrections.txt`, a byte-identical
  copy of the root file). It always loads — the dictionary can no longer "silently
  disable" because a file is missing next to the exe.
- **The override** is layered on top of the baseline (`dict.LoadDefault` → `loadLayered` →
  `mergeRules`): an override entry **adds** or **replaces per key**
  a baseline entry. Replacement happens on the first (and only firing) occurrence, so
  the override wins under first-match-wins.
- **Path resolution unified.** `resolvePath` (dict) returns the OVERRIDE path:
  `PRATA_DICT_PATH` (dev) → otherwise `%LOCALAPPDATA%\Prata\dictionary-corrections.txt`.
  `cmd/prata`'s `loadDict` delegates to `dict.LoadDefault` — they no longer
  compute the path independently, so the daemon/`Save`/`Reload` always agree.
- **F9/`dict.Save` writes ONLY to the override file** (creating
  `%LOCALAPPDATA%\Prata` if needed), never the baseline, never next to the exe.
- **Side effect resolved:** the `go run` quirk (`os.Executable` → build cache → the dictionary
  was disabled) is gone because the baseline is always embedded.
- **Transient duplicate:** the root `dictionary-corrections.txt` was **removed 2026-06-20
  (Phase 7)** — it was byte-identical to the embed source and was previously shipped by
  `release.yml`/`install.ps1`; harmless to remove since the runtime never read
  next to the exe. `internal/dict/dictionary-corrections.txt` is the only baseline source.

**Build-time fold-in — IMPLEMENTED 2026-06-25 (`cmd/dict-foldin`)**

Valuable override entries should be able to be "folded into" the embedded baseline ahead of
a release, so they ship to all users. The contract:

- **Form:** a small Go CLI, `cmd/dict-foldin` (stdlib-only, no
  daemon coupling), run **manually by the developer** before a release build —
  not in the daemon hot path, not automatically in CI.
- **Invocation:**
  `dict-foldin --override <path> [--baseline internal/dict/dictionary-corrections.txt] [--dry-run]`
  - `--override` (required): the source to fold in (typically a user's
    `%LOCALAPPDATA%\Prata\dictionary-corrections.txt`).
  - `--baseline` (default `internal/dict/dictionary-corrections.txt`): the target
    embedded at the next build — the **only** baseline source.
  - `--dry-run`: print the diff (would-be-added / would-be-replaced), write
    nothing.
- **Semantics:** identical to the runtime `mergeRules` — per key, add or
  replace in place; preserve comments, blank lines, and order in the baseline; skip
  empty/identity rules (like `Save`). **Never removes** baseline rules.
- **Output:** an updated baseline file (idempotent) + a short report
  (added/replaced/skipped). Exit ≠ 0 on a parse error in any file.
- **Invariant:** the baseline remains the only embedded source; the tool
  edits only that file, never touches the user's override.

Implemented 2026-06-25 exactly to this contract — see the dated entry at the end
of this log for the one new decision (where the merge logic lives).

### 2026-06-17: `--install` machine-wide, self-elevating — happy path (Phase 5a implemented)

**Status:** Implemented (clean install, no prior Prata). Carries out Phase 5a.
Deferred: migration of a per-user install (5b), `--uninstall` (5c),
overwrite-while-running/update (6), Webroot allowlisting + `Installera-Prata.bat`
(7).

**What was done**

- A new package `internal/installer` (raw `syscall`, no new dependency — keeps the
  stdlib-only principle). `dispatchSubcommand` got a `case "--install"`.
  No-args = daemon is unchanged.
- **Elevation:** `isElevated` (`OpenProcessToken` + `GetTokenInformation`
  `TokenElevation`). Not elevated → `ShellExecuteW` verb `runas` params
  `--install`, exit; return ≤ 32 (UAC denied) → Swedish MessageBox. Already elevated
  (relaunched child / Branch B) → continue. The `isElevated` check prevents a loop.
- **Copying:** `os.Executable()` → `%ProgramFiles%\Prata\prata.exe`.
  source==dest is compared on a normalized, case-insensitive path → skip the copy
  but re-register the task (idempotent repair). Locked/unwritable target → error,
  no silent continuation.
- **Machine-wide task** via generated XML (UTF-16LE + BOM, `schtasks /Create /XML
  … /F`): `LogonTrigger` without `UserId`, `GroupId` = `S-1-5-32-545`,
  `RunLevel` `LeastPrivilege`, `MultipleInstancesPolicy` `Parallel`,
  `ExecutionTimeLimit` `PT0S`. **No explicit `LogonType`:** for a
  group principal an interactive logon is implicit, and the v1.2 schema requires
  `LogonType` *before* `GroupId` — an explicit value produced the `schtasks` error
  "unexpected node" (fixed 2026-06-20; the element order is now guarded by a
  unit test). The medium-IL guarantee lies in `RunLevel`, not in `LogonType`.
- **Post-install start:** `schtasks /Run /TN "Prata"` (best-effort, medium IL).
  Fails → non-fatal ("next logon").

**Decisions worth noting**

- **GroupId via the SID `S-1-5-32-545`, not the literal "Users"/"BUILTIN\\Users".**
  The group's *display name* is localized (Swedish Windows: "Användare"); the
  well-known SID is language-independent and always resolvable. The correct technique despite
  the prompt writing "BUILTIN\\Users".
- **`MultipleInstancesPolicy` = `Parallel`** (not `IgnoreNew`): one instance per
  session in multisession; the session mutex prevents duplicates within a session.
  `IgnoreNew` could have blocked other sessions' daemons.
- **`AllowStartOnDemand` = true** is required for `schtasks /Run` to work.

**Known risk (verified on hardware)**

- `schtasks /Run` on a **group-principal task (implicit interactive logon)** should run the
  daemon in the logged-in user's session at medium IL regardless of the installer's
  HIGH IL (the Scheduler service creates the process per the principal's RunLevel).
  This is the point that can act up on some Windows versions. If `/Run` does not
  start in-session: the non-fatal "next logon" path covers it, and
  a documented **`explorer.exe <exe>` trick** (Explorer runs at medium IL →
  the child inherits medium IL) exists as an emergency fallback — **not coded now**.

**Manual smoke-test protocol (run on a CLEAN, Webroot-allowlisted machine)**

The built unsigned exe is blocked under Webroot and `go run` cannot
meaningfully test `--install`, so this is deferred until an allowlisted
machine is available. Steps:

1. Double-click the install path (`prata.exe --install`) → the UAC prompt appears.
2. Approve UAC → the binary lands in `%ProgramFiles%\Prata\prata.exe`.
3. `schtasks /Query /TN Prata /XML` → confirm `RunLevel` `LeastPrivilege`,
   `GroupId` `S-1-5-32-545`, LogonTrigger without `UserId`.
4. The daemon started in the session → verify **medium IL** (Process Explorer:
   Integrity = Medium), not High.
5. F1 → dictate into a non-elevated Webdoc → text is injected (the UIPI invariant
   holds).
6. Run `--install` again and cancel UAC → a clean Swedish MessageBox, no crash.

**Diagnostics:** every installation step and error is written, timestamped, to
`%TEMP%\prata-install.log` (shared by the non-elevated parent and the
elevated child). Read it if any step above fails — it captures the error even
when the installer only shows a modal MessageBox.

**Verified 2026-06-20** on the development/home PC (not Webroot): steps 1–5
confirmed — the binary copied to `%ProgramFiles%\Prata`, the task registered,
`RunLevel` = `Limited` (via `Get-ScheduledTask`), the daemon runs from
`%ProgramFiles%\Prata\prata.exe` without arguments, and F1 injected into a non-elevated
window — which in itself proves medium IL (high IL would UIPI-block
the injection, so step 4 needs no separate Process Explorer check).
The F1 test was run in an ordinary non-elevated window, not specifically Webdoc;
the mechanism is identical but an explicit Webdoc confirmation remains. Left to
verify: the UAC-cancel box (step 6) and a run on a clean clinic machine.

### 2026-06-21: Incorrect word splits (särskrivningar) in dictation — the cause is the client's segment joining, not the whisper version

**Status:** Fixed and live-verified. The fix is in
`internal/transcribe/client.go` (`normalizeTranscript`): the whisper segments are
now joined **without** a separator, the way Diktell does, instead of turning every line break
into a space. The user has live-verified (Swedish dictation via
Prata against the **Rngv GPU-server**) that the word splits are gone. `gofmt`, `go vet`,
and `go test` are green, including a new unit test on the real
server output.

**Background / symptom**

Dictation via Prata produced spaces in the middle of Swedish words (word splits) — e.g.
"tydlighet" → "tyd lighet", "kärnenergifrågan" → "kärnenergifrå gan", "enligt" →
"en ligt". Short words were usually unharmed; it affected long compound words. The fault
is in how Prata assembles the transcript — not in the model and not in
the whisper version.

**Investigation — rejected hypothesis (whisper version)**

The first hypothesis was a detokenization regression in whisper.cpp after the tag
`v1.8.6` (the source on the Rngv server was at `v1.8.6-80-g0ec08451`, 80 commits after
the release). The server was rebuilt pinned to `v1.8.6` and run live —
**the word splits persisted**. A deterministic A/B settled it: the same audio
(a recorded WAV) was sent against both `v1.8.6` and HEAD+80 with
`curl … response_format=verbose_json` and produced **byte-identical** output, including
exactly the same break in the middle of the word. The bug is therefore version-independent; the pinning
was a red herring and is not needed. The runbook's HEAD clone (PRATA-GPU-SERVER.md) is
therefore unchanged.

**Diagnosis — actual cause**

`verbose_json` on real Swedish speech revealed the mechanism. whisper sometimes places a
**segment boundary in the middle of a long word**:

```
"text": " … kärnenergifrågan. Tyd\nlighet, små, enligt, akromeoplastik.\n"
segment 0 ends:   "… kärnenergifrågan. Tyd"   (no trailing space)
segment 1 starts: "lighet, små, enligt, …"     (no leading space)
```

The server serializes each segment on its own line in the `text` field (`"Tyd\nlighet"`).
Prata's `normalizeTranscript` ran `strings.Join(strings.Fields(s), " ")`, which
treats the line break like any whitespace and turns it into a
space → "Tyd lighet". At a **real** word boundary the next segment carries its own
leading space (cf.: `"… country can\n do for you …"`), but at a boundary
**inside** a word it is missing. Diktell is not affected because it concatenates
the segments without a separator — exactly what `normalizeTranscript` claimed to do but
did not.

**Decision**

Remove the segments' line breaks without replacing them with spaces (concatenate, like
Diktell), then collapse remaining whitespace. A real word boundary already has
its leading space on the next segment; a boundary in the middle of a word does not — so
dropping the line break gives the right result in both cases. A unit test runs the exact
recorded server output and requires "Tydlighet", not "Tyd lighet".

**Alternatives considered**

- **Pin/rebuild whisper.cpp to `v1.8.6`** (the original hypothesis). Rejected:
  the A/B shows the bug is version-independent, so pinning changes nothing.
- **Patch the whisper.cpp server** (stop breaking in the middle of words / stop inserting line breaks).
  Rejected: fork maintenance, and the joining belongs on the client — Diktell
  proves that client-side concatenation is correct.
- **Correct in Prata's dictionary.** Rejected: masks the fault and does not generalize
  (arbitrary words are affected).

**Consequences**

- The fix is a small change in `normalizeTranscript` + tests; verified locally and
  in live dictation.
- The whisper.cpp server's version is irrelevant to this bug; no pinning is needed
  and nothing in PRATA-GPU-SERVER.md changes. The v1.8.6 rebuild done during
  the investigation is harmless but unnecessary.
- Applies to all backends that serialize segments per line (the whisper.cpp server and
  Berget alike). **Corrected later the same day — see below: Berget trims its
  segment lines, so the joining must be backend-specific.**
- The spelling variation io→eo ("akromeoplastik") is an ASR recognition error from
  the model and is not affected by this fix.

### 2026-06-21: The word-split fix regressed Berget — segment joining is backend-specific

**Status:** Fixed. `internal/transcribe/client.go`: `normalizeTranscript`
now takes a `trimmedSegments` flag, and `Backend` has the field `TrimmedSegments`
(true only for Berget). `gofmt`, `go vet`, `go build`, and `go test` green,
including a new regression test (`TestTranscribeBergetKeepsSentenceSpaces`).

**Symptom**

The same audio against Berget and Rngv GPU. Rngv gave correct spacing; Berget removed
the space after the period at sentence boundaries: "förluster.Ungdomarna", "haft.Vi",
"sörjer.Både", and also "fåskriva" in the middle of a phrase.

**Diagnosis — the cause is today's own word-split fix**

The earlier fix (above) switched `normalizeTranscript` from
`strings.Join(strings.Fields(s), " ")` (line break → space) to **dropping**
line breaks without a separator. That assumption — that the whisper.cpp server and Berget
serialize segments the same way — was wrong:

- **Local whisper.cpp** leaves the segment text **untrimmed**: a genuine word boundary carries
  its own leading space on the next segment, and only a boundary *inside* a
  word lacks it. Dropping the line break is therefore correct (preserves "Tydlighet").
- **Berget** **trims** each segment line. Then the line break is the *only* thing separating
  one sentence from the next ("förluster." + "Ungdomarna"). Dropping it glues
  the sentences together. Before today's fix, `Fields/Join` gave a space here and Berget
  looked right — so the fix that solved the local server broke Berget.

The two cases "Tyd"+"lighet" (should be glued) and "förluster."+"Ungdomarna" (should be
separated) cannot be told apart from the line break alone, so no single rule
works for both servers.

**Decision**

Make the joining backend-specific via `Backend.TrimmedSegments`:
- `false` (local whisper.cpp servers): drop line breaks without a separator —
  unchanged behavior, preserves mid-word compounds.
- `true` (Berget): let the line breaks become spaces (`Fields/Join` suffices, since
  it already treats a line break as whitespace).

**Alternatives considered**

- **Period-based heuristic** (a space only after punctuation). Rejected:
  "få"+"skriva" (should be separated) and "Tyd"+"lighet" (should be glued) are both
  letter+letter without a space — impossible to tell apart without token data.
- **`verbose_json` with word timestamps.** Rejected as overkill; Berget trims
  anyway, and the backend-specific flag solves the problem deterministically.

**Consequences**

- "fåskriva" disappears as a side effect: with Berget every segment boundary becomes a
  space, so "få" + "skriva" becomes "få skriva" (correct — they are two words).
- If a future backend is added, its segment serialization must be verified
  and `TrimmedSegments` set accordingly.

### 2026-06-21: F8 popup restyle — Win32 decisions and traps

**Status:** Done and live-verified. The code is in
`internal/popup/popup.go`. All steps committed with individual
conventional commit messages; the entire CHANGELOG.md is up to date.

**Background**

The F8 popup's original look was a blank `WS_BORDER` window with a white
background and an ordinary edit field — functional but visually uninspired. Three
restyle iterations (Variant 1, Variant B, and the shadow addition) produced the current
look: a tonal mint panel background, a white centered edit field with rounded corners,
a teal border via DWM, an F8 chip in the caption row, generous padding, and a DWM shadow
that follows the rounded corners.

**Decision 1 — DWM shadow via a custom frame (not CS_DROPSHADOW)**

`CS_DROPSHADOW` produced a rectangular legacy shadow whose square corners stuck out
past the window's DWM-rounded corners (an artifact visible in the lower right corner). A
shadow that truly follows the rounded corners requires the DWM compositor's own
shadow rendering, which is only active when DWM treats the window as "framed".

The way there: create the window with `WS_CAPTION` (not just `WS_POPUP`) so DWM
considers the window framed and draws the shadow; then remove the visible
title bar and border by returning 0 from `WM_NCCALCSIZE` (with
`wParam == TRUE`). `DwmExtendFrameIntoClientArea({cyBottomHeight: 1})` keeps the
DWM frame alive while the client area covers the whole window, and
`SetWindowPos(SWP_FRAMECHANGED)` forces an immediate recalculation. `WS_VISIBLE` is
removed from creation and the window is shown via `ShowWindow` after the shaping — otherwise
the title bar flashes for one frame before it disappears.

`CS_DROPSHADOW` was removed permanently as an interim fix (the only clean solution
to the rectangle clash); it was then replaced by the DWM variant above.

**Decision 2 — ES_MULTILINE for vertical text centering**

A true single-line `EDIT` (`!ES_MULTILINE`) ignores `EM_SETRECT` / `EM_SETRECTNP`
top/bottom — Win32 always top-aligns the text and ignores the
vertical position of the formatting rectangle. This means the centering calculation
(read `tmHeight`, compute the centered y, set the format rect) has no effect.

The solution: give the field `ES_MULTILINE`. A multiline edit respects the
formatting rectangle vertically. The field is still used as a single line:
`ES_AUTOHSCROLL` prevents line wrapping, and Enter is caught in the modal loop before
it reaches the control, so no new line can be created. The behavior is identical to an
ordinary single-line field, but the centering code works.

**Decision 3 — The region must be set AFTER WM_SETFONT**

Both `EDIT` (with `ES_MULTILINE`) and `STATIC` reset their window region
(set by `SetWindowRgn`) when they receive `WM_SETFONT`, because the control
recomputes its internal layout and rewrites the region as part of that work.
The consequence is that rounded corners set in `createEdit` / `createChip` are silently lost
when the font is assigned later in `run()`.

The solution: move the region application out into dedicated helper functions
(`roundEdit`, `roundChip`) that are called *after* `WM_SETFONT` has been sent. This
is not an edge case — it applies to all common Win32 controls that may redraw
themselves on a font change.

**Consequences**

- The popup requires Windows Vista+ for the DWM shadow (a `.Find()` guard → harmless
  no-op on older systems if such systems can even run the rest of Prata).
- `WM_NCCALCSIZE` with `wParam == TRUE` returns 0 → the client area fills the whole
  window. `wParam == FALSE` falls through to `DefWindowProc` normally.
- Three GDI brushes (panel, chip/teal, field/white) are created in `run()` and freed
  via `defer` in LIFO order (after `DestroyWindow`) — never per message.
- Fonts: 11pt normal for the field, 10pt semibold for the caption and chip. Both
  DPI-scaled via `CreateFontW` with a negative `lfHeight` (point size, not px).
- Layout constants (Variant B): `baseMargin` 16, `baseGap` 14, `baseHeight`
  104, `baseTextMargin` 12, `baseChipGap` 12 — all @ 96 DPI, scaled up.

### 2026-06-22: Daemon log — durable per-dictation record (`internal/daemonlog`)

**Context:**

The daemon writes all diagnostics with `fmt.Fprintf(os.Stderr, …)`, but in
production it is built `-H windowsgui` and runs with no attached console, so
every one of those lines is silently discarded. When a dictation misbehaves in
the field there is no record of what the daemon saw — which backend was active,
whether the capture was too short, whether transcription failed, whether
injection landed. The installer already solved the same "no console under
windowsgui" problem with append-mode file logging (`internal/installer.logf`);
this ports that pattern to the daemon.

**Decision:**

A new `internal/daemonlog` package, deliberately tiny: `Open`/`Close`/`Printf`,
a package-level `*os.File` behind a mutex, stdlib only, no levels, no structured
fields. `Open` is best-effort and returns a no-op closer plus an error on
failure — a missing log must never be fatal in a patient-facing tool, so the
caller logs to stderr and continues. `Printf` is a no-op until `Open` succeeds,
so a stray early call can't panic. The path is
`%LOCALAPPDATA%\Prata\logs\prata-YYYY-MM-DD.log` (LOCALAPPDATA read directly,
like the rest of the codebase), with `PRATA_DAEMON_LOG` as a full-path override
for test isolation — the same lever as `PRATA_INSTALL_LOG`.

`cmd/prata` now mirrors each per-dictation stderr event to the log (the existing
stderr lines are kept verbatim), stamped with `client.ActiveBackend().ID` and
the round's elapsed time. `processEvents` gained the `*transcribe.Client`
argument purely to read that backend ID at log time.

**Privacy (the load-bearing constraint):**

The log lines carry **metadata only** — backend ID, elapsed seconds, character
count, sanity ratio, error strings — and **never the transcribed text**. This is
the opposite of the kept stderr lines (`injected %q`), which do echo the text;
those are fine because stderr is discarded in production, but a durable file
beside patient work must not. Per AGENTS.md §11 the `logs/` directory and
`prata-*.log` are gitignored as a second layer of defense (the file already
lives outside the repo under `%LOCALAPPDATA%`; the ignore guards against a stray
`PRATA_DAEMON_LOG` override or manual run).

**Placement trap:**

The processor goroutine (`processEvents`) is launched *before* the tray is built
in `main`, so `daemonlog.Open` goes immediately before that goroutine starts —
the binding requirement is "open the log before anything writes to it," and the
processor is the only writer.
- Released in **v0.3.0** (redistribution via `prata.exe --install` / `Installera-Prata.bat`).

### 2026-06-22: Explicit stale-HWND validation before injection (`inject.IsWindow`)

**Context:**

Transcription is asynchronous: F1-release queues the capture to a worker that
can take up to ~24s on a slow Berget round. The foreground HWND at press time is
captured into `transcribeJob.targetHwnd` and carried to
`transcribeResult.targetHwnd`; before injection, `processEvents` calls
`inject.RestoreForeground` to bring that window back.

The patient-safety scenario: a doctor dictates into patient A's record, switches
to patient B's record while the slow backend is still working, and the result
arrives targeting A's now-closed window. This already failed *safely* —
`GetWindowThreadProcessId` inside `RestoreForeground` returns thread-ID 0 for a
dead HWND, so it returns an error and the result is dropped with an error cue —
but the failure was implicit and its log line ("inject restore foreground
failed") did not name the actual cause.

**Decision:**

Add a thin `inject.IsWindow(hwnd) bool` wrapper over the Win32 `IsWindow` and
fast-fail in `processEvents` *before* `RestoreForeground` when the target HWND no
longer refers to a live window. This is a clarity/speed improvement, not a new
line of defense — `RestoreForeground` still guards the same case, so the check is
deliberately redundant rather than load-bearing.

**Two distinct "no window" cases, two messages:**

The pre-existing `res.targetHwnd == 0` guard and the new `!IsWindow` guard are
*different* failures and must read differently in the log, or a diagnostic is
useless:

- `targetHwnd == 0` — no foreground window existed *at press time* (nothing was
  ever captured). Logged as `inject skipped: no target window`.
- `!IsWindow(targetHwnd)` — a window was captured but has since been *closed*.
  Logged as `inject skipped: target window gone`.

The daemon-log line for the `== 0` case was relabeled from the earlier "target
window gone" to "no target window" as part of this change, so the genuine
window-closed case owns the "gone" wording. Order matters: the `== 0` check runs
first, because `IsWindow(0)` is also false and would otherwise swallow the
distinct "nothing captured" case.

## 2026-06-25 — Dictionary fold-in tool + daemon-log retention

Two low-risk hardening items from the AI-council review (run #22) that needed no
hardware to verify, so both are unit-tested on Linux and covered by CI.

### `cmd/dict-foldin` implemented (the specified fold-in tool)

The build-time fold-in contract (above) is now built. One decision worth
recording: **the merge logic lives in `internal/dict` (`FoldIn`), not in the
CLI.** The contract says fold-in semantics must be "identical to the runtime
`mergeRules`". The only durable way to guarantee that is to share code, so
`FoldIn` sits beside `mergeRules`/`Save` and reuses the same `parse`. The CLI is
then a thin shell: read two files, call `dict.FoldIn`, print an
added/replaced/skipped report, write the baseline back (unless `--dry-run`).
`FoldIn` operates on the raw baseline *text* (line-preserving, like `Save`)
rather than on a parsed rule list, because the baseline is a hand-maintained
file — its comments, blank lines, and order must survive a fold-in. Re-folding
the same override is idempotent, and baseline rules are never removed.

### Daemon-log retention

`internal/daemonlog` now deletes `prata-YYYY-MM-DD.log` files older than 30 days
when it opens today's log. A "see and forget" daemon that runs for years would
otherwise grow the `logs/` directory by one small file per active day forever —
unbounded, even if tiny. The date is parsed from the *filename*, not the file's
mtime, so a copied or touched file is still pruned on its real day, and only
names matching the exact `prata-YYYY-MM-DD.log` pattern are ever removed (an
unrelated file beside the logs is left alone). Best-effort throughout: any error
is ignored, because failing to prune a log must never disrupt dictation. Prune
is skipped when `PRATA_DAEMON_LOG` overrides the path (tests).

### Clipboard hardening on the paste path (claim_009)

The non-Chromium paste path (`Type`) saved/restored only `CF_UNICODETEXT`, so
dictated medical-record text briefly sat in the clipboard like any other entry —
visible in clipboard history (Win+V) and syncable to the cloud clipboard.
SendInput targets (Chromium/Webdoc) never had this exposure; the paste fallback
did. Fix: after setting the text, the paste now also sets three registered
marker formats — `CanIncludeInClipboardHistory` (DWORD 0),
`CanUploadToCloudClipboard` (DWORD 0), and
`ExcludeClipboardContentFromMonitorProcessing` — in the same clipboard session,
via a new `setDictatedClipboardText`.

Three decisions worth recording:

- **Every Prata clipboard write is marked — the dictated text and the restore
  alike** (revised 2026-06-25). `writeClipboardText` now *always* sets the
  markers; both `setDictatedClipboardText` (the dictated write) and
  `setClipboardText` (restoring the user's prior clipboard after a paste or
  selection read) go through it. The SendInput path never touches the clipboard
  at all. Originally only the dictated text was marked and the restore was left
  unmarked to put the clipboard back "exactly as it was" — but that re-added a
  duplicate of the user's *own* prior copy to Win+V on every paste-path
  dictation, which a heavy clipboard-history user (copying radiology reports into
  the journal) sees as noise. Marking the restore too means Prata contributes
  nothing to Win+V; the user's original copy stays (it was an ordinary, unmarked
  Ctrl+C), so nothing of theirs is lost.
- **Best-effort, never fatal.** A failed `RegisterClipboardFormatW` /
  `SetClipboardData` for a marker is ignored: the text is already on the
  clipboard, and the worst case is the prior behavior (text in history), never a
  failed paste or a worse leak. This mirrors the package's existing best-effort
  clipboard posture.
- **The dictated text never lingers.** The dictated entry exists only for the
  brief paste window; the restore's `EmptyClipboard` drops it and its markers.
  The restore then re-marks the user's *own* content so it stays out of Win+V as
  a new entry too, while their original copy remains.

Verified live on Windows (2026-06-25): dictated text stays out of Win+V, and the
paste path no longer duplicates the user's own copy. **Note:** the markers were
briefly suspected of breaking the paste in Notepad++ that same session, but were
exonerated — manual Ctrl+V of marked text pastes fine there. The real cause was a
clipboard restore race (the paste path's `pasteSettleDelay` was too short); see
the 2026-06-25 entry below.

### Explicit, notify-only backend failover hint (claim_004)

"No silent failover" is a deliberate invariant — a dead backend must not
auto-route patient audio to the cloud. But it left a gap: when the LAN GPU is
down, the user only hears the error cue and cannot tell a backend outage from a
bad dictation. Fix: after two consecutive transcription failures on a
local/keyless backend, the tray shows a one-time balloon suggesting a manual
switch. It only *suggests*; the switch stays a deliberate menu action, so the
confidentiality model is untouched.

Decisions:

- **Notify-only, never auto-switch.** The hint points the user at the menu; the
  app never changes the backend or routes audio itself. That is the whole point
  of the invariant, so the failover package is named for the feature but does
  exactly one thing: decide *when to hint*.
- **Logic in a pure package (`internal/failover`), glue in `cmd/prata`.** The
  threshold / once-per-streak decision is stdlib-only and unit-tested on any OS;
  the Windows-only daemon just feeds it failures/successes and calls
  `tray.Notify`. Keeps the only hard-to-test part (the Win32 tray) thin.
- **Reset on any response.** A response that yields empty/degenerate text still
  counts as reachable and clears the streak — the hint is about reachability,
  not content. Only a true request failure (network/HTTP) advances it.
- **Suggest only from a local backend.** On Berget itself (`RequiresKey`) we do
  not hint — there is no more-available fallback to point at.
- **Tray created before the processor goroutine.** processEvents now needs the
  tray to raise the balloon, so `tray.New` moved above the processor launch
  (the rest of the tray configuration and `Run` still follow). `tray.Notify` is
  goroutine-safe and blocks until Run is ready, and the hint can only fire after
  the user has dictated twice, so the earlier creation is safe.

`internal/failover` is unit-tested and cross-compiles for windows. Verified
end-to-end on Windows (2026-06-25): with the active backend pointed at a
genuinely unreachable keyless GPU (Tailscale taken down), two consecutive
dictations produced two `transcribe error` log lines, then one
`failover hint shown` line and the tray balloon; a third failure produced no new
hint, confirming the once-per-streak guard.

### 2026-06-25: Notepad++ silently drops paste — the real cause is the clipboard restore race, not the markers

During live testing of the claim_009 clipboard markers, dictation into
**Notepad++** failed in the most dangerous way: the start/stop cues played, but
no text appeared and **no error cue sounded** — the dictation just vanished. The
daemon log even recorded `injected ... chars=N`, because the clipboard-paste
path (`Type`) reports success once `SetClipboardData` + the synthesized Ctrl+V
return; it never confirms the target actually accepted the paste.

Diagnosis path (worth recording, because the symptom is so quiet and the first
hypothesis was wrong):

- The symptom — start+stop cue, **no** error cue, no text — matches exactly one
  code path other than a working paste: `len(pcm) < minCaptureBytes` (a too-short
  capture, which also skips silently). The daemon log ruled that out (0
  occurrences) and showed `injected` lines instead — capture and transcription
  were fine, the text was leaving the app.
- The mic was fine (Diktell worked), and the text appeared correctly in **Word
  (`OpusApp`), classic Notepad (`Notepad`), PowerPoint, and Chrome**. Only
  **Notepad++** failed. Word/Notepad/PPT use the *same* clipboard-paste path and
  work, so the path itself is sound.
- **First hypothesis (WRONG): the claim_009 exclusion markers.** Plausible — only
  the clipboard path carries them, and Notepad++ is Scintilla. Disproved with a
  throwaway tool (`cmd/cliptest`) that puts CF_UNICODETEXT + a chosen subset of
  markers on the clipboard: **manual Ctrl+V of all-three-markers text pastes fine
  in Notepad++.** So the markers do not break the paste — something in Prata's
  *automated* paste sequence did.
- **Real cause: the restore race.** `Type` waits `pasteSettleDelay` after Ctrl+V,
  then restores the user's clipboard via `EmptyClipboard` + re-set. At 50 ms,
  Scintilla had not yet read the clipboard, so the restore wiped the dictated
  text before it landed. Confirmed by putting Notepad++ back on the clipboard
  path and bumping `pasteSettleDelay` to 400 ms — dictation then worked. Notepad/
  Word/PPT read the clipboard faster than 50 ms, which is why they never failed.

Fix (two parts):

- **General:** `pasteSettleDelay` 50 ms → 400 ms. This hardens the *whole*
  clipboard-paste path against slow readers, which is the true §2/§3 silent-loss
  risk — not Notepad++ alone. 400 ms is deliberately generous: silent dictation
  loss in a journal is far worse than an imperceptible restore delay.
- **Notepad++ specifically:** kept on `sendInputSafeClasses` (added the same day).
  SendInput is clipboard-free and so race-immune; it is the most robust path for
  an editor the user actively uses. The class is stable (unlike Qt classes such
  as `Qt6102QWindowIcon`, which encode the version and would break exact-match
  allowlisting on upgrade).

Lessons:

- The clipboard-paste path can fail **silently**: there is deliberately no
  execution fallback (it would risk a double-inject in a medical record), and the
  path never confirms the insert landed. The residual gap — *generic* detection
  that a paste was consumed — is unsolved here (Win32 has no clean signal short of
  delayed-rendering, a larger architectural change). The delay bump mitigates the
  known mechanism; the detection question is logged as open (PRATA-REVIEW §15 #3).
- Get a window class timing-independently from the process's `MainWindowHandle` +
  `GetClassName`, not a timed foreground probe (focus timing is unreliable).
- Don't stop at the first plausible cause: isolate it. The marker hypothesis was
  reasonable but a five-minute manual-paste test (`cmd/cliptest`) saved shipping
  the wrong mental model.

### 2026-06-25: Degenerate-output guard — validated the gzip threshold, added a complementary phrase-loop check (§15 #7)

The guard discarded transcriptions whose gzip ratio exceeds 2.4 (Whisper's own
`compression_ratio_threshold`) to catch KB-Whisper repetition loops on digit
strings. Open question: false positives on legitimate repetitive clinical text,
false negatives on subtle loops.

Measured against a corpus of realistic Swedish clinical phrases:

- **No false positives, wide margin.** Real token loops ("O A O A ...", repeated
  digits) score 8–12. The *most repetitive legitimate* dictation — "ingen X,
  ingen Y, ..."; bilateral findings; "utan anmärkning" lists — tops out at ~1.8.
  The 2.4 threshold sits in the empty gap. It must NOT be lowered: the worst
  legitimate case (~1.8) is the real floor, and dropping a true dictation is far
  worse than keeping a loop.
- **One real gap: low-repetition phrase loops.** A sentence repeated only ~4x
  compresses to ~1.9 — under the threshold. Lowering the ratio can't catch it
  without hitting the legitimate ~1.8 cases. They're genuinely inseparable *by
  compression ratio*.

So a second, orthogonal signal was added: `looksRepeated` flags a multi-word
phrase repeated back-to-back ≥4 times (scanning all positions, so an
end-of-output loop after real dictation is caught). It is false-positive-safe by
construction — four identical 2+-word phrases in a row never occur in real
dictation, while legitimate repetition repeats a *word* across *varied* content,
so the following words differ and the window never matches. Deliberately
conservative: a phrase repeated only 2–3x, and short single-word runs, are left
alone (ambiguous with a spoken read-back, short, visible to the user). Both
signals are locked in by regression tests so a future threshold tweak can't
silently regress the legitimate cases. The whole guard remains best-effort and
discard-only: on any doubt it keeps the text, because there is no fallback.

### 2026-06-25: Failure-mode review (§9) — a staleness guard for late injection; why a wrong-patient guard can't be window-based

A systematic sweep of the dictation pipeline (capture → queue → transcribe →
inject → F8/failover) for silent text loss, misdirected injection, and leaks. Two
fixes shipped; the headline risk turned out narrower than first feared.

**Async injection into a stale context.** `targetHwnd` is captured at F1 press,
but transcription is async (up to 30s on a Berget hiccup or queue backlog). The
existing guard only aborts if the target window was *closed* (`inject.IsWindow`),
not if its content changed — its comment even claims to handle "the user moved
from patient A's record to patient B's", which it does not. First framed as a
cross-patient hazard. Investigation killed the obvious guard: the journal
(Webdoc) is a hash-route SPA (`webdoc.atlan.se/#`) whose **window title is static
across patients** (the patient is in the page body), and the user keeps patients
in **tabs that share one Chrome HWND**. Prata operates at the window level, so it
**cannot** see an in-app patient switch — title and HWND are identical. A
title/HWND fingerprint is therefore ineffective for that case.

Recalibrated with the user's actual workflow: they finish one patient before the
next, so bouncing between patients is unlikely. The real, likely harm of a *very
late* result is that it lands mid-sentence in text the user has since started
typing **by hand**. Fix: a **staleness guard** (`maxInjectAge`, 8s). The backend
still counts as reachable (the failover streak is cleared first), but a result
older than the bound is dropped with an error cue + tray hint instead of injected.
Normal transcription is sub-second to ~2.7s; only an abnormal tail exceeds 8s.
Verified live: normal dictation injects; with the bound temporarily at 1ms every
result is dropped (`stale result dropped age=…` logged, no text, error cue, the
`age` matching the transcription time — proving the capture-time wiring). Residual
gap (a fast in-window tab switch under 8s) is undetectable by Prata and left to
operational discipline; separate Chrome *windows* per patient would make a future
HWND check effective if ever needed.

**Too-short capture dropped silently.** The `len(pcm) < minCaptureBytes` path
skipped with only the stop cue — a real dictation clipped by a slow device start
would vanish like the paste race. Now plays the error cue too, so no drop is
silent. An accidental F1 tap then beeps, which is honest feedback.

Lower-severity items, left as documented notes rather than code: F8's synthetic
Ctrl+C makes the *app* copy the selected text to the clipboard, so the selection
briefly enters Win+V — inherent to reading a selection by synthesizing a copy, and
not removable after the fact. And the events channel (buffer 4) can briefly
backpressure under rapid dictation during a long injection — it delays, never
loses.

### 2026-06-25: Multi-model external review (council) — triaged, two fixes acted on

PRATA-REVIEW.md was run through a four-model AI council (Claude Opus, Gemini, GPT,
Grok) with a deliberation round. The council surfaced ~51 distinct findings; each
was verified **against the actual v0.5.0 code** (a 57-agent triage), because a
plausible external finding is worth nothing until checked — the same lesson as the
Notepad++ marker red-herring. Most findings were already handled in v0.5.0
(staleness guard, paste race, 30s HTTP timeout — the "net/http has no timeout"
claim was a model hallucination the council itself flagged, UIPI/medium-IL,
markers, DPAPI, mutex), misframed, or hallucinated.

The council's headline — **wrong-patient injection** — was set aside as misframed:
it is inherent to switching windows in any journal system, not specific to Prata,
and transcription removes the need to type the patient name at all (the journal
pulls identity from the central population register). Prata should not, and does
not, manage patient identity; the one Prata-specific sliver (a late async result)
is already covered by the staleness guard.

Two findings were genuinely worth acting on, both in the "no dictation silently
goes wrong" family:

- **Silent-capture guard.** `minCaptureBytes` checked only length, so a muted /
  disconnected / wrong-default microphone produced a long-enough but silent
  capture; Whisper hallucinates a short phrase on silence, which then lands in the
  journal with no cue. A **muted mic is a recurring real case**, not a rare
  misconfiguration: on a shared clinic PC a nurse mutes her microphone to talk to
  a doctor who stops by to ask something, then forgets to unmute it — the next
  dictation captures silence. So the guard (and the named reminder below) pays off
  in everyday use, which is why it earns its keep. Added `audio.Peak` (loudest sample) and a conservative
  `silencePeakFloor` (512, far below real speech) — a silent capture now drops
  with the error cue. Conservative so a genuine quiet dictation is never dropped;
  the dropped peak is logged so the floor can be retuned. **Plus a named hint:**
  the error cue is the same double-pulse for five failure paths, and this one is
  specific and actionable, so the silent-mic path also shows a tray balloon
  "INGET LJUD — KONTROLLERA MIKROFONEN" (uppercase, readable at a glance). The
  developer's first idea was to *speak* it (a brief SAPI TTS sentence, since a
  balloon shares the §15 #11 discoverability gap) — built and tried, but the
  default Swedish SAPI voice was hard to understand, so it was dropped in favour
  of the written balloon. Lesson: a clear written message beat an unclear spoken
  one; the discoverability trade-off (balloon may be missed) was the lesser evil.
- **Panic recovery.** The long-running goroutines had no `recover()`; a panic in
  the transcription worker, F8 worker, or processor would silently take the
  daemon down — the worst outcome for a "see and forget" tool. Worker panics now
  become ordinary errors (`transcribeSafely`); F8 and the processor recover, log,
  and cue.

Set aside as documented decisions, not code (see PRATA-REVIEW §15): the LAN GPU's
plaintext-HTTP + unauthenticated response (real, but a clinic-LAN threat model and
a GPU-server-side change — the developer's network-trust call); a "see and forget"
**health/longevity signal** for silent task/hotkey breakage (a larger design
question, the genuinely under-defended axis); clipboard text stranded on a hard
kill (a ~400ms window, markers limit the blast radius); and the F8 Win+V leak
(inherent to reading a selection via synthetic Ctrl+C).

### 2026-06-25: "See and forget" health signal — design, and the first slice shipped

The under-defended axis: on an unmanaged clinic PC the daemon can stop working
silently (F1 hotkey unavailable, a crash, the Task Scheduler task disabled, a
Defender exclusion reset) and nobody knows until a clinician notices dictation
died, possibly weeks later. A design pass (an 8-agent orchestration: map the
silent-failure modes from the code, weigh five surfacing approaches against the
minimalism budget, synthesize) mapped ~14 silent modes and recommended:

- A **startup self-check** — probe F1 availability (register + unregister) and a
  mic, one log line per start, a balloon only on failure, **no** backend probe
  (its timeout harms startup and it duplicates the failover hint).
- **Task Scheduler restart-on-failure** for the crash class — the one thing a
  *non-running* daemon cannot report about itself.

Honest boundary: a non-running daemon cannot announce a deleted task, an AV reset
before it starts, or hotkey theft after launch — those need the OS (the task's
own restart policy) or an external probe, not the daemon.

**Shipped now (the highest-value, lowest-risk slice, no install-path change):**

- **Startup log anchor** (`daemon started version=… backend=…`) — a durable
  "I came up" record so the log shows whether/when Prata last started.
- **The darkest mode made visible.** F1's `RegisterHotKey` failure was the worst
  silent failure: it happens before the tray exists and, under `-H windowsgui`,
  exits with no console, no cue, no balloon — the tool simply appears dead. The
  listener-error path now logs `FATAL listener stopped` and shows a **modal
  message box** (which blocks, so it is seen even though we exit right after).
  A modal box is justified here despite "never interrupt": this is a fatal
  *can't-start* error, not a steady-state interruption.

**Deferred, pending the developer's decisions** (so the install path and a
behaviour change are not touched unilaterally):

- *Restart-on-failure* in the Task Scheduler task — bounded (e.g. 3×) vs unbounded
  (crash-loop risk); and it needs a real non-zero exit to trigger on.
- *F1 failure: stay fatal (current) or keep the daemon alive and self-heal* when
  the offending app closes — the latter is nicer for "forget" but adds a re-probe
  loop and a "alive but F1 dead" degraded state.
- *Surface:* a one-shot balloon (cheap, can be missed) vs a persistent degraded
  tray state (seen on Monday).
- A *startup mic probe* (earlier warning than the first failed dictation, which
  the silent-capture guard already covers).