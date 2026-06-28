# CONTEXT-PACK — Prata

> **GENERATED — do not edit by hand.** Run `go run ./cmd/gen-context-pack > CONTEXT-PACK.md`.
> CI regenerates this and fails on any diff, so it cannot silently drift from the code.
> Deterministic: a pure function of the repository sources (no timestamps, no git calls).

## 0. Provenance

- Generator: `cmd/gen-context-pack` (stdlib-only, OS-independent).
- Source of truth: **this repository**. For the exact commit, see `git log`.
- This file is the *compiled* form of the spine docs + facts extracted from code.
- To rebuild Prata from docs alone, read in the order in §1; this pack embeds the
  highest-value pieces so you need not chase links for the essentials.

## 1. Read order (what answers what)

| Read | For | Role |
| --- | --- | --- |
| `PROJECT-IDENTITY.md` | module path, build cmd, absent secrets, traps | SOURCE |
| `PRATA-MASTER.md` | what it is + how it was reasoned, at a glance | SOURCE |
| `CONSTANTS.md` | every load-bearing constant + why | SOURCE |
| `DECISIONS-REJECTED.md` | dead ends + Status + Re-try trigger | SOURCE |
| `PRATA-DESIGN-LOG.md` | the full reasoning dialogue (dated) | SOURCE |
| `PRATA-GPU-SERVER.md` | backend/server/network setup | SOURCE |
| `PRATA-REVIEW.md` §15 | open vs resolved questions | DERIVED |
| `CHANGELOG.md` | release/work history | SOURCE |

## 2. Identity (embedded: PROJECT-IDENTITY.md)

> **Role: SOURCE.** The un-guessable identity facts a rebuilder needs *first* and
> cannot infer from prose: canonical names, the module path, the one build
> command, and the facts that are *deliberately absent* from the docs (and where
> they really live). Pin one fact in exactly one place. Keep it tiny.
>
> *Why this file exists:* an AI rebuilding Prata from the docs alone was found to
> guess the module path (the docs only showed the GitHub URL, which differs) and
> to mis-purpose a package (see "Known traps"). This file removes both classes of
> guess.

## Canonical names

| Fact | Value | Notes |
| --- | --- | --- |
| Product name | **Prata** | Swedish for "talk / chat". |
| Go module path | **`github.com/carlosriveros/prata`** | Declared in `go.mod`. **Load-bearing for import paths.** |
| GitHub repository | **`carlosriverosiri/prata`** | The repo *slug* is `carlosriverosiri` — it deliberately **differs** from the module path's `carlosriveros`. Both are correct; they are not the same string. A rebuilder must use the **module path** above for imports, not the repo URL. |
| Primary binary | `prata.exe` | Single binary: daemon + `--install` / `--uninstall` / `--set-key`. |
| Sibling project | Diktell | Finished and frozen; runs on GPU machines. Prata targets machines *without* a GPU. Not a version of Prata — a sibling. |

## Platform & toolchain

| Fact | Value |
| --- | --- |
| Language | Go **1.26** (`go.mod` pins `go 1.26.3`). |
| Build constraint | **Windows-only**, `CGO_ENABLED=1` (the `malgo` audio dependency uses cgo → needs a C compiler: MinGW-w64 / TDM-GCC). |
| Only external dependency | `github.com/gen2brain/malgo v0.11.25` (audio capture). Everything else is stdlib + hand-rolled Win32. |

## The one build command (production)

```
CGO_ENABLED=1 go build -ldflags="-s -w -H windowsgui -X main.version=<tag>" -o prata.exe ./cmd/prata
```

Verification gate (mirror of `.github/workflows/ci.yml`, runs on `windows-latest`):

```
gofmt -l .            # must print nothing
CGO_ENABLED=1 go vet ./...
go build ./...
go test ./... -count=1
```

> **Note for non-Windows machines (e.g. CI Linux runners):** the daemon and most
> `internal/` packages do **not** compile off Windows (Win32 syscalls + cgo). Only
> pure-stdlib, OS-independent packages such as `cmd/gen-context-pack` build on
> Linux. The docs-freshness CI job relies on that fact.

## Deliberately absent from the docs (and where the real values live)

These are omitted on purpose — do **not** treat their absence as a documentation
gap, and do **not** invent values:

| Absent fact | Why absent | Where it really lives |
| --- | --- | --- |
| Berget AI **API key** | Secret. DPAPI-encrypted per user. | `%LOCALAPPDATA%\Prata\apikey.dat`; set via `prata --set-key`. Never committed, never logged. |
| GPU-server **endpoint URLs** | Operational / confidentiality. | `CONSTANTS.md` (the hard-coded constants) + `PRATA-GPU-SERVER.md` (topology). They are compile-time constants, not config. |
| Clinic **network addressing** | Site-specific. | `PRATA-GPU-SERVER.md`. |

