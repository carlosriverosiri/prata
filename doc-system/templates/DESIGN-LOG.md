# DESIGN-LOG — <PROJECT>

> **Role: SOURCE.** The dated, append-only narrative of *how the thinking evolved* —
> the reasoning dialogue that the spine docs point to but do not carry. `CONSTANTS`,
> `DECISIONS-REJECTED`, and the Decision Records hold the *conclusions*; this log
> holds the *story that produced them*: the symptom, the hypotheses (including the
> wrong ones), the experiment that settled it, and the `REJ-NNN` / `DR-NNNN` it fed.
>
> This is the asset a future model cannot reconstruct from the outcome alone. Append
> at the bottom; **never rewrite history** — correct a past entry with a new dated one.

<!-- freshness: valid-as-of <git-short-sha> (<tag>) <ISO-date> — stamp on release -->

## How to use this log

- One entry per reasoning-heavy session or investigation. Newest at the bottom.
- Reference IDs (`REJ-NNN`, `DR-NNNN`, `INV-name`) instead of re-explaining them.
- When an entry settles a dead end, add the `REJ-NNN` row in `DECISIONS-REJECTED.md`
  in the **same commit** (`AGENTS.md` §2) and cite it here.
- Cheap capture: end a session with *"append a DESIGN-LOG entry for what we just
  worked through, using the template — include the wrong turns and the experiment
  that settled it."* The AI was in the dialogue; the context is free.

---

## <YYYY-MM-DD> (<git-short-sha> / <tag>) — <short topic>

- **Symptom / question:** <the concrete thing that forced the investigation — quote
  the real repro, error, or measurement, not a paraphrase>.
- **What we believed going in:** <the starting hypothesis, even if it turned out
  wrong — the wrong belief is the valuable part>.
- **What we tried, in order:**
  1. <attempt> → <result>.
  2. <attempt> → <result>.
- **The experiment that settled it:** <name the throwaway harness / test / measurement
  and its one-line result — this is what converts a belief into re-try-proof knowledge>.
- **Conclusion:** <what we now hold true, stated as a rebuilder could act on it>.
- **Produced:** <`DR-NNNN`, `REJ-NNN` rows, `CONSTANTS` entries, `INV-name` this set>.
- **Still open:** <anything unresolved — becomes the next entry's starting point, or a
  question thread in the review doc>.

---

*Role: SOURCE. Dated narrative; the spine docs (`CONSTANTS`, `DECISIONS-REJECTED`,
Decision Records) carry the conclusions. See `AGENTS.md` §1 for the doc map.*
