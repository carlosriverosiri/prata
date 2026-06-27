# CONTEXT-PACK — specification (tech-stack-agnostic)

A **context pack** is a single, self-describing file that bundles the highest-value
onboarding material for a rebuilding AI: the read order, the identity facts, the
load-bearing constants (extracted *from code*), and the negative-knowledge index.
It is the *compiled* form of the spine docs — not a replacement for committing the
markdown, but a single entry point with code-checked facts and a drift gate.

> Reference implementation: Prata ships a stdlib-Go generator at
> `cmd/gen-context-pack/` and a CI drift gate in `.github/workflows/ci.yml`
> (job `docs`). Port the spec below to any language; only the §3 extraction is
> project-specific.

## Why a generated pack, not just committed markdown

Committed markdown gives you the *content*. A generated pack adds four things the
markdown alone does not:

1. **A single entry point** with the read order (which doc is SOURCE vs DERIVED).
2. **Code-extracted ground-truth facts** — constants/IDs/module-path pulled *from
   the source* with `file:line` provenance, closing the "must-guess" gap.
3. **A verifiable freshness guarantee** — see the drift gate.
4. **A drift gate** — CI regenerates the pack and fails on any diff, so a constant
   that changes in code but not in the pack turns CI red.

## Hard requirement: determinism

The generator output MUST be a pure function of the repository sources — **no
timestamps, no git calls, no randomness**. Determinism is what makes the drift gate
work:

```
<generate> > CONTEXT-PACK.md && git diff --exit-code CONTEXT-PACK.md
```

Same sources → identical output → no diff. Changed constant → diff → CI fails.
(Put any commit SHA / build time *outside* the drift-checked file, or omit it.)

## Structure (sections, in rebuild-priority order)

```
0. PROVENANCE   generator name; "source of truth = this repo, see git log"; the
                statement that the pack is the compiled form of the spine docs.
1. READ ORDER   the doc map, each marked SOURCE / DERIVED / TRANSIENT (authored).
2. IDENTITY     PROJECT-IDENTITY.md, embedded (a rebuilder needs no link-chase).
3. PINNED FACTS auto-extracted table: a CI-checked SUBSET of the highest-churn,
                code-only constants + the module path, each with value and source
                file. THIS is the anti-drift table; `CONSTANTS.md` stays the
                complete registry (do not imply this table is exhaustive).
4. NEGATIVE     the DECISIONS-REJECTED.md index table, embedded, + the count.
   KNOWLEDGE
5. DEEPER       pointers to MASTER / DESIGN-LOG / infra doc for the full reasoning.
```

Sections 2 & 4 are **embedded** (read the files at gen time → CI keeps them fresh).
Section 3 is **extracted from code**. Sections 0, 1, 5 are authored/static.

## §3 extraction — the only project-specific part

A small in-code table of `(label, source-file, regex/AST-query, note)`. Each query
captures one value. Examples by stack:

- **Go:** `regexp` over `const name = value` / `name = N * time.Unit` (see Prata).
  Anchor the query to the **declaration**, not a bare substring, so a comment or test
  can't satisfy it (e.g. `(?:^|[^\w])name\s*=`).
- **TS/JS:** match `export const NAME = …` or read a `config.ts`.
- **Python:** match `NAME = …` or read `pyproject.toml` / a constants module.
- **Config-driven (ML / data):** when the load-bearing values live in YAML/JSON/TOML
  rather than source (learning rate, batch size, seed, data-version hash), read the
  config key directly — the extractor is still "open file, pull one value".
- **Module/package path:** `go.mod` `module …`, `package.json` `"name"`,
  `pyproject.toml` `[project] name`.

If a query finds nothing, emit a visible `⚠️ NOT FOUND` marker **and exit non-zero**
(not merely a stderr note) — otherwise a maintainer can regenerate-and-commit the
broken marker to clear the drift diff, and CI goes green on a pack that lost a fact.

## Minimal wiring

1. One generator program/script (stdlib only — keep it dependency-free).
2. One entry point to run it (a `make`/`just`/`npm` target, or `go run …`).
3. One CI job: regenerate + `git diff --exit-code` the pack. See
   `freshness/ci-drift-gate.example.yml` for a copy-paste job that regenerates into a
   scratch path and `git diff --no-index`-compares it, so a generator crash can never
   truncate the committed baseline.
4. One committed `CONTEXT-PACK.md`.

Optional delivery: a `--export-context` flag on your binary that prints the embedded
pack (e.g. via an embed directive) for a future owner who has the artifact but not
the repo. Build the *generator + gate* first; the flag is a thin reader on top.
