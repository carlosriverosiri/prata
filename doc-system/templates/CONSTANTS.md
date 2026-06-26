# CONSTANTS — <PROJECT>

> **Role: SOURCE.** Every value here is **load-bearing for a rebuild**: a future AI
> cannot invent a project-specific magic number it was never told. Rule: **if a
> load-bearing constant lives only in a code comment, that is a documentation bug.**
> When you change a value, change it here in the same commit (`AGENTS.md` §2).

<!-- freshness: valid-as-of <git-short-sha> (<tag>) <ISO-date> -->

| Constant | Value | Source (file:line) | Why this value |
| --- | --- | --- | --- |
| `<name>` | **<value + unit>** | `<path:line>` | <why this number and not another; what breaks if it changes; the decision/DR that set it> |
| `<name>` | **<value>** | `<path:line>` | <…> |

<!-- Examples of what belongs here, by project type:
  web app:   rate limits, cache TTLs, retry/backoff, timeouts, pagination sizes
  CLI:       exit codes, default flags, buffer sizes
  ML:        learning rate, batch size, seed, train/val split, early-stop patience, data-version hash
  any:       queue depths, channel/thread-pool sizes, thresholds, magic timeouts -->

## Facts that are partially code-only (flag them)

> Things whose *behavior* is documented elsewhere but whose exact value/ordering
> still lives in code. List them so a rebuilder knows to read the code for these.

- <e.g. the exact config-file schema ordering, a retry cadence, a struct layout>

---

*Role: SOURCE. Owned by `AGENTS.md` §2 routing ("a constant → CONSTANTS.md").
Consider auto-extracting these from code into a context pack — see
`doc-system/freshness/CONTEXT-PACK-SPEC.md`.*
