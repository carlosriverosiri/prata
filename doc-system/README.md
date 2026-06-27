# doc-system — an AI-rebuildable documentation kit

A small, **tech-stack-agnostic** documentation system you drop into any project so
that a future, more capable AI — with little knowledge of the old code — can read
the docs and **rebuild the software**, including *why* each decision was made and
what was *tried and rejected*.

> **Provenance.** This kit is distilled from an audit of the Prata project, where a
> 16-agent reconstruction test asked: *"Could a future model rebuild each subsystem
> from the docs alone?"* Prata scored ~80%. The shortfall was never missing
> *content* — it was **structure**: load-bearing constants that lived only in code,
> rejected-path reasoning buried in chronological prose, and a freshness rule with
> no enforcement. This kit packages what scored 8–9/10 and fixes exactly those gaps.

## The six principles (why the kit is shaped this way)

1. **One job per document; one fact in one place.** Every doc declares `Role:
   SOURCE` or `Role: DERIVED`. A derived doc may be stale and must say so.
2. **WHY lives inline at the altitude of the fact; the deep WHY lives in a Decision
   Record.** The spine carries conclusions + one-line rationale and points to the DR
   for the dialogue.
3. **Negative knowledge is a first-class artifact, not prose.** Rejected options are
   *mandatory* fields in a Decision Record, and every dead end gets a row in the
   register with a `Status` and a `Re-try trigger`.
4. **Every rebuild-load-bearing constant has one home, and it is a doc, not a code
   comment.** If a magic number only lives in code, a rebuilder must guess it.
5. **Freshness needs a machine-checkable marker and a specified detector — not a
   discipline.** The kit ships both.
6. **Identity facts (module path, endpoints, canonical names) are pinned once,
   explicitly — including when deliberately absent.**

## The document set

| File | Tier | Role | Single job |
| --- | --- | --- | --- |
| `PROJECT-IDENTITY.md` | spine | SOURCE | Un-guessable identity: names, module/package path, the one build command, deliberately-absent secrets + where they live, literal-reader traps. |
| `MASTER.md` | spine | SOURCE | The synthesized "what it is + how it was reasoned", at a glance. |
| `AGENTS.md` | spine | SOURCE | How to work here: the doc map, the **freshness rule + routing table + negative list**, the NEVER list. |
| `CONSTANTS.md` | spine | SOURCE | Registry of every load-bearing constant: value, source, *why this value*. |
| `DECISIONS-REJECTED.md` | spine | SOURCE | Negative-knowledge register: rejected paths with `Status` + `Re-try trigger`. |
| `DECISION-RECORD.md` | spine | SOURCE | The per-decision template (Context → Options incl. **mandatory rejected** → Decision → Reasoning dialogue → Constants → Invariants). |
| `DESIGN-LOG.md` | spine | SOURCE | Dated, append-only reasoning narrative — the dialogue the conclusions came from (what the Decision Records and `DECISIONS-REJECTED` point back to). |
| `CHANGELOG.md` | record | SOURCE | Keep-a-Changelog history. |
| `HANDOFF.md` | transient | TRANSIENT | Ephemeral cross-session brief; self-deletes; carries a `valid-as-of` stamp. |

`MASTER` is the orientation doc; `DECISIONS-*` are the negative-knowledge spine;
`CONSTANTS` closes the "must-guess" gap; `PROJECT-IDENTITY` removes day-1 guesses.

## How to use it

1. **New project, day 1:** follow `BOOTSTRAP.md` — copy `templates/` to the repo
   root, fill the `<PLACEHOLDER>` tokens, and seed the first Decision Record with
   "why this stack" (and what you rejected).
2. **Every change:** apply the same-run rule in `AGENTS.md` §2 — update the routed
   doc *in the same commit* as the code. The AI coder does this; you approve.
3. **Freshness:** wire `freshness/check-docs.sh` as a pre-commit warning and a CI
   gate, and keep `freshness/doc-asserts.txt` (seeded from `doc-asserts.example.txt`)
   filled with your real load-bearing facts. The detector also sanity-checks each
   spine doc's `valid-as-of` stamp against HEAD (Layer C), so the freshness marker is
   enforced rather than decorative. Optionally add a **context-pack generator**
   (`freshness/CONTEXT-PACK-SPEC.md`) — Prata ships a stdlib-Go reference impl at
   `cmd/gen-context-pack/` that auto-extracts constants from code and a CI job that
   fails on drift.

## Minimal viable adoption (don't start with all nine files)

The full set pays off on a project that will outlive its first month. For a spike, a
throwaway, or a sub-1-week tool, seed only the two highest-leverage docs and skip the
rest until the project earns them:

- `PROJECT-IDENTITY.md` — the un-guessable facts (module path, build command, traps).
- `DECISIONS-REJECTED.md` — the dead ends, so you don't re-tread them next week.

Add `MASTER.md` + `CONSTANTS.md` + `AGENTS.md` and wire the freshness hooks the moment
the spike turns into something you'll hand off or come back to. Starting small beats
not starting; the system is additive.

## What this kit deliberately is NOT

- **Not a Notion/wiki system.** The consumer is an AI rebuilding from the repo; the
  source of truth must sit next to the code and move in the same commit. Use Notion,
  if at all, as a thin *portfolio index* (one row per project, pointers only).
- **Not an ADR tool.** The hand-written Decision Record format here outscored
  tooling in the audit; borrow the format, skip the build step.

---

*Tier meaning for a rebuilding AI: read the **spine** to rebuild, **records** for
history, ignore **transient** if its stamp is far behind HEAD.*
