# CONSTANTS — Prata

> **Role: SOURCE.** Every value here is **load-bearing for a rebuild**: a future AI
> cannot invent a project-specific magic number it was never told. The rule is
> simple — **if a load-bearing constant lives only in a code comment, that is a
> documentation bug.** Each row gives the value, where it lives, *why this value*,
> and (where relevant) the doc that argues it.
>
> *Why this file exists:* an AI rebuilding Prata from the docs alone could
> reconstruct ~80% of the app, but the remaining gap was almost entirely
> **constants that existed only in code** (`transcribeQueueDepth`, the inject
> sub-timeouts, the queue/channel sizes). This file turns "must guess" into "look
> it up". When you change a value below, change it here in the same commit
> (`AGENTS.md` §2).

## Dictation pipeline (`cmd/prata/main.go`)

| Constant | Value | Source | Why this value |
| --- | --- | --- | --- |
| `silencePeakFloor` | **512** (amplitude, of 32767 full-scale) | `cmd/prata/main.go:91` | Below this peak the capture is treated as silence (muted/dead mic) → drop with the error cue, do **not** inject Whisper's hallucinated boilerplate. Conservative (~1.5% full scale). |
| `transcribeQueueDepth` | **8** | `cmd/prata/main.go:122` | Bounds how many finished captures may queue for the FIFO transcribe worker; also sizes the `jobs`, `results`, and `dictAdds` channels. Queue-full → oldest is dropped. **Was code-only — the #1 reconstruction gap.** |
| `failoverFailureThreshold` | **2** | `cmd/prata/main.go:127` | Consecutive failures on the active keyless backend before the once-per-streak tray hint fires. See `internal/failover`. |
| `maxInjectAge` | **8 s** | `cmd/prata/main.go:137` | A transcription older than this (measured from F1 release) is dropped, not injected: the normal path is sub-second to ~2.7 s, so a tail > 8 s means the user has already started typing by hand. **Not** a wrong-patient guard (Webdoc shares one HWND across patient tabs — see `DECISIONS-REJECTED.md` REJ-037). |
| `events` channel size | **4** | `cmd/prata/main.go` | Covers any realistic F1 press/release burst. Code-only. |
| `f1Available` channel size | **4** | `cmd/prata/main.go` | As above, for the F1 self-heal signal. Code-only. |

## Text injection (`internal/inject/inject.go`)

| Constant | Value | Source | Why this value |
| --- | --- | --- | --- |
| `pasteSettleDelay` | **400 ms** | `internal/inject/inject.go:44` | Wait after Ctrl+V before restoring the prior clipboard. Was 50 ms; Scintilla (Notepad++) read the clipboard slower than 50 ms, so the restore's `EmptyClipboard` wiped the dictated text before it landed. Applies to **all** paste targets; deliberately generous. See `DECISIONS-REJECTED.md` REJ-011 and `PRATA-DESIGN-LOG.md` (2026-06-25). |
| `copySettleTimeout` | **300 ms** | `internal/inject/inject.go:49` | How long `CopySelection` (F8) waits for the synthesized Ctrl+C to populate the clipboard. |
| `focusSettle` | **30 ms** | `internal/inject/inject.go:55` | How long `RestoreForeground` waits after `SetForegroundWindow` before re-reading the foreground to confirm the restore. Code-only. |
| `interEventDelay` | **2 ms** | `internal/inject/inject.go:33` | Inter-event delay on the SendInput path. Code-only. |
| `sendInputSafeClasses` | **`{ "Chrome_WidgetWin_1", "Notepad++" }`** | `internal/inject/inject.go:203-214` | Exact-match allowlist of foreground window classes routed to SendInput Unicode; everything else uses clipboard paste. Allowlist, **not** denylist (untested apps default to the proven paste path). See REJ-006/REJ-009. |
| `OpenClipboard` retry | **500 ms window, 10 ms poll** | `internal/inject/inject.go` (~505) | Retry loop when `OpenClipboard` is contended. Code-only. |

## Transcription & backends (`internal/transcribe/client.go`)

