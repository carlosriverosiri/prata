# DECISIONS-REJECTED — Prata's negative-knowledge register

> **Role: SOURCE.** The dead ends. Both what was rejected *and the reasoning* are
> first-class here — because the reasoning is usually a dialogue about what works
> and doesn't in practical clinical use, and a future AI rebuilding Prata must not
> re-tread paths we already disproved.
>
> *Why this file exists:* the reasoning already lived in `PRATA-DESIGN-LOG.md` and
> `PRATA-REVIEW.md` §15, but **as chronological prose** — ~40 rejected paths
> scattered across 1,500 lines with no index, no machine-findable status, and no
> "would it be safe to revisit, and when". This register lifts them out so the
> question *"show me every dead end and whether it's safe to retry"* is one scan,
> not a full read. Full narratives stay in the design log; this file is the
> **index + the two fields the prose lacked**: `Status` and `Re-try trigger`.

## How to read this

- **Status** tells you whether a path is permanently off-limits or merely parked:
  - `LOCKED` — never revisit; usually a patient-safety invariant.
  - `DISPROVEN` — factually settled by an experiment; the belief was wrong.
  - `INEFFECTIVE` — tried, didn't work for a structural reason.
  - `BUILT-THEN-DROPPED` — implemented, then removed after trying it live.
  - `DEFERRED` — a good idea, parked pending a precondition (see the trigger).
  - `SUPERSEDED-BY <id>` — replaced by a later decision.
- **Re-try trigger** is the load-bearing field: the *exact* precondition under
  which a parked path becomes worth reconsidering. `none` means do not revisit.
- IDs are stable. The design log and review reference them; never renumber.

---

## Register (scan this first)

