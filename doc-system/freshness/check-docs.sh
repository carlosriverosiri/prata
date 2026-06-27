#!/usr/bin/env bash
# check-docs.sh — the doc-freshness detector this kit specifies (Prata's audit
# found the freshness *rule* was documented but the *detector* never was).
#
# Three layers, portable (bash + git + grep, no extra tooling):
#
#   Layer A — staleness pairing (ADVISORY, never blocks): if the change touches
#   code but no paired SOURCE doc, warn. A blocking pairing hook gets bypassed
#   reflexively and trains the wrong habit, so this only warns.
#
#   Layer B — load-bearing fact assertions (HARD, blocks): run doc-asserts.txt;
#   each check is a shell command that must exit 0. This is what catches a doc
#   claim drifting from code/config (e.g. a module path or a constant).
#
#   Layer C — freshness-stamp sanity (ADVISORY, never blocks): warn when a spine
#   doc's `valid-as-of <sha>` stamp is not an ancestor of HEAD or has fallen far
#   behind it — i.e. the stamp is lying about how current the doc is. Unfilled
#   `<git-short-sha>` placeholders are skipped, so a freshly scaffolded repo (and
#   this kit's own templates/) stays quiet.
#
# SECURITY / trust boundary: Layer B runs `eval` on each non-comment line of the
# asserts file with full shell privileges. Treat doc-asserts.txt as TRUSTED, code-
# reviewed code — never point ASSERTS_FILE at an untrusted/attacker-controlled file.
# (Shell pipelines in an assertion are a deliberate feature; the trade is trust.)
#
# Usage:
#   ./check-docs.sh                          # pre-commit: staged files, else working tree
#   BASE_REF=origin/main ./check-docs.sh     # CI: diff a base..HEAD range
#   ASSERTS_FILE=doc-asserts.txt ./check-docs.sh
#   FRESHNESS_MAX_BEHIND=50 ./check-docs.sh  # Layer C: commits-behind warn threshold
#
# Canonical invocation is `bash check-docs.sh` (do not rely on the +x bit surviving
# a Windows clone; ship .gitattributes `*.sh text eol=lf` so the shebang is not CRLF).
#
# Exit: non-zero iff a Layer-B assertion fails. Layers A and C only print warnings.

set -uo pipefail

# --- config (override via env) -------------------------------------------------
# Extended-regex (grep -E) over changed file paths.
# CODE_GLOBS is intentionally BROAD (any source extension, any directory) so flat
# Python/TS/Rust layouts trigger the advisory too; narrow it per-project via the env
# var if a vendored/generated tree produces noise. It is advisory-only, so a false
# positive never blocks a commit.
CODE_GLOBS="${CODE_GLOBS:-\.(go|rs|ts|tsx|js|jsx|mjs|py|java|kt|rb|c|cc|cpp|h|hpp|cs|php|swift|scala|clj|ex|exs)$}"
DOC_GLOBS="${DOC_GLOBS:-(AGENTS|MASTER|CONSTANTS|DECISIONS-REJECTED|DECISION-RECORD|CHANGELOG|PROJECT-IDENTITY|DESIGN-LOG)\.md$|(^|/)DECISIONS/}"
ASSERTS_FILE="${ASSERTS_FILE:-doc-asserts.txt}"
FRESHNESS_MAX_BEHIND="${FRESHNESS_MAX_BEHIND:-50}"

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
    printf '%s\n' "${code_hit}" | sed 's/^/      /'
    echo "    (advisory — does not block the commit)"
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

# --- Layer C: freshness-stamp sanity (advisory) -------------------------------
# Scope: SOURCE spine docs, by basename. EXEMPT templates/ and examples/ (they ship
# placeholders on purpose) and the transient HANDOFF.md (it is *meant* to go stale).
if git rev-parse --git-dir >/dev/null 2>&1; then
  spine="$(git ls-files \
        'AGENTS.md' '*/AGENTS.md' \
        'MASTER.md' '*MASTER.md' \
        'CONSTANTS.md' '*/CONSTANTS.md' \
        'DECISIONS-REJECTED.md' '*/DECISIONS-REJECTED.md' \
        'PROJECT-IDENTITY.md' '*/PROJECT-IDENTITY.md' \
        'CHANGELOG.md' '*/CHANGELOG.md' \
        'DESIGN-LOG.md' '*DESIGN-LOG.md' 2>/dev/null \
      | grep -Ev '(^|/)(templates|examples)/' | sort -u || true)"
  while IFS= read -r doc; do
    [ -n "${doc}" ] || continue
    [ -f "${doc}" ] || continue
    sha="$(grep -oE 'valid-as-of(-commit)?[: ]+[0-9a-f]{7,40}' "${doc}" 2>/dev/null \
            | head -n1 | grep -oE '[0-9a-f]{7,40}$' || true)"
    [ -n "${sha}" ] || continue            # unfilled <git-short-sha> placeholder ⇒ skip
    if ! git cat-file -e "${sha}^{commit}" 2>/dev/null; then
      echo "⚠️  doc-freshness: ${doc} stamped valid-as-of ${sha}, not a commit in this repo (re-stamp on release)."
      continue
    fi
    if ! git merge-base --is-ancestor "${sha}" HEAD 2>/dev/null; then
      echo "⚠️  doc-freshness: ${doc} stamped ${sha}, which is not an ancestor of HEAD (re-stamp on release)."
      continue
    fi
    behind="$(git rev-list --count "${sha}..HEAD" 2>/dev/null || echo 0)"
    if [ "${behind:-0}" -gt "${FRESHNESS_MAX_BEHIND}" ]; then
      echo "⚠️  doc-freshness: ${doc} stamp is ${behind} commits behind HEAD (> ${FRESHNESS_MAX_BEHIND}); confirm it is still current."
    fi
  done <<EOF
${spine}
EOF
fi

if [ "${fail}" -ne 0 ]; then
  echo
  echo "A documented fact no longer matches the code/config. Fix the doc or the code"
  echo "so they agree, then re-run."
  exit 1
fi

echo "✅ doc-freshness: assertions hold."
exit 0
