# AGENTS.md — how to work on <PROJECT>

> Read this at the start of every session. It says **how to work here**.
> `MASTER.md` says **what this is**. Read both before changing anything.
> Replace every `<PLACEHOLDER>`. Keep this file in <DOC-LANGUAGE, e.g. English>.

<!-- freshness: valid-as-of <git-short-sha> (<tag>) <ISO-date> — stamp on release -->

## Current Truth (orient here first)

- <PROJECT> is <one sentence>. Source of truth: `MASTER.md`.
- Canonical identity (module/package path, repo, build cmd): `PROJECT-IDENTITY.md`.
- Stack: <language/runtime>. Only external dependency/-ies: <list, or "none">.

## 1. Documentation map — ONE job per file

| File | Role | Job |
| --- | --- | --- |
| `PROJECT-IDENTITY.md` | SOURCE | un-guessable identity facts + traps |
| `MASTER.md` | SOURCE | what it is + how it was reasoned |
| `CONSTANTS.md` | SOURCE | every load-bearing constant + why |
| `DECISIONS-REJECTED.md` | SOURCE | rejected paths + Status + Re-try trigger |
| `DECISION-RECORD.md` (→ `DR-NNNN`) | SOURCE | one record per decision: Context → Options incl. **rejected** → Decision → reasoning |
| `DESIGN-LOG.md` | SOURCE | dated, append-only reasoning narrative (the dialogue behind the decisions) |
| `CHANGELOG.md` | SOURCE | user-visible / work history |
| `HANDOFF.md` | TRANSIENT | ephemeral continuation brief (deletable) |
| `<DERIVED-DOC>.md` | DERIVED — **not** a source of truth, may be stale | <job> |

## 2. The single most important rule — keep the docs fresh

Docs drift the moment behavior changes and no one updates them. Therefore:

**RULE: doc updates happen in the SAME commit/agent-run as the code change. Never
defer, never "I'll note it later." Code changed with no matching doc edit is
incomplete.**

Routing — when you change X, update Y in the same run:

| Change | Update |
| --- | --- |
| Observable behavior / feature / user-flow | `MASTER.md` |
| A decision, or a path you **rejected** | a Decision Record + a `REJ-NNN` row in `DECISIONS-REJECTED.md` |
| The **reasoning** behind an investigation (incl. the wrong turns) | a dated `DESIGN-LOG.md` entry |
| A load-bearing **constant / threshold** | `CONSTANTS.md` (never leave it only in a code comment) |
| An **identity fact** (module path, endpoint, canonical name) | `PROJECT-IDENTITY.md` |
| Infra / backend / network / deploy | `<INFRA-DOC>.md` |
| Any user-visible change | `CHANGELOG.md` `[Unreleased]` |

**What does NOT require a doc update** (this negative list is what keeps the rule
survivable rather than nagging):

- Pure refactor with no behavior change · formatting · comments/typos · tests ·
  variable renames · dependency bumps that don't change behavior.

If unsure whether a change is "observable": it is. Update `MASTER.md`.

## 3. NEVER do without explicit confirmation

- <destructive/irreversible actions specific to this project>
- Never delete a Decision Record — supersede it (`status: SUPERSEDED-BY:DR-NNNN`).
- Never put a load-bearing constant only in a code comment.
- Never let `MASTER.md` and a Decision Record disagree without resolving which is SOURCE.

## 4. Verification gate (before declaring a task done)

```
<the project's format/lint/build/test commands — mirror CI>
```

## 5. Mirror the freshness rule into the IDE's always-on path

Put a 3–5 line always-on rule (e.g. `.cursor/rules/…`, `.github/copilot-instructions.md`)
that points to §2 here — duplicate only the one sentence agents forget most ("update
the routed doc in the same commit as the code"), not the whole table.

---

*Role: SOURCE. Language: <DOC-LANGUAGE>.*