| Constant | Value | Source | Why this value |
| --- | --- | --- | --- |
| `httpTimeout` | **30 s** | `internal/transcribe/client.go:22` | Whole-request timeout for a transcription POST. |
| `HomeURL` | `http://100.87.6.56:8080/v1/audio/transcriptions` | `client.go:35` | Home GPU server over Tailscale (`Hemma`). |
| `WorkURL` | `http://10.64.3.60:8080/v1/audio/transcriptions` | `client.go:36` | Clinic LAN GPU server (`Jobb`), fixed IP. |
| `BergetURL` | `https://api.berget.ai/v1/audio/transcriptions` | `client.go:37` | Cloud fallback (`Berget`, Bearer-authenticated). |
| Backend IDs | `Hemma` / `Jobb` / `Berget` | `client.go:65-67` | Stable persisted IDs, decoupled from display names. `RequiresKey` true only for Berget; `TrimmedSegments` true only for Berget. |
| Audio format | **16 kHz, mono, 16-bit PCM** | `internal/transcribe/wav.go` | OpenAI-compatible multipart (`file`, `model`, `language`, `response_format`); `model = KBLab/kb-whisper-large`, `language = sv`. |

## Degenerate-output guard (`internal/sanity/sanity.go`)

| Constant | Value | Source | Why this value |
| --- | --- | --- | --- |
| `maxRatio` | **2.4** | `internal/sanity/sanity.go:48` | gzip compression-ratio threshold (mirrors Whisper's `compression_ratio_threshold`). Token loops score 8–12; worst *legitimate* repetitive clinical dictation tops out ~1.8. **Must NOT be lowered** — doing so discards legitimate text. See REJ-020. |
| `minLength` | **60** (bytes) | `internal/sanity/sanity.go:44` | Length floor below which the gzip ratio is not trusted (avoids short-text false positives). Code-only in the curated docs. |
| `minPhraseWords` | **2** | `internal/sanity/sanity.go` | `looksRepeated` flags a back-to-back repeat of a phrase of at least this many words. |
| `minPhraseReps` | **4** | `internal/sanity/sanity.go:58` | …repeated at least this many times (a sentence emitted ~4× compresses to only ~1.9, under `maxRatio`). 2–3× repeats are left alone deliberately (ambiguous with legitimate read-back). See REJ-021. |

## Hotkeys, logging, installer, cues, update

| Constant | Value | Source | Why this value |
| --- | --- | --- | --- |
| `f1RetryInterval` | **3 s** | `internal/hotkey/listener.go:74` | F1 self-heal re-probe cadence: when another program owns F1 the daemon stays alive and re-registers the instant F1 frees. |
| `retentionDays` | **30** | `internal/daemonlog/daemonlog.go:41` | Per-day daemon logs older than this are pruned on startup (date read from filename, only `prata-*.log`). Keeps the dir bounded over years of unattended running. |
| `copyRetryAttempts` | **10** (× 200 ms) | `internal/installer/installer.go:100` | Bounded retry for a transiently locked target binary during install/uninstall copy/remove. |
| Cue tones | start **880 Hz**, stop **587 Hz**, error **2×330 Hz** | `internal/cue/cue.go:45-47` | Distinguishable PTT start/stop tones; a double low pulse for the silent-failure error paths. |
| `httpTimeout` (update) | **10 s** | `internal/update/update.go:28` | Whole-request timeout for the notify-only "check for update" call. |

## Task Scheduler (installer) — partially code-only

| Fact | Value | Why |
| --- | --- | --- |
| RunLevel | **LeastPrivilege** (medium IL) | **Hard invariant:** an elevated (high-IL) daemon silently breaks SendInput into a non-elevated Webdoc (UIPI). The daemon is never exec'd from the elevated installer. See REJ-034. |
| Restart-on-failure | **3× / PT1M** | Bounded so a transient crash self-heals but a permanent failure stays down instead of crash-looping an unattended PC. See REJ-040. |
| `MultipleInstancesPolicy` | **Parallel** | One daemon per logon session on a shared PC (the single-instance mutex is session-scoped, not `Global\`). |
| Group principal | SID **`S-1-5-32-545`** (BUILTIN\Users) | Used as a SID because the display name is localized ("Användare" on Swedish Windows). |

> **Reconstruction note.** The exact, fully-ordered Task XML body is *not* enumerated
> in the docs (the design log stresses that child-element **order** is load-bearing
> and unit-tested, but doesn't list it). A rebuilder must iterate against
> `schtasks` "unexpected node" errors, or read `internal/installer/installer.go`.
> This is the largest remaining "must read the code" gap.

---

*Role: SOURCE. Owned by `AGENTS.md` §2 routing ("a constant → CONSTANTS.md").*
<!-- freshness: valid-as-of <git-short-sha> (<tag>) <ISO-date> — stamp on release -->
