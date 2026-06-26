# DECISIONS-REJECTED — <PROJECT>'s negative-knowledge register

> **Role: SOURCE.** The dead ends, as a scannable index. Both what was rejected
> *and the reasoning* are first-class — a future AI must not re-tread paths already
> disproved. Full narratives live in the Decision Records / design log; this file
> adds the two fields prose lacks: **Status** and **Re-try trigger**.

<!-- freshness: valid-as-of <git-short-sha> (<tag>) <ISO-date> -->

## How to read this

- **Status:** `LOCKED` (never revisit) · `DISPROVEN` (an experiment settled it) ·
  `INEFFECTIVE` (tried, structural failure) · `BUILT-THEN-DROPPED` ·
  `DEFERRED` (good idea, parked) · `SUPERSEDED-BY <id>`.
- **Re-try trigger:** the exact precondition under which a parked path is worth
  reconsidering. `none` = do not revisit. *This is the load-bearing field.*
- IDs (`REJ-NNN`) are stable; Decision Records and the design log reference them.

## Register (scan this first)

| ID | Rejected / failed path | Class | Status | Re-try trigger |
| --- | --- | --- | --- | --- |
| REJ-001 | <path> | <class> | <status> | <trigger or none> |
| REJ-002 | <path> | <class> | <status> | <trigger or none> |

Class legend: `architecture · safety-invariant · safety-mechanism · implementation · wrong-hypothesis · dependency · ergonomics · process`.

## Detail entries (high-value subset)

> Add a full block only for the entries whose reasoning is most valuable; the rest
> are covered by the index row + the dated design-log narrative.

### REJ-NNN — <title>
- **Date / version:** <…>
- **Class:** <…> · **Status:** <…> · **Re-try trigger:** <…>
- **What was tried:** <…>
- **Why it died / how it was disproven:** <the concrete reason; name the experiment if one killed it>
- **What replaced it:** <…>
- **Lesson:** <the generalizable takeaway>
- **Cross-refs:** <DR / design-log date / review thread>

---

## Maintenance

- Every newly rejected/abandoned/built-then-dropped path gets a `REJ-NNN` index row
  **before the work is done** (`AGENTS.md` §2).
- The design-log narrative references the `REJ-NNN` id rather than re-explaining.
- Keep `Status:` and `Re-try trigger:` greppable — they are the machine-findable
  answer to *"which dead ends are safe to revisit, and when?"*

*Role: SOURCE.*
