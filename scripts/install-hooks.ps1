<#
.SYNOPSIS
    Installs git hooks for the Prata repo.

.DESCRIPTION
    Installs a NON-BLOCKING pre-commit hook that reminds you to update the
    hand-maintained PRATA-MASTER.md when a notable document changed but
    PRATA-MASTER.md was not staged. The hook never blocks a commit — it only
    prints a reminder. Idempotent: safe to run repeatedly.

    Trigger: a commit that stages CHANGELOG.md, README.md, or
    PRATA-DESIGN-LOG.md but NOT PRATA-MASTER.md. Pure code commits do not fire.

    Remove the hook anytime with: Remove-Item .git\hooks\pre-commit

.EXAMPLE
    .\scripts\install-hooks.ps1
#>

[CmdletBinding()]
param()

Set-StrictMode -Version Latest
$ErrorActionPreference = "Stop"

$scriptDir   = Split-Path -Parent $MyInvocation.MyCommand.Path
$projectRoot = Split-Path -Parent $scriptDir
$hooksDir    = Join-Path $projectRoot ".git\hooks"
$hookFile    = Join-Path $hooksDir "pre-commit"

if (-not (Test-Path $hooksDir)) {
    Write-Error "Could not find .git/hooks/. Are you in the repo root?"
    exit 1
}

if (Test-Path $hookFile) {
    Write-Warning "An existing .git/hooks/pre-commit will be overwritten."
}

# Git on Windows runs hooks via Git Bash (sh.exe), so the hook is a /bin/sh script.
$hookContent = @'
#!/bin/sh
# Prata pre-commit hook — non-blocking PRATA-MASTER.md staleness reminder.
#
# PRATA-MASTER.md is hand-maintained (not generated). If a notable document
# changed but PRATA-MASTER.md was not staged, remind the author. Never blocks.

staged=$(git diff --cached --name-only)

echo "$staged" | grep -Eq '^(CHANGELOG\.md|README\.md|PRATA-DESIGN-LOG\.md)$'
notable=$?

echo "$staged" | grep -qx 'PRATA-MASTER.md'
master=$?

if [ "$notable" -eq 0 ] && [ "$master" -ne 0 ]; then
    echo "" >&2
    echo "  WARNING: PRATA-MASTER.md is not staged, but a notable document changed." >&2
    echo "           If behavior or the project overview changed, update PRATA-MASTER.md too." >&2
    echo "           (Non-blocking reminder — the commit proceeds.)" >&2
    echo "" >&2
fi

exit 0
'@

# Unix line endings are required for Git Bash.
$hookContentUnix = $hookContent -replace "`r`n", "`n"
[System.IO.File]::WriteAllText($hookFile, $hookContentUnix, [System.Text.UTF8Encoding]::new($false))

& git -C $projectRoot config core.hooksPath ".git/hooks" 2>$null

Write-Host "OK  pre-commit hook installed: .git/hooks/pre-commit" -ForegroundColor Green
Write-Host "    Non-blocking. Reminds you to update PRATA-MASTER.md on notable doc commits."
Write-Host "    Remove with: Remove-Item .git\hooks\pre-commit"
