#Requires -Version 5.1
<#
.SYNOPSIS
  Read-only prerequisite check for a MasterDrive Windows local demonstration.
.DESCRIPTION
  This script does not start MasterDrive, enumerate user files, change firewall
  settings, modify Tailscale, install software, or persist credentials.
#>
[CmdletBinding()]
param(
    [ValidateRange(1024, 65535)][int]$ControllerPort = 18788,
    [ValidateRange(1024, 65535)][int]$AgentPort = 18789
)

$ErrorActionPreference = 'Stop'
$problems = New-Object 'System.Collections.Generic.List[string]'
$warnings = New-Object 'System.Collections.Generic.List[string]'

Write-Output ('Windows PowerShell version: ' + $PSVersionTable.PSVersion.ToString())
if ($ControllerPort -eq $AgentPort) {
    $problems.Add('ControllerPort and AgentPort must be different.')
}

$repoRoot = Split-Path -Parent $PSScriptRoot
foreach ($required in @('go.mod', 'cmd/masterdrive/main.go', 'web/index.html')) {
    if (-not (Test-Path -LiteralPath (Join-Path $repoRoot $required) -PathType Leaf)) {
        $problems.Add('Required checkout file missing: ' + $required)
    }
}
Write-Output ('MasterDrive checkout: ' + $repoRoot)

$goCommand = Get-Command go -CommandType Application -ErrorAction SilentlyContinue
if ($null -eq $goCommand) {
    $problems.Add('Go executable is not on PATH. Install a current, patched Go 1.27+ release.')
} else {
    try {
        $goVersion = (& $goCommand.Source version 2>&1 | Out-String).Trim()
        Write-Output $goVersion
        $match = [regex]::Match($goVersion, 'go version go([0-9]+)\.([0-9]+)(?:\.([0-9]+))?')
        if (-not $match.Success) {
            $problems.Add('Could not determine the installed Go version.')
        } else {
            $major = [int]$match.Groups[1].Value
            $minor = [int]$match.Groups[2].Value
            if ($major -lt 1 -or ($major -eq 1 -and $minor -lt 27)) {
                $problems.Add('This repository requires Go 1.27 or newer.')
            }
        }
    } catch {
        $problems.Add('Could not run go version: ' + $_.Exception.Message)
    }
}

$tailscaleCommand = Get-Command tailscale -CommandType Application -ErrorAction SilentlyContinue
if ($null -eq $tailscaleCommand) {
    $warnings.Add('Tailscale CLI not found; optional for a local loopback smoke test.')
} else {
    Write-Output ('Tailscale CLI: ' + $tailscaleCommand.Source)
    $tailscaleService = Get-Service -Name 'Tailscale' -ErrorAction SilentlyContinue
    if ($null -ne $tailscaleService) {
        Write-Output ('Tailscale Windows service: ' + $tailscaleService.Status.ToString())
    } else {
        $warnings.Add('Tailscale Windows service not found or not queryable; remote tests require separate verification.')
    }
}

$tcpCommand = Get-Command Get-NetTCPConnection -ErrorAction SilentlyContinue
if ($null -eq $tcpCommand) {
    $warnings.Add('Get-NetTCPConnection unavailable; skipped local listener checks.')
} else {
    foreach ($port in @($ControllerPort, $AgentPort)) {
        try {
            $listeners = @(Get-NetTCPConnection -State Listen -LocalPort $port -ErrorAction SilentlyContinue)
            if ($listeners.Count -gt 0) {
                $problems.Add('TCP port ' + $port + ' is already listening; select another port for the smoke test.')
            } else {
                Write-Output ('TCP port ' + $port + ': no current listener detected')
            }
        } catch {
            $warnings.Add('Could not inspect TCP port ' + $port + ': ' + $_.Exception.Message)
        }
    }
}

foreach ($warning in $warnings) { Write-Warning $warning }
if ($problems.Count -gt 0) {
    foreach ($problem in $problems) { Write-Error -Message $problem -ErrorAction Continue }
    exit 1
}
Write-Output 'PASS: local Windows prerequisites inspected; no configuration was changed.'
exit 0
