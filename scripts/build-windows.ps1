param([switch]$Run)
$ErrorActionPreference = 'Stop'
$taskRoot = [IO.Path]::GetFullPath((Join-Path $PSScriptRoot '..'))
$taskApp = Join-Path $taskRoot 'QuietLab.exe'
$taskBuild = Join-Path $taskRoot '.quietlab-build.tmp'
Push-Location $taskRoot
try {
    & cmd /c native\vst3\build.cmd
    if ($LASTEXITCODE -ne 0) { throw 'Native VST3 host/icon build failed.' }
    & go test ./...
    if ($LASTEXITCODE -ne 0) { throw 'Tests failed.' }
    $taskTests = @(Get-ChildItem -LiteralPath (Join-Path $taskRoot 'tests') -Filter '*.test.cjs' -File | ForEach-Object FullName)
    & node --test @taskTests
    if ($LASTEXITCODE -ne 0) { throw 'Interface behavior tests failed.' }
    & python scripts/verify-studio.py
    if ($LASTEXITCODE -ne 0) { throw 'Interface validation failed.' }
    & go build -trimpath -ldflags '-H=windowsgui -s -w' -o $taskBuild .
    if ($LASTEXITCODE -ne 0) { throw 'Application build failed.' }
    $taskRunning = @(Get-CimInstance Win32_Process -Filter "name = 'QuietLab.exe'" | Where-Object { $_.ExecutablePath -eq $taskApp })
    foreach ($taskProcess in $taskRunning) {
        $taskInstance = Get-Process -Id $taskProcess.ProcessId -ErrorAction SilentlyContinue
        if ($taskInstance) { $taskInstance.Kill(); $taskInstance.WaitForExit() }
    }
    Move-Item -LiteralPath $taskBuild -Destination $taskApp -Force
    if ($Run -or $taskRunning.Count -gt 0) { Start-Process -FilePath $taskApp }
    Write-Output "Updated $taskApp"
} finally { Pop-Location }
