# PROJECT-IDENTITY — <PROJECT>

> **Role: SOURCE.** The un-guessable identity facts a rebuilder needs *first* and
> cannot infer from prose. Pin one fact in exactly one place. Keep it tiny.

<!-- freshness: valid-as-of <git-short-sha> (<tag>) <ISO-date> -->

## Canonical names

| Fact | Value | Notes |
| --- | --- | --- |
| Product name | **<PROJECT>** | |
| Module / package path | **`<exact import root>`** | **Load-bearing.** Pin the exact string a rebuilder imports/installs. Call out any mismatch: import root vs repo URL (Go `go.mod`), distribution name vs import name (Python `pip install X` → `import y`), scoped vs unscoped (npm `@scope/pkg`). |
| Repository | **`<host/owner/repo>`** | |
| Primary artifact | `<binary / entrypoint / package>` | |

## Platform & toolchain

| Fact | Value |
| --- | --- |
| Language / runtime | <e.g. Go 1.26 / Node 22 / Python 3.12> |
| Build constraints | <OS, cgo, native deps> |
| External dependencies | <list, or "none — intentional"> |

## How it is built / run (the one canonical command)

```
<the single canonical build OR run command — e.g. `go build ./...`, `npm run build`,
`python -m <pkg>`, `cargo build --release`. For a library with no build step: the
install + import smoke-test. For ML/data: the train or eval entrypoint.>
```

Verification gate (mirror CI): `<format · lint/typecheck · build-or-import · test>`

## Deliberately absent from the docs (and where the real values live)

> Omitted on purpose — do **not** treat their absence as a gap, do **not** invent values.

| Absent fact | Why absent | Where it really lives |
| --- | --- | --- |
| <secret / API key> | secret | <store + how it's set; never committed> |
| <endpoint / address> | operational / confidentiality | <which doc / constant> |

## Known traps (facts a literal reader gets wrong)

1. <e.g. a two-step default: the constructor default differs from the resolved
   startup value — name the load-bearing two-step>
2. <e.g. a package whose name suggests the wrong purpose>

---

*Role: SOURCE. See `AGENTS.md` §1 for the full doc map.*
