# Prata — Roadmap & Status

> A curated status + backlog. The per-change record is in `CHANGELOG.md`; the
> detailed open-question discussion is in `PRATA-REVIEW.md` §15; the "why" behind
> each decision is in `PRATA-DESIGN-LOG.md`. This file is the high-level index.
>
> **Status:** v0.5.0 shipped 2026-06-25; `master` is clean.

---

## ✅ Completed (through v0.5.0)

Grouped by theme; commit refs in brackets.

**Dictation safety / injection**
- Silent **paste-loss fixed** — root cause was the clipboard *restore race*
  (`pasteSettleDelay` 50 ms → 400 ms), not the exclusion markers (exonerated via
  manual-paste test). Notepad++ additionally routed to SendInput. Verified live in
  Word, Notepad, PowerPoint, Chrome, Cursor, Notepad++. [`3da73db`, `e04c032`]
- **Late-injection staleness guard** (`maxInjectAge` 8 s) — a result returning long
  after the user finished dictating is dropped (cue + balloon) instead of injected
  mid-sentence. Verified live. [`d7d785a`]
- **Too-short capture** now plays the error cue — no dictation drops silently. [`6fea54f`]
- **Silent-capture guard** (`audio.Peak` + `silencePeakFloor`) — a muted /
  disconnected mic no longer injects Whisper's hallucinated text; it drops with the
  error cue **and an uppercase tray balloon "INGET LJUD — KONTROLLERA MIKROFONEN"**.
  (Real clinical case: a nurse mutes her mic to talk to a doctor who stops by, then
  forgets to unmute.) [`0e708a8`, `839dbbf`]
- **§9 failure-mode review** — the "wrong-patient" theme was analysed and set aside
  as misframed (inherent to window switching; the journal manages patient identity);
  the staleness guard is the real mitigation.

**Confidentiality / clipboard**
- **Clipboard / Win+V hardening** — the dictated text *and* the restore of the
  user's prior clipboard are both marked out of clipboard history, the cloud
  clipboard, and monitors, so Prata never adds an entry to Win+V (not even a
  duplicate of the user's own copy). [`cd01476`, `3da73db`]

**Robustness / "see and forget"**
- **Panic recovery** on the transcription worker, F8 worker, and processor
  goroutines — a bug no longer silently kills the daemon. [`0e708a8`]
- **Backend failover hint** (`internal/failover`) — notify-only, one balloon per
  outage streak; never auto-switches, patient audio never auto-routed. Verified
  end-to-end. [`84f0092`]
- **Daemon-log 30-day retention** — per-day logs auto-pruned so the directory stays
  bounded over years. [`f4e49c3`]

**Transcription quality**
- **Degenerate-output guard strengthened** — gzip threshold validated against a
  realistic clinical corpus (no false positives) + `looksRepeated` for the
  low-repetition phrase loops the ratio missed. Regression-tested. [`4f1ef55`, `65511bb`]

**Tooling / process**
- **Dictionary fold-in tool** (`cmd/dict-foldin`) — folds per-user corrections into
  the embedded baseline before a release. [`f4e49c3`]
- **External review** — ran PRATA-REVIEW through a 4-model AI council and triaged all
  51 findings **against the code** (57-agent pass): most were already handled,
  misframed, or hallucinations the council itself flagged. Two were acted on (the
  silent-capture guard and panic recovery above).
- **v0.5.0 cut + tagged + released**; feature branch merged to `master` and deleted;
  the whole doc set brought up to date and PRATA-REVIEW prepared for the review
  round. [`ea863ed`, `a156a10`, `87b93d3`]

---

## 🔭 Possible improvements (backlog)

Prioritised. Each cross-references `PRATA-REVIEW.md` §15 where there is a detailed
discussion. None of these are committed work — they are candidates.

### Higher value
1. **"See and forget" health signal** (§15 #14). *🟢 Three slices shipped
   2026-06-25:* (1) a durable startup log anchor (`daemon started …`); (2) a Task
   Scheduler **restart-on-failure** (bounded 3× / PT1M — self-heals a transient
   crash, no crash-loop) and a **persistent degraded tray state** (`SetDegraded`, a
   non-fading tooltip suffix; `SVARAR INTE` on a backend outage); (3) **F1
   self-heal** — an F1 `RegisterHotKey` failure no longer exits the daemon: it
   stays alive, cues + balloons + shows a persistent `F1 UPPTAGEN` badge, and
   re-probes every 3 s, recovering the instant F1 frees (the conflicting program
   closes) without a restart. **Remaining:** only a low-value startup mic probe.
   The hard limit: a *non-running* daemon can't report a deleted task / pre-launch
   AV block — needs the OS or an external probe.
2. **Transport security + backend-response authenticity** (§15 #13). The LAN GPU is
   reached over **plaintext HTTP** with no auth, and the response is injected with no
   integrity check → a LAN attacker (ARP spoof / MITM) could read patient audio *and*
   inject arbitrary text into the record. → HTTPS (self-signed + pinned) and/or an
   HMAC on the response. *Needs the GPU-server side too; a network-trust decision.*

### Medium / niche
3. **Generic paste-landing confirmation** (§15 #2/#3 remainder). The paste path can't
   confirm the insert landed; a high-latency RDP/Citrix target could still lose a
   dictation. Win32 has no clean signal short of delayed-rendering (a redesign).
   *Open research question.*
4. **Session-aware update** (§15 #4). `--install`/update kills *other* clinicians'
   active dictations on a shared PC. → Defer the update if someone is dictating.
   *Medium.*
5. **F8 quick-fix Win+V leak** (§15 #10). F8's synthetic Ctrl+C makes the *app* copy
   the selected journal text → it enters Win+V unmarked. → Read the selection without
   a clipboard write (UI Automation `TextPattern`?) or scrub the entry. *Hard;
   inherent to the synthetic-copy approach.*
6. **Extend named failure hints** (§15 #11). Generalise the uppercase-balloon hint to
   other specific failures (e.g. "BACKEND SVARAR INTE") and/or a persistent tray-icon
   state, weighed against more notifications / a voice in a room with a patient.
   *Low–medium.*
7. **Code signing / delivery** (§15 #1). The unsigned-binary AV problem; pick a
   signing path (OV/EV, Azure Trusted Signing) before IT-driven scaling. *Decision +
   cost.*
8. **Ergonomics** (§15 #8). F1/F8 Fn-layer risk on mini-PC keyboards; a configurable
   or better key choice. *Design.*
9. **Icon-resource drift** (§15 #12). CI guard so `Prata.ico` and the committed
   `rsrc_windows_amd64.syso` can't drift. *Low.*

### Lower / hardening (from the triage's lower-severity items)
10. **Audio-device re-open after device change/invalidation** mid-session — graceful
    today, but a stuck wrong default could persist; explicit handling would help.
11. **Browser-chrome focus consuming injected text** — focus landing on the address
    bar or a Chrome control instead of the field; worth a targeted check.
12. **Per-user dictionary corruption / oversized override / PHI guard** — F8 writes a
    plaintext correction file; a stray patient-specific correction would apply to all
    dictations. A size/sanity guard would help.
13. **Silence-threshold tuning** — if a real dictation is ever dropped, `silencePeakFloor`
    (512) can be lowered; the dropped peak is already logged for exactly this.
14. **Minor**: UIPI elevated-target pre-check; partial/truncated-JSON hardening;
    no-execution-fallback doc clarification.
