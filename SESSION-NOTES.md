# Prata — Session Notes

> A running, dated log of where each working session left off — the
> "what were we doing last?" anchor for picking work back up across
> machines and AI sessions. The durable record of *what changed* lives in
> `CHANGELOG.md`; the *why* in `PRATA-DESIGN-LOG.md`; the *what's next* in
> `ROADMAP.md`. This file is the lightweight bridge between sessions.
>
> Newest entry on top.

---

## 2026-06-30 — state at session start

**Branch:** `master` is clean; v0.6.0 shipped 2026-06-25. No work in flight.

### What we did most recently (newest first)

1. **Clipboard paste no longer risks pasting your *previously copied*
   content** (`internal/inject`, commit `486516b`). Live repro: dictating
   into a non-allowlisted (clipboard-paste) app right after copying a
   Markdown document pasted the **old document** instead of the dictation.
   Cause: `SendInput`/Ctrl+V is asynchronous, so a cold/slow target read the
   clipboard *after* `Type` had restored the prior content. Fix: `Type` no
   longer restores the prior clipboard — it **clears it** after the paste
   settles, so the clipboard only ever goes dictation → empty and the old
   content can't be re-published into the race. Worst residual is a silent
   empty paste (re-dictate), the patient-safe direction. `pasteSettleDelay`
   400 → 700 ms as defense-in-depth. See PRATA-DESIGN-LOG (2026-06-29),
   DECISIONS-REJECTED REJ-050 (locked) / REJ-051 (deferred).

2. **AI-rebuildable documentation system** (#13, commit `01e9696`). Added
   `PROJECT-IDENTITY.md`, `CONSTANTS.md`, `DECISIONS-REJECTED.md`, plus
   `cmd/gen-context-pack` which assembles `CONTEXT-PACK.md` with pinned
   facts auto-extracted from the code. A CI job regenerates it and fails on
   `git diff`, so doc-vs-code drift is caught automatically. Closes the gap
   an audit found (a future AI scored ~80% rebuilding from docs alone; the
   shortfall was constants and rejected-path reasoning).

3. **Staleness window now scales with dictation length** (`cmd/prata`,
   commit `6477704`). `maxInjectAge` (8 s, calibrated on Berget's fast
   transcription) produced false "tog för lång tid" drops on the slower
   local GPU server, where a long dictation legitimately takes 8–10 s. The
   window now keeps 8 s as a floor for short taps and grows to ×2 of the
   spoken length, capped at 30 s. See PRATA-DESIGN-LOG (2026-06-28).

### Closest candidates on deck (from ROADMAP)

- The "see and forget" health signal (§15 #14) is **done** except a
  low-value startup mic probe.
- **Transport security + response authenticity** (§15 #13) is the top
  open item: the LAN GPU is plaintext HTTP, no auth, response injected with
  no integrity check. Needs a GPU-server-side change and a network-trust
  decision — **not a solo engineering call.**
- Lower-effort hardening candidates: audio-device re-open after device
  change (#10), browser-chrome focus check (#11), per-user dictionary
  size/PHI guard (#12).

_No code changes this session — notes only._
