param([switch]$Run)
$ErrorActionPreference = 'Stop'
$taskRoot = [IO.Path]::GetFullPath((Join-Path $PSScriptRoot '..'))
$taskApp = Join-Path $taskRoot 'MolovoxLabs.exe'
$taskLegacyApp = Join-Path $taskRoot 'QuietLab.exe'
$taskBuild = Join-Path $taskRoot '.molovox-build.tmp'
$taskEnvironment = @{}
foreach ($taskName in @('TEMP', 'TMP', 'GOTMPDIR', 'GOCACHE')) {
    $taskEnvironment[$taskName] = [Environment]::GetEnvironmentVariable($taskName, 'Process')
}
Push-Location $taskRoot
try {
    $taskTemp = Join-Path $taskRoot '.build\tmp'
    $taskCache = Join-Path $taskRoot '.build\go-cache'
    New-Item -ItemType Directory -Path $taskTemp, $taskCache -Force | Out-Null
    $env:TEMP = $taskTemp
    $env:TMP = $taskTemp
    $env:GOTMPDIR = $taskTemp
    $env:GOCACHE = $taskCache
    & cmd /c native\vst3\build.cmd
    if ($LASTEXITCODE -ne 0) { throw 'Native VST3 host/icon build failed.' }
    & go test ./...
    if ($LASTEXITCODE -ne 0) { throw 'Tests failed.' }
    $taskTests = @(Get-ChildItem -LiteralPath (Join-Path $taskRoot 'tests') -Filter '*.test.cjs' -File | ForEach-Object FullName)
    & node --test @taskTests
    if ($LASTEXITCODE -ne 0) { throw 'Interface behavior tests failed.' }
    & python scripts/verify-studio.py
    if ($LASTEXITCODE -ne 0) { throw 'Interface validation failed.' }
    & go build -trimpath -ldflags '-H=windowsgui -s -w' -o $taskBuild ./cmd/molovoxlabs
    if ($LASTEXITCODE -ne 0) { throw 'Application build failed.' }
    $taskRunning = @(Get-CimInstance Win32_Process -Filter "name = 'MolovoxLabs.exe' OR name = 'QuietLab.exe'" | Where-Object { ($_.ExecutablePath -eq $taskApp -or $_.ExecutablePath -eq $taskLegacyApp) })
    foreach ($taskProcess in $taskRunning) {
        $taskInstance = Get-Process -Id $taskProcess.ProcessId -ErrorAction SilentlyContinue
        if ($taskInstance) { $taskInstance.Kill(); $taskInstance.WaitForExit() }
    }
    Move-Item -LiteralPath $taskBuild -Destination $taskApp -Force
    if (Test-Path -LiteralPath $taskLegacyApp) { Remove-Item -LiteralPath $taskLegacyApp }
    if ($Run -or $taskRunning.Count -gt 0) { Start-Process -FilePath $taskApp -WindowStyle Hidden }
    Write-Output "Updated $taskApp"
} finally {
    foreach ($taskName in $taskEnvironment.Keys) {
        [Environment]::SetEnvironmentVariable($taskName, $taskEnvironment[$taskName], 'Process')
    }
    Pop-Location
}
