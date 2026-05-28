#Requires -Version 5.1

<#
.SYNOPSIS
    Installs Prata to %LOCALAPPDATA%\Prata and registers it for autostart.

.DESCRIPTION
    Downloads the latest release from GitHub (or builds and copies from
    the current directory with -Local), encrypts your Berget API key
    with DPAPI, and registers a Task Scheduler entry that launches
    Prata at login.

.PARAMETER Local
    Build binaries from the current source tree and copy them to the
    install location, instead of downloading a release from GitHub.
    Use this for development / installer smoke-testing.

.EXAMPLE
    iwr https://raw.githubusercontent.com/carlosriverosiri/prata/master/install.ps1 | iex

.EXAMPLE
    .\install.ps1 -Local
#>

param(
    [switch]$Local
)

$ErrorActionPreference = "Stop"

Write-Host ""
Write-Host "=== Prata installer ===" -ForegroundColor Cyan
Write-Host ""

$InstallDir = Join-Path $env:LOCALAPPDATA "Prata"
Write-Host "Install location: $InstallDir"

if (-not (Test-Path $InstallDir)) {
    New-Item -ItemType Directory -Path $InstallDir | Out-Null
}

$Files = @("prata.exe", "prata-setkey.exe", "dictionary-corrections.txt")

if ($Local) {
    Write-Host "Source: current directory (local mode)"
    Write-Host ""
    Write-Host "Building prata.exe (windowsgui, stripped)..."
    & go build -ldflags="-s -w -H windowsgui" -o prata.exe ./cmd/prata/
    if ($LASTEXITCODE -ne 0) { Write-Error "go build prata.exe failed" }

    Write-Host "Building prata-setkey.exe..."
    & go build -ldflags="-s -w" -o prata-setkey.exe ./cmd/prata-setkey/
    if ($LASTEXITCODE -ne 0) { Write-Error "go build prata-setkey.exe failed" }

    foreach ($File in $Files) {
        if (-not (Test-Path $File)) {
            Write-Error "Missing $File in current directory"
        }
        Copy-Item $File (Join-Path $InstallDir $File) -Force
        Write-Host "  copied $File"
    }
} else {
    $Repo = "carlosriverosiri/prata"
    Write-Host "Source: github.com/$Repo (latest release)"

    $ApiUrl = "https://api.github.com/repos/$Repo/releases/latest"
    $Release = Invoke-RestMethod -Uri $ApiUrl
    $Version = $Release.tag_name
    Write-Host "Version: $Version"

    foreach ($File in $Files) {
        $Asset = $Release.assets | Where-Object { $_.name -eq $File }
        if (-not $Asset) {
            Write-Error "Asset $File not found in release $Version"
        }
        $Dest = Join-Path $InstallDir $File
        Write-Host "  downloading $File..."
        Invoke-WebRequest -Uri $Asset.browser_download_url -OutFile $Dest
    }
}

$KeyPath = Join-Path $InstallDir "apikey.dat"
if (Test-Path $KeyPath) {
    Write-Host ""
    Write-Host "An encrypted API key already exists at $KeyPath"
    $Reuse = Read-Host "Keep it? (Y/n)"
    $SetKey = ($Reuse -eq "n" -or $Reuse -eq "N")
} else {
    $SetKey = $true
}

if ($SetKey) {
    Write-Host ""
    $PlainKey = Read-Host "Enter your Berget API key"
    & (Join-Path $InstallDir "prata-setkey.exe") $PlainKey
    Write-Host ""
}

Write-Host "Registering autostart at login..."
$TaskName = "Prata"
$ExePath = Join-Path $InstallDir "prata.exe"

Unregister-ScheduledTask -TaskName $TaskName -Confirm:$false -ErrorAction SilentlyContinue

$Action = New-ScheduledTaskAction -Execute $ExePath
$Trigger = New-ScheduledTaskTrigger -AtLogOn -User $env:USERNAME
$Settings = New-ScheduledTaskSettingsSet `
    -AllowStartIfOnBatteries `
    -DontStopIfGoingOnBatteries `
    -StartWhenAvailable `
    -ExecutionTimeLimit ([TimeSpan]::Zero)
$Principal = New-ScheduledTaskPrincipal `
    -UserId $env:USERNAME `
    -LogonType Interactive `
    -RunLevel Limited

Register-ScheduledTask `
    -TaskName $TaskName `
    -Action $Action `
    -Trigger $Trigger `
    -Settings $Settings `
    -Principal $Principal | Out-Null

Write-Host "  task '$TaskName' registered"

Write-Host ""
Write-Host "=== Installation complete ===" -ForegroundColor Green
Write-Host ""
Write-Host "Prata will start automatically at your next login."
Write-Host ""
Write-Host "Start it now without waiting for a re-login:"
Write-Host "  Start-ScheduledTask -TaskName Prata"
Write-Host ""
Write-Host "Uninstall:"
Write-Host "  Unregister-ScheduledTask -TaskName Prata -Confirm:`$false"
Write-Host "  Remove-Item -Recurse $InstallDir"
Write-Host ""