| ID | Rejected / failed path | Class | Status | Re-try trigger |
| --- | --- | --- | --- | --- |
| REJ-001 | Ctrl+Win PTT via `WH_KEYBOARD_LL` hook | architecture | LOCKED | none — the hook failure class is inherent |
| REJ-002 | Ctrl+Win+Space via RegisterHotKey | ergonomics | DEFERRED | only if F1 turns out to need an Fn layer on a target keyboard |
| REJ-003 | Belief: bare F-keys reach the focused app | wrong-hypothesis | DISPROVEN | none — disproved by `cmd/regkey-test` canary |
| REJ-004 | F9 for the quick-fix hotkey | dependency | LOCKED | only if Diktell stops owning F9 |
| REJ-005 | Per-rune `KEYEVENTF_UNICODE` SendInput | implementation | DISPROVEN | none — fixed by one-call SendInput |
| REJ-006 | Unconditional SendInput everywhere | safety-invariant | LOCKED | none |
| REJ-007 | Allowlist the modern "Notepad" class | implementation | INEFFECTIVE | only if Win32 Notepad's SendInput handling changes |
| REJ-008 | Full all-format clipboard snapshot/restore | safety-invariant | LOCKED | none — TOCTOU race |
| REJ-009 | Denylist instead of allowlist | safety-invariant | LOCKED | none |
| REJ-010 | Execution fallback (SendInput fail → paste) | safety-invariant | LOCKED | none — double-inject in a record is unsafe |
| REJ-011 | Belief: clipboard markers caused Notepad++ paste loss | wrong-hypothesis | DISPROVEN | none — exonerated by `cmd/cliptest` |
| REJ-012 | Leaving the restored prior clipboard unmarked | implementation | BUILT-THEN-DROPPED | none — all writes now marked |
| REJ-013 | Qt exact-class allowlisting | implementation | DEFERRED | only if a safe paste-landing confirmation removes the need |
| REJ-014 | Pin whisper.cpp to v1.8.6 to fix word-splits | wrong-hypothesis | DISPROVEN | none — byte-identical A/B; bug is version-independent |
| REJ-015 | Patch/fork the whisper.cpp server | architecture | LOCKED | none — joining belongs on the client |
| REJ-016 | Fix word-splits via the dictionary | implementation | INEFFECTIVE | none — masks the fault, doesn't generalize |
| REJ-017 | One uniform segment-join rule for all backends | implementation | DISPROVEN | none — broke Berget; join is backend-specific |
| REJ-018 | Period-based join heuristic | implementation | INEFFECTIVE | none — letter+letter cases are indistinguishable |
| REJ-019 | `verbose_json` + word timestamps for joining | implementation | DEFERRED | only if a backend needs sub-segment timing |
| REJ-020 | Lower the gzip degenerate threshold below 2.4 | safety-invariant | LOCKED | none — corpus-validated, test-locked |
| REJ-021 | Flag 2–3× phrase repeats / short single-word runs | implementation | LOCKED | none — ambiguous with legitimate speech |
| REJ-022 | In-app self-update (download + replace + restart) | architecture | DEFERRED | only if code-signing lands (removes the AV/EDR flag) |
| REJ-023 | Silent auto-check for updates on startup | implementation | DEFERRED | low-cost to add later if cadence increases |
| REJ-024 | Reputation seasoning (age an unsigned binary into trust) | dependency | LOCKED | none — unreliable for a clinical tool |
| REJ-025 | Webroot folder allowlist as the durable AV fix | dependency | DEFERRED | superseded once signing lands; allowlist is the stopgap |
| REJ-026 | `%ProgramData%` shared writable dictionary | architecture | LOCKED | none — per-user override avoids the multisession race |
| REJ-027 | Machine-scope DPAPI for the Berget key | safety-invariant | LOCKED | none — exposes the secret on a shared PC |
| REJ-028 | `HKLM\Run` instead of Task Scheduler autostart | architecture | LOCKED | kept only as an emergency fallback |
| REJ-029 | MSI / Inno / NSIS / WiX packaging | architecture | LOCKED | none — breaks the single-file principle |
| REJ-030 | Separate named builds per site/branch | architecture | LOCKED | none — `Jobb` default + per-user override replaces it |
| REJ-031 | Explicit `LogonType` in the task XML | implementation | DISPROVEN | none — v1.2 schema ordering; caused "unexpected node" |
| REJ-032 | Literal "BUILTIN\Users" group name | implementation | DISPROVEN | none — display name is localized; use SID S-1-5-32-545 |
| REJ-033 | `MultipleInstancesPolicy = IgnoreNew` | implementation | LOCKED | none — would block other sessions' daemons |
| REJ-034 | Elevated (high-IL) daemon | safety-invariant | LOCKED | none — UIPI breaks SendInput into a non-elevated record |
| REJ-035 | Migrate per-user *data* across users on install | implementation | LOCKED | none — `apikey.dat` is user-scope DPAPI, unreadable |
| REJ-036 | Uninstall Option B (temp-copy relaunch) | implementation | DEFERRED | only if running `--uninstall` from the installed binary becomes required |
| REJ-037 | Window/HWND wrong-patient guard (title fingerprint) | safety-mechanism | INEFFECTIVE | only if patients ever get separate Chrome *windows* (not tabs) |
| REJ-038 | Treat wrong-patient injection as Prata's problem | architecture | LOCKED | none — misframed; journal owns patient identity |
| REJ-039 | Spoken SAPI TTS for the mic-failure alert | implementation | BUILT-THEN-DROPPED | only if a clearly intelligible Swedish TTS voice exists |
| REJ-040 | Unbounded Task Scheduler restart-on-failure | safety-mechanism | LOCKED | none — bounded 3×/PT1M instead |
| REJ-041 | Distinct degraded tray icon | implementation | DEFERRED | a louder option if the tooltip suffix proves too quiet |
| REJ-042 | F1-failure stays fatal / notify-and-give-up | implementation | DISPROVEN | none — strands a non-technical clinician; self-heal instead |
| REJ-043 | Heuristic to name the program holding F1 | implementation | INEFFECTIVE | only if Windows exposes a reliable API for it |
| REJ-044 | Startup mic probe | implementation | DEFERRED | low value; the silent-capture guard already names a dead mic |
| REJ-045 | Belief: `net/http` has no timeout (council finding) | wrong-hypothesis | DISPROVEN | none — a 30 s timeout was already present (hallucination) |
| REJ-046 | Rust (Diktell's stack) | architecture | LOCKED | none — Go chosen for "see and forget" longevity |
| REJ-047 | Cross-platform layer / config files / env-var key | architecture | DEFERRED | only when a real second platform/user actually needs it |
| REJ-048 | Whisper Flow (commercial competitor) | dependency | LOCKED | none — KB-Whisper quality + GDPR win |
| REJ-049 | Keep improving Diktell in parallel | process | LOCKED | none — "Diktell is finished" is the discipline |
| REJ-050 | Restore the user's prior clipboard after a dictation paste | safety-invariant | LOCKED | none — async-paste race can re-publish & paste the old content |
| REJ-051 | Win32 delayed-rendering to keep clipboard-restore safely | implementation | DEFERRED | only if losing clipboard-restore proves painful AND a deadlock-safe owner-thread design is validated |

Class legend: `architecture · safety-invariant · safety-mechanism · implementation · wrong-hypothesis · dependency · ergonomics · process`.

---

## Detail entries (high-value subset)

The full narrative for every item is in `PRATA-DESIGN-LOG.md` (dated) and
`PRATA-REVIEW.md` §15. Below are the entries whose reasoning is most valuable to a
rebuilder — each in the fixed template. The rest are summarized in the index above
and detailed in the design log.

### REJ-010 — Execution fallback (SendInput fails → fall back to paste)
- **Date / version:** 2026-05-31 (Decision 6)
- **Class:** safety-invariant · **Status:** LOCKED · **Re-try trigger:** none
- **What was tried:** On a SendInput error, retry the injection via the clipboard paste path.
- **Why it died:** SendInput may already have sent *some* characters before failing. A paste on top would double-inject into a patient record — a safety hazard. Lost text is the safe failure: the user simply re-dictates.
- **What replaced it:** No execution fallback. A failed inject plays the error cue and stops; nothing is pasted.
- **Lesson:** In a clinical injector, *silent partial duplication* is worse than *visible total loss*.
- **Cross-refs:** `PRATA-DESIGN-LOG.md` Decision 6; `PRATA-REVIEW.md` §7.

### REJ-011 — Belief: clipboard exclusion markers caused the Notepad++ paste loss
- **Date / version:** 2026-06-25 (v0.5.0)
- **Class:** wrong-hypothesis · **Status:** DISPROVEN · **Re-try trigger:** none
- **What was believed:** The three `claim_009` clipboard-history exclusion markers were breaking the paste in Notepad++ (text silently not landing).
- **How it was disproven:** A throwaway harness, `cmd/cliptest`, wrote all-three-markers text and a **manual** Ctrl+V pasted fine in Notepad++. Markers exonerated. The real cause was the **restore race**: at `pasteSettleDelay = 50 ms`, Scintilla hadn't yet read the clipboard, so the restore's `EmptyClipboard` wiped the dictated text first.
- **What replaced it:** `pasteSettleDelay` 50 ms → **400 ms** (REJ-011 sets the value in `CONSTANTS.md`), plus Notepad++ added to the SendInput allowlist as race-immune.
- **Lesson:** Don't stop at the first plausible cause — isolate it with a throwaway test before acting.
- **Cross-refs:** `PRATA-DESIGN-LOG.md` (2026-06-25, Notepad++); `PRATA-REVIEW.md` §15 #3.

### REJ-017 — One uniform segment-join rule for all backends
- **Date / version:** 2026-06-21
- **Class:** implementation · **Status:** DISPROVEN · **Re-try trigger:** none
- **What was tried:** Join whisper's per-line segments with a single rule regardless of backend.
- **Why it died:** Local servers return **untrimmed** segments (a real word boundary already carries its leading space), so concatenating *without* a separator preserves "Tydlighet". Berget returns **pre-trimmed** segments, so the same rule glued sentences ("förluster.Ungdomarna"). The two cases are indistinguishable from the line break alone.
- **What replaced it:** Backend-specific join driven by `Backend.TrimmedSegments` (true only for Berget): untrimmed → drop the newline with no separator; trimmed → newline becomes a space.
- **Lesson:** A normalization rule that depends on upstream trimming behavior must be keyed on the backend, not guessed from the text.
- **Cross-refs:** `PRATA-DESIGN-LOG.md` (2026-06-21, both entries); `PRATA-REVIEW.md` §5.4.

### REJ-022 — In-app self-update
- **Date / version:** 2026-06-15
- **Class:** architecture · **Status:** DEFERRED · **Re-try trigger:** *only if code-signing lands*
- **What was tried (on paper):** The binary downloads a new binary and replaces itself, then restarts.
- **Why it was rejected:** Download-and-execute is exactly the behavior AV/EDR (e.g. Webroot) flags on an *unsigned* exe, and it adds a silent failure path to the one operation that must not fail. Low gain at an annual update cadence.
- **What replaced it:** Notify-only ("Sök efter uppdatering…"); the upgrade is a manual USB re-run of `--install`. The binary never replaces itself.
- **Re-try trigger (explicit):** If Authenticode signing is in place, the AV-flag objection disappears and a guarded self-update becomes reconsiderable. Until then, do **not** add it.
- **Cross-refs:** `PRATA-DESIGN-LOG.md` (2026-06-15, update); `PRATA-REVIEW.md` §9.3.

### REJ-034 — Elevated (high integrity-level) daemon
- **Date / version:** 2026-06-16 / 2026-06-17
- **Class:** safety-invariant · **Status:** LOCKED · **Re-try trigger:** none
- **What was tried:** Run the daemon elevated (e.g. started directly from the elevated installer).
- **Why it died:** UIPI silently blocks SendInput from a **high-IL** process into a **non-elevated** target (the Webdoc journal). The text just never lands, with no error. This is a hard invariant.
- **What replaced it:** Only the *install action* elevates. The daemon runs at **medium IL** (Task Scheduler `RunLevel = LeastPrivilege`) and is started via `schtasks /Run`, never exec'd from the elevated installer.
- **Lesson:** An injector must run at the same (or lower) integrity level as its targets.
- **Cross-refs:** `PRATA-DESIGN-LOG.md` (2026-06-16 invariants, 2026-06-17); `PRATA-REVIEW.md` §9.2.

### REJ-037 — Window/HWND wrong-patient guard
- **Date / version:** 2026-06-25
- **Class:** safety-mechanism · **Status:** INEFFECTIVE · **Re-try trigger:** *only if patients ever get separate Chrome windows (not tabs)*
- **What was tried:** Fingerprint the target window (title / HWND) at F1 press and refuse to inject if it changed, to prevent dictating into the wrong patient.
- **Why it died:** Webdoc is a hash-route SPA with a **static title across patients**, and patients live in browser **tabs sharing one Chrome HWND**. Prata cannot see an in-app patient switch, so the fingerprint is blind exactly when it would matter.
- **What replaced it:** The `maxInjectAge` (8 s) staleness guard — a *temporal* proxy, not an identity check. The journal pulls patient identity from the population register; Prata never manages it (REJ-038).
- **Re-try trigger (explicit):** Only worth revisiting if the journal UI ever opens patients in *separate windows*, giving a real per-patient HWND to fingerprint.
- **Cross-refs:** `PRATA-DESIGN-LOG.md` (2026-06-25, §9 failure-mode); `PRATA-REVIEW.md` §15 #14.

### REJ-039 — Spoken SAPI TTS for the mic-failure alert
- **Date / version:** 2026-06-25
- **Class:** implementation · **Status:** BUILT-THEN-DROPPED · **Re-try trigger:** *only if a clearly intelligible Swedish TTS voice exists*
- **What was tried:** Speak "INGET LJUD — KONTROLLERA MIKROFONEN" via a short SAPI sentence, to beat the discoverability gap of a tray balloon on a shared PC.
- **What happened:** Built and tried live. The default Swedish SAPI voice was too unclear to understand.
- **What replaced it:** An uppercase written tray balloon, kept.
- **Re-try trigger (explicit):** Revisit only if a clearly intelligible Swedish voice is available. Until then do **not** re-add spoken alerts.
- **Lesson:** A clear written message beats an unclear spoken one — accept the lesser discoverability evil.
- **Cross-refs:** `PRATA-DESIGN-LOG.md` (2026-06-25, council); `PRATA-REVIEW.md` §15 #11.

### REJ-046 — Rust (Diktell's stack)
- **Date / version:** Decision 3
- **Class:** architecture · **Status:** LOCKED · **Re-try trigger:** none
- **What was rejected:** Build Prata in Rust, like its sibling Diktell.
- **Why:** The governing value is "see and forget" longevity. Go won on the Go 1 compatibility promise, broad stdlib coverage (Win32 via `syscall` with no third-party crates), a 150 MB vs 4–6 GB toolchain, a single static binary, and AI fluency in the language. Accepted trade-off: `malgo` is less battle-tested than Rust's `cpal`.
- **Lesson:** For an unattended tool maintained mostly through AI, toolchain durability and language fluency outweigh raw performance.
- **Cross-refs:** `PRATA-DESIGN-LOG.md` Decision 3; `PRATA-REVIEW.md` §12.

### REJ-050 — Restore the user's prior clipboard after a dictation paste
- **Date / version:** 2026-06-29
- **Class:** safety-invariant · **Status:** LOCKED · **Re-try trigger:** none — only a confirmed paste-landing signal could supersede it (see REJ-051)
- **What was rejected:** `Type` saving the prior `CF_UNICODETEXT` clipboard and re-setting it ~400 ms after Ctrl+V, as a courtesy so a copy → dictate → paste-the-original workflow kept the original copy.
- **Why:** `SendInput` posts Ctrl+V **asynchronously** — it does not wait for the target to read the clipboard. A cold/slow first paste (here: Infinity's chat field, not on the SendInput allowlist) read the clipboard *after* the restore had already put the user's prior content (a just-copied Markdown/code doc) back, so the OLD content was pasted instead of the dictation. In a patient journal a wrong-content paste is the worst outcome; nothing-pasted is acceptable. The restore is the *only* path by which old content can reach the target, so it was removed: once the dictation is set the clipboard only ever goes dictation → empty (clear). The worst residual is a silent empty paste, guarded by `pasteSettleDelay`.
- **What replaced it:** `Type` now `clearClipboard()`s after the settle instead of restoring (`internal/inject/inject.go`); `pasteSettleDelay` 400 → 700 ms as defense-in-depth. Accepted cost: the prior clipboard is not preserved (copy → dictate → paste-the-copy must re-copy). Bonus: dictated medical text no longer lingers on the clipboard at all.
- **Lesson:** A "courtesy" that re-publishes data into an async race can turn a silent *empty* failure into a silent *wrong-content* one — far worse. Related to REJ-008 (the all-format restore, already rejected for the TOCTOU race); this removes the single-format restore for the same family of reason.

### REJ-051 — Win32 delayed-rendering to keep clipboard-restore safely
- **Date / version:** 2026-06-29
- **Class:** implementation · **Status:** DEFERRED · **Re-try trigger:** only if losing clipboard-restore (REJ-050) proves painful in daily use AND a deadlock-safe owner-thread design is validated live
- **What was rejected (for now):** Make Prata the clipboard owner via `SetClipboardData(CF_UNICODETEXT, NULL)` and render the dictation on `WM_RENDERFORMAT` (the instant a consumer reads), restoring the prior clipboard only after the render — keeping the restore convenience while still fixing the wrong-content bug.
- **Why deferred:** It needs a dedicated message-only window + pinned-thread message loop, cross-thread coordination while `inputMu` is held (deadlock risk if the owner thread wedges), and `WM_RENDERALLFORMATS`/`WM_DESTROYCLIPBOARD` handling — and it is still **not** an absolute guarantee: the timeout-then-restore fallback re-publishes the prior content for a consumer that never honors `WM_RENDERFORMAT`. REJ-050 (clear-only) is a ~6-line change that *guarantees* no wrong-content paste, so it was chosen over this.
- **Lesson:** Prefer the simple change that makes the bad outcome structurally impossible over the complex one that only makes it unlikely — especially in patient-safety code.
- **Cross-refs:** `PRATA-DESIGN-LOG.md` (2026-06-29); evaluated as "Approach C" in the diagnosis workflow.

---

## Maintenance

- Every newly rejected / abandoned / built-then-dropped path gets a `REJ-NNN` row
  in the index **before the work is considered done** (`AGENTS.md` §2). Add a full
  detail block only if the reasoning is high-value; otherwise the index row + the
  dated design-log entry suffice.
- The design-log narrative should reference the `REJ-NNN` id rather than
  re-explaining, so the chronological story and this register never drift.
- `Status: LOCKED` and `Re-try trigger:` are the machine-findable fields — keep
  them greppable.

---

*Role: SOURCE. See `PRATA-DESIGN-LOG.md` for full narratives, `PRATA-REVIEW.md` §15
for open/resolved threads.*
<!-- freshness: valid-as-of <git-short-sha> (<tag>) <ISO-date> — stamp on release -->
