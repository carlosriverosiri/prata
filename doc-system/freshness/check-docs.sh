#!/usr/bin/env bash
# check-docs.sh — the doc-freshness detector this kit specifies (Prata's audit
# found the freshness *rule* was documented but the *detector* never was).
#
# Two layers, portable (bash + git + grep, no extra tooling):
#
#   Layer A — staleness pairing (ADVISORY, never blocks): if the change touches
#   code but no paired SOURCE doc, warn. A blocking pairing hook gets bypassed
#   reflexively and trains the wrong habit, so this only warns.
#
#   Layer B — load-bearing fact assertions (HARD, blocks): run doc-asserts.txt;
#   each check is a shell command that must exit 0. This is what catches a doc
#   claim drifting from code/config (e.g. a module path or a constant).
#
# Usage:
#   ./check-docs.sh                 # pre-commit: staged files, else working tree
#   BASE_REF=origin/master ./check-docs.sh   # CI: diff a base..HEAD range
#   ASSERTS_FILE=doc-asserts.txt ./check-docs.sh
#
# Exit: non-zero iff a Layer-B assertion fails. Layer A only prints warnings.

set -uo pipefail

# --- config (override via env) -------------------------------------------------
# Extended-regex (grep -E) over changed file paths.
CODE_GLOBS="${CODE_GLOBS:-(^|/)(src|cmd|internal|lib|app|pkg)/.*\.(go|rs|ts|tsx|js|py|java|kt|rb|c|cc|cpp|h)$}"
DOC_GLOBS="${DOC_GLOBS:-(MASTER|CONSTANTS|DECISIONS-REJECTED|CHANGELOG|PROJECT-IDENTITY|DESIGN-LOG)\.md$|(^|/)DECISIONS/}"
ASSERTS_FILE="${ASSERTS_FILE:-doc-asserts.txt}"

fail=0

# --- which files changed -------------------------------------------------------
if [ -n "${BASE_REF:-}" ]; then
  changed="$(git diff --name-only "${BASE_REF}...HEAD")"
elif ! git diff --cached --quiet 2>/dev/null; then
  changed="$(git diff --cached --name-only)"
else
  changed="$(git diff --name-only HEAD 2>/dev/null)"
fi

# --- Layer A: staleness pairing (advisory) ------------------------------------
if [ -n "${changed}" ]; then
  code_hit="$(printf '%s\n' "${changed}" | grep -E "${CODE_GLOBS}" || true)"
  doc_hit="$(printf '%s\n'  "${changed}" | grep -E "${DOC_GLOBS}"  || true)"
  if [ -n "${code_hit}" ] && [ -z "${doc_hit}" ]; then
    echo "⚠️  doc-freshness: code changed but no SOURCE doc was touched."
    echo "    Apply the same-run rule (AGENTS.md §2) or confirm this is refactor-only."
    echo "    Changed code:"
    printf '      %s\n' ${code_hit}
    echo "    (advisory — not blocking; bypass a hook with --no-verify)"
  fi
fi

# --- Layer B: load-bearing fact assertions (hard) -----------------------------
if [ -f "${ASSERTS_FILE}" ]; then
  claim=""
  while IFS= read -r line || [ -n "${line}" ]; do
    case "${line}" in
      '# claim:'*) claim="${line#'# claim:'}"; claim="${claim# }" ;;
      ''|'#'*)     : ;;                      # blank or other comment
      *)
        if eval "${line}" >/dev/null 2>&1; then
          : # assertion holds
        else
          echo "❌ doc-asserts: FAILED — ${claim:-<unlabelled>}"
          echo "    check: ${line}"
          fail=1
        fi
        claim=""
        ;;
    esac
  done < "${ASSERTS_FILE}"
else
  echo "ℹ️  doc-freshness: no ${ASSERTS_FILE} found — skipping fact assertions."
  echo "    Seed it from doc-system/freshness/doc-asserts.example.txt."
fi

if [ "${fail}" -ne 0 ]; then
  echo
  echo "A documented fact no longer matches the code/config. Fix the doc or the code"
  echo "so they agree, then re-run."
  exit 1
fi

echo "✅ doc-freshness: assertions hold."
exit 0