## Known traps (facts a literal reader gets wrong)

1. **Two-step backend default.** `transcribe.NewClient()` constructs with the
   **Berget** backend as its in-code default, but `main` immediately calls
   `loadBackendPref()` → `SetBackend(Work)` at startup. The **observable** first-run
   default is therefore **LAN GPU-server (`Jobb`)**, *not* Berget. A rebuilder who
   codes `NewClient`'s default straight from prose will ship the wrong startup
   backend. The two-step is load-bearing.
2. **`internal/sanity` is the degenerate-output guard**, *not* "startup
   self-checks". It rejects Whisper repetition loops (gzip ratio + phrase-loop)
   before they reach the journal — a patient-safety feature. (An older `AGENTS.md`
   §5 label called it "startup self-checks"; that was wrong and is corrected.)
3. **Stable backend IDs are decoupled from display names.** Persisted choice is the
   ID (`Hemma` / `Jobb` / `Berget`) in `backend.txt`; the display name can change
   without breaking a saved choice.

---

*Role: SOURCE. Pin facts here once. See `AGENTS.md` §1 for the full doc map.*
<!-- freshness: valid-as-of <git-short-sha> (<tag>) <ISO-date> — stamp on release -->

## 3. Pinned facts — auto-extracted from the code

This is a CI-checked **subset** — the highest-churn, code-only constants — read live from
the source file named; `CONSTANTS.md` is the complete registry. If one changes in code, this
table changes, and the CI drift gate forces this pack to be regenerated.

| Fact | Value | Source | Why it matters |
| --- | --- | --- | --- |
| `Go module path` | `github.com/carlosriveros/prata` | `go.mod` | import root — differs from the GitHub slug carlosriverosiri |
| `silencePeakFloor` | `512` | `cmd/prata/main.go` | peak below this ⇒ treat capture as silence (dead mic) |
| `transcribeQueueDepth` | `8` | `cmd/prata/main.go` | FIFO transcribe queue + jobs/results/dictAdds channel sizes |
| `failoverFailureThreshold` | `2` | `cmd/prata/main.go` | consecutive failures before the once-per-streak tray hint |
| `maxInjectAge` | `8 * time.Second` | `cmd/prata/main.go` | drop (not inject) a transcription older than this after F1 release |
| `pasteSettleDelay` | `400 * time.Millisecond` | `internal/inject/inject.go` | wait after Ctrl+V before restoring the prior clipboard |
| `copySettleTimeout` | `300 * time.Millisecond` | `internal/inject/inject.go` | F8 wait for the synthesized Ctrl+C to populate the clipboard |
| `focusSettle` | `30 * time.Millisecond` | `internal/inject/inject.go` | wait before re-reading the foreground to confirm a restore |
| `interEventDelay` | `2 * time.Millisecond` | `internal/inject/inject.go` | inter-event delay on the SendInput path |
| `httpTimeout (transcribe)` | `30 * time.Second` | `internal/transcribe/client.go` | whole-request timeout for a transcription POST |
| `maxRatio` | `2.4` | `internal/sanity/sanity.go` | gzip degenerate-output threshold — must NOT be lowered |
| `minLength` | `60` | `internal/sanity/sanity.go` | byte floor below which the gzip ratio is not trusted |
| `minPhraseReps` | `4` | `internal/sanity/sanity.go` | phrase-loop: repeats before a back-to-back phrase is flagged |
| `f1RetryInterval` | `3 * time.Second` | `internal/hotkey/listener.go` | F1 self-heal re-probe cadence |
| `retentionDays` | `30` | `internal/daemonlog/daemonlog.go` | per-day daemon logs older than this are pruned on startup |
| `copyRetryAttempts` | `10` | `internal/installer/installer.go` | bounded retry for a transiently locked target binary |
| `httpTimeout (update)` | `10 * time.Second` | `internal/update/update.go` | notify-only update-check request timeout |

## 4. Negative knowledge — rejected paths (index from DECISIONS-REJECTED.md)

**49 rejected/abandoned paths recorded.** A rebuild must not re-tread these.
`Status: LOCKED` = never revisit; `DEFERRED` = parked pending the `Re-try trigger`.

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

Full detail (Status + Re-try trigger per item): `DECISIONS-REJECTED.md`.
Dated narratives: `PRATA-DESIGN-LOG.md`. Open threads: `PRATA-REVIEW.md` §15.

## 5. Where to go deeper

- Full "what + why" synthesis: `PRATA-MASTER.md`.
- The reasoning dialogue behind each decision: `PRATA-DESIGN-LOG.md` (dated).
- Backend / server / network runbook: `PRATA-GPU-SERVER.md`.
- How to work on the project + doc-freshness rules: `AGENTS.md`.
