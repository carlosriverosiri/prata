# BOOTSTRAP — stand up the doc system on day 1

Run this **before** the feature code, on commit #1 of a new project. The whole kit
is one prompt to your AI coder plus a few seed files.

> **Spike or throwaway?** You don't need all nine files on day 1 — see *Minimal viable
> adoption* in `README.md`. Seed `PROJECT-IDENTITY.md` + `DECISIONS-REJECTED.md` first
> and add the rest when the project earns them.

## The bootstrap prompt (paste verbatim to your AI coder)

```
Bootstrap the doc-freshness system for this project using doc-system/templates/.
Create at the repo root, with every <PLACEHOLDER> filled from this repo:

1. PROJECT-IDENTITY.md — PIN the exact module/package path, repo, the ONE build
   command, deliberately-absent secrets + where they live, and any literal-reader
   traps (two-step defaults, misleadingly-named modules).
2. AGENTS.md — the doc map (one job per file), the same-run freshness rule +
   routing table + negative list, the verification gate, and the NEVER list.
3. MASTER.md — "what this is", the main flows with inline WHY, components, and the
   IS / is-NOT constraints. Conclusion-altitude; point to Decision Records for the
   full reasoning.
4. CONSTANTS.md — start empty but with the header rule; add a row the moment any
   load-bearing constant is introduced.
5. DECISIONS-REJECTED.md — the register header + an empty index, and seed REJ-001
   with the stack/architecture you rejected on day 1 and WHY.
6. CHANGELOG.md + HANDOFF.md + DESIGN-LOG.md — skeletons, stamped (DESIGN-LOG is the
   dated, append-only home for the reasoning narrative; start it with just the header).
7. The first Decision Record (DECISIONS/DR-0001-stack.md) for "why this stack",
   with the rejected alternatives filled in.

Then wire freshness:
- copy doc-system/freshness/check-docs.sh and doc-asserts.example.txt; fill
  doc-asserts.txt with the canonical module/package path + each load-bearing
  constant as it appears;
- install check-docs.sh as an advisory pre-commit hook AND a CI step;
- mirror the same-run rule into the IDE always-on rule (point to AGENTS §2, don't
  restate the whole table).
Stamp every spine doc header: valid-as-of <git-short-sha> (<tag>) <date>.

Finally, print an ACCEPTANCE REPORT and stop: list every file you created, every
<PLACEHOLDER> you could NOT fill (file + token), and anything you skipped. Do not
start feature code until a human has reviewed this report.
```

## Day-1 invariants to pin immediately

These are exactly the things that drift if you don't pin them up front:

1. **Canonical module/package/repo path** — one exact string in `PROJECT-IDENTITY.md`
   + one `doc-asserts.txt` line. (If the module path can differ from the repo URL,
   say so — that mismatch is a classic rebuilder trap.)
2. **Every load-bearing constant** gets a `doc-asserts.txt` line the moment
   `MASTER`/`CONSTANTS` cites it.
3. **Two-step / non-obvious defaults** get an explicit "trap" note in
   `PROJECT-IDENTITY.md`.
4. **Intentionally-absent info** (secrets, endpoints) is marked absent + where it
   lives — never silently omitted.

## The one-page running checklist

```
PER CHANGE (same commit, AI does it)
  [ ] Changed observable behavior/constant/flow?  -> MASTER.md
  [ ] Made a decision / rejected an option?       -> Decision Record + REJ-NNN row,
                                                      name the throwaway experiment
  [ ] Believed something that turned out false?    -> log it as a rejected hypothesis
  [ ] User-visible?                                -> CHANGELOG.md [Unreleased]
  [ ] New load-bearing constant/path?              -> CONSTANTS.md + doc-asserts line
  [ ] Pure refactor/format/test/comment?           -> NO doc update needed

PER RELEASE
  [ ] CHANGELOG entry   [ ] re-stamp freshness markers (from git)
  [ ] HANDOFF deleted or refreshed+stamped   [ ] drift check green

PERIODIC AUDIT (~N weeks / before a handoff)
  [ ] doc-asserts CI green   [ ] spot-check 5 constants doc-vs-code
  [ ] resolve/confirm OPEN question threads   [ ] stamps near HEAD?
  [ ] re-check DEFERRED re-try triggers — any precondition now true? (DECISIONS-REJECTED)
```

## Cheap-capture habits (so the dialogue gets recorded for free)

- **AI writes the Decision Record, human approves.** End a reasoning-heavy exchange
  with: *"Append a Decision Record for what we just decided, using the template.
  Include what we rejected and why, and name any throwaway experiment that settled
  it."* The AI was in the dialogue — it has the context for free.
- **Log rejected *beliefs*, not just rejected *code*.** A wrong diagnosis that was
  disproven is gold; it stops the next model re-chasing it.
- **Name the throwaway.** When a quick harness settles a hypothesis, record its name
  and one-line result — that converts a belief into re-try-proof negative knowledge.
