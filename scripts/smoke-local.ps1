#Requires -Version 7.0
<#
  Local-only MasterDrive smoke test. No real user directories are exported.
  Builds into a unique temporary directory and removes only its own fixture.
#>
[CmdletBinding()]
param(
    [ValidateRange(1024, 65535)][int]$AgentPort = 18789,
    [ValidateRange(1024, 65535)][int]$ControllerPort = 18788
)

$ErrorActionPreference = 'Stop'
if ($AgentPort -eq $ControllerPort) { throw 'Agent and controller ports must differ.' }
if (-not (Get-Command go -ErrorAction SilentlyContinue)) { throw 'Go is required.' }

$repoRoot = (Resolve-Path -LiteralPath (Join-Path $PSScriptRoot '..')).Path
if (-not (Test-Path -LiteralPath (Join-Path $repoRoot 'web') -PathType Container)) {
    throw 'Missing web/ UI; integrate the frontend before running this smoke test.'
}

$tempParent = [IO.Path]::GetFullPath([IO.Path]::GetTempPath())
$tempPath = Join-Path $tempParent ('masterdrive-smoke-' + [Guid]::NewGuid().ToString('N'))
[IO.Directory]::CreateDirectory($tempPath) | Out-Null
$agentProcess = $null
$controllerProcess = $null

function Start-PrivateProcess {
    param(
        [Parameter(Mandatory)][string]$Executable,
        [Parameter(Mandatory)][string[]]$Arguments,
        [hashtable]$EnvironmentOverrides = @{}
    )
    $start = [Diagnostics.ProcessStartInfo]::new($Executable)
    $start.UseShellExecute = $false
    $start.CreateNoWindow = $true
    $start.RedirectStandardOutput = $true
    $start.RedirectStandardError = $true
    foreach ($argument in $Arguments) { [void]$start.ArgumentList.Add($argument) }
    foreach ($key in $EnvironmentOverrides.Keys) {
        $start.Environment[[string]$key] = [string]$EnvironmentOverrides[$key]
    }
    $process = [Diagnostics.Process]::Start($start)
    if ($null -eq $process) { throw "Failed to start local smoke process." }
    return $process
}

