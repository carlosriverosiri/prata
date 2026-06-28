# PROJECT-IDENTITY — Prata

> **Role: SOURCE.** The un-guessable identity facts a rebuilder needs *first* and
> cannot infer from prose: canonical names, the module path, the one build
> command, and the facts that are *deliberately absent* from the docs (and where
> they really live). Pin one fact in exactly one place. Keep it tiny.
>
> *Why this file exists:* an AI rebuilding Prata from the docs alone was found to
> guess the module path (the docs only showed the GitHub URL, which differs) and
> to mis-purpose a package (see "Known traps"). This file removes both classes of
> guess.

## Canonical names

| Fact | Value | Notes |
| --- | --- | --- |
| Product name | **Prata** | Swedish for "talk / chat". |
| Go module path | **`github.com/carlosriveros/prata`** | Declared in `go.mod`. **Load-bearing for import paths.** |
| GitHub repository | **`carlosriverosiri/prata`** | The repo *slug* is `carlosriverosiri` — it deliberately **differs** from the module path's `carlosriveros`. Both are correct; they are not the same string. A rebuilder must use the **module path** above for imports, not the repo URL. |
| Primary binary | `prata.exe` | Single binary: daemon + `--install` / `--uninstall` / `--set-key`. |
| Sibling project | Diktell | Finished and frozen; runs on GPU machines. Prata targets machines *without* a GPU. Not a version of Prata — a sibling. |

## Platform & toolchain

| Fact | Value |
| --- | --- |
| Language | Go **1.26** (`go.mod` pins `go 1.26.3`). |
| Build constraint | **Windows-only**, `CGO_ENABLED=1` (the `malgo` audio dependency uses cgo → needs a C compiler: MinGW-w64 / TDM-GCC). |
| Only external dependency | `github.com/gen2brain/malgo v0.11.25` (audio capture). Everything else is stdlib + hand-rolled Win32. |

## The one build command (production)

```
CGO_ENABLED=1 go build -ldflags="-s -w -H windowsgui -X main.version=<tag>" -o prata.exe ./cmd/prata
```

Verification gate (mirror of `.github/workflows/ci.yml`, runs on `windows-latest`):

```
gofmt -l .            # must print nothing
CGO_ENABLED=1 go vet ./...
go build ./...
go test ./... -count=1
```

> **Note for non-Windows machines (e.g. CI Linux runners):** the daemon and most
> `internal/` packages do **not** compile off Windows (Win32 syscalls + cgo). Only
> pure-stdlib, OS-independent packages such as `cmd/gen-context-pack` build on
> Linux. The docs-freshness CI job relies on that fact.

## Deliberately absent from the docs (and where the real values live)

These are omitted on purpose — do **not** treat their absence as a documentation
gap, and do **not** invent values:

| Absent fact | Why absent | Where it really lives |
| --- | --- | --- |
| Berget AI **API key** | Secret. DPAPI-encrypted per user. | `%LOCALAPPDATA%\Prata\apikey.dat`; set via `prata --set-key`. Never committed, never logged. |
| GPU-server **endpoint URLs** | Operational / confidentiality. | `CONSTANTS.md` (the hard-coded constants) + `PRATA-GPU-SERVER.md` (topology). They are compile-time constants, not config. |
| Clinic **network addressing** | Site-specific. | `PRATA-GPU-SERVER.md`. |

## Known traps (facts a literal reader gets wrong)

1. **Two-step backend default.** `transcribe.NewClient()` constructs with the
   **Berget** backend as its in-code default, but `main` immediately calls
   `loadBackendPref()` → `SetBackend(Work)` at startup. The **observable** first-run
   default is therefore **LAN GPU-server (`Jobb`)**, *not* Berget. A rebuilder who
   codes `NewClient`'s default straight from prose will ship the wrong startup
   backend. The two-step is load-bearing.
2. **`internal/sanity` is the degenerate-output guard**, *not* "startup
   self-checks". It rejects Whisper repetition loops (gzip ratio + phrase-loop)
   before they reach the journal — a patient-safety feature. (An older `AGENTS.md`
   §5 label called it "startup self-checks"; that was wrong and is corrected.)
3. **Stable backend IDs are decoupled from display names.** Persisted choice is the
   ID (`Hemma` / `Jobb` / `Berget`) in `backend.txt`; the display name can change
   without breaking a saved choice.

---

*Role: SOURCE. Pin facts here once. See `AGENTS.md` §1 for the full doc map.*
<!-- freshness: valid-as-of <git-short-sha> (<tag>) <ISO-date> — stamp on release -->