function Wait-ForAgent {
    param([string]$Address, [string]$Token, [Diagnostics.Process]$Process)
    for ($i = 0; $i -lt 60; $i++) {
        if ($Process.HasExited) { throw 'Agent exited during startup.' }
        try {
            $response = Invoke-WebRequest -Uri ("http://$Address/v1/shares") `
                -Headers @{ Authorization = "Bearer $Token" } -NoProxy -TimeoutSec 1
            if ($response.StatusCode -eq 200) { return }
        } catch { Start-Sleep -Milliseconds 100 }
    }
    throw 'Agent did not become available on loopback.'
}

try {
    $exportDir = Join-Path $tempPath 'export'
    $dataDir = Join-Path $tempPath 'state'
    [IO.Directory]::CreateDirectory($exportDir) | Out-Null
    [IO.Directory]::CreateDirectory($dataDir) | Out-Null
    $fixture = 'MasterDrive local mesh smoke fixture' + "`n"
    [IO.File]::WriteAllText((Join-Path $exportDir 'hello.txt'), $fixture,
        [Text.UTF8Encoding]::new($false))

    $bytes = [byte[]]::new(32)
    [Security.Cryptography.RandomNumberGenerator]::Fill($bytes)
    $token = ([BitConverter]::ToString($bytes)).Replace('-', '').ToLowerInvariant()
    $binary = Join-Path $tempPath 'masterdrive.exe'

    Push-Location -LiteralPath $repoRoot
    try {
        & go build -o $binary ./cmd/masterdrive
        if ($LASTEXITCODE -ne 0) { throw 'Go build failed.' }
    } finally { Pop-Location }

    $agentAddress = "127.0.0.1:$AgentPort"
    $controllerAddress = "127.0.0.1:$ControllerPort"
    $agentProcess = Start-PrivateProcess -Executable $binary `
        -Arguments @('agent', '-listen', $agentAddress, '-export', "smoke=$exportDir") `
        -EnvironmentOverrides @{ MASTERDRIVE_AGENT_TOKEN = $token }
    Wait-ForAgent -Address $agentAddress -Token $token -Process $agentProcess

    $registration = @(& $binary add-host -data-dir $dataDir -name smoke-node `
        -url "http://$agentAddress")
    if ($LASTEXITCODE -ne 0 -or $registration.Count -lt 1) {
        throw 'Host registration failed or returned an unexpected ID.'
    }
    $registrationMatch = [regex]::Match([string]$registration[0], '^Added .+: ([0-9a-f]{32})$')
    if (-not $registrationMatch.Success) {
        throw 'Host registration failed or returned an unexpected ID.'
    }
    $hostId = $registrationMatch.Groups[1].Value
    $tokens = @{}
    $tokens[$hostId] = $token
    $tokenJson = ConvertTo-Json -InputObject $tokens -Compress

    $controllerProcess = Start-PrivateProcess -Executable $binary `
        -Arguments @('controller', '-listen', $controllerAddress, '-data-dir', $dataDir,
                     '-web-dir', (Join-Path $repoRoot 'web')) `
        -EnvironmentOverrides @{ MASTERDRIVE_PEER_TOKENS_JSON = $tokenJson }

    # The one-time token is read from an in-memory pipe, never written to disk.
    $lineTask = $controllerProcess.StandardOutput.ReadLineAsync()
    $timeoutTask = [Threading.Tasks.Task]::Delay([TimeSpan]::FromSeconds(10))
    $firstCompleted = [Threading.Tasks.Task]::WhenAny($lineTask, $timeoutTask).GetAwaiter().GetResult()
    if (-not [object]::ReferenceEquals($firstCompleted, $lineTask)) {
        throw 'Controller did not emit a session launch URL within 10 seconds.'
    }
    $launchLine = $lineTask.GetAwaiter().GetResult()
    if ($controllerProcess.HasExited) { throw 'Controller exited during startup.' }
    $sessionMatch = [regex]::Match([string]$launchLine, '#session=([A-Za-z0-9_-]{40,})$')
    if (-not $sessionMatch.Success) {
        throw 'Controller did not provide an ephemeral browser session.'
    }
    $sessionToken = $sessionMatch.Groups[1].Value
    $controllerBase = "http://$controllerAddress"
    $ready = $false
    for ($i = 0; $i -lt 60; $i++) {
        if ($controllerProcess.HasExited) { throw 'Controller exited during startup.' }
        try {
            $response = Invoke-WebRequest -Uri "$controllerBase/api/v1/hosts" `
                -SkipHttpErrorCheck -NoProxy -TimeoutSec 1
            if ($response.StatusCode -eq 401) { $ready = $true; break }
        } catch { Start-Sleep -Milliseconds 100 }
    }
    if (-not $ready) { throw 'Controller did not enforce local session access.' }

    $webSession = [Microsoft.PowerShell.Commands.WebRequestSession]::new()
    $activation = Invoke-WebRequest -Uri "$controllerBase/api/v1/session" `
        -Method Post -Headers @{ 'X-MasterDrive-Session' = $sessionToken } `
        -WebSession $webSession -SkipHttpErrorCheck -NoProxy
    if ($activation.StatusCode -ne 204) { throw 'Local session activation failed.' }

    $hosts = Invoke-RestMethod -Uri "$controllerBase/api/v1/hosts" -WebSession $webSession -NoProxy
    $shares = Invoke-RestMethod -Uri "$controllerBase/api/v1/hosts/$hostId/shares" -WebSession $webSession -NoProxy
    $entries = Invoke-RestMethod -Uri "$controllerBase/api/v1/hosts/$hostId/shares/smoke/entries" -WebSession $webSession -NoProxy
    $downloadPath = Join-Path $tempPath 'downloaded.txt'
    Invoke-WebRequest -Uri "$controllerBase/api/v1/hosts/$hostId/shares/smoke/download?path=hello.txt" `
        -WebSession $webSession -NoProxy -OutFile $downloadPath | Out-Null

    if ($hosts.hosts[0].id -ne $hostId -or $shares.shares[0].id -ne 'smoke' -or
        $entries.entries[0].name -ne 'hello.txt' -or
        [IO.File]::ReadAllText($downloadPath) -ne $fixture) {
        throw 'Read-only mesh smoke assertions failed.'
    }
    Write-Output 'PASS: authenticated local agent, registry, controller, session, listing, and download.'
}
finally {
    foreach ($process in @($controllerProcess, $agentProcess)) {
        if ($null -eq $process) { continue }
        try {
            if (-not $process.HasExited) {
                $process.Kill()
                [void]$process.WaitForExit(5000)
            }
        } finally { $process.Dispose() }
    }
    $safePrefix = Join-Path $tempParent 'masterdrive-smoke-'
    if ([IO.Path]::GetFullPath($tempPath).StartsWith($safePrefix,
            [StringComparison]::OrdinalIgnoreCase)) {
        Remove-Item -LiteralPath $tempPath -Recurse -Force -ErrorAction Stop
    }
}
