$ErrorActionPreference = 'Stop'
$ProgressPreference = 'SilentlyContinue'
[Net.ServicePointManager]::SecurityProtocol = [Net.SecurityProtocolType]::Tls12

function Get-VerifiedArchive($Uri, $Destination, $Sha256) {
    Invoke-WebRequest -UseBasicParsing -Uri $Uri -OutFile $Destination -TimeoutSec 240
    $hash = [Security.Cryptography.SHA256]::Create()
    $stream = [IO.File]::OpenRead($Destination)
    try {
        $actual = [BitConverter]::ToString($hash.ComputeHash($stream)).Replace('-', '')
    } finally {
        $stream.Dispose()
        $hash.Dispose()
    }
    if ($actual -ne $Sha256) {
        throw "Download checksum mismatch: $Uri"
    }
}

function Get-VirtualCable {
    Get-CimInstance -ClassName Win32_PnPEntity -Filter "PNPClass = 'MEDIA' AND Present = TRUE" |
        Where-Object { $_.HardwareID -contains 'VBAudioVACWDM' }
}

function Test-WebViewRuntime {
    $runtimeKey = 'HKLM:\SOFTWARE\WOW6432Node\Microsoft\EdgeUpdate\Clients\{F3017226-FE2A-4295-8BDF-00C3A9A7E4C5}'
    $version = (Get-ItemProperty -LiteralPath $runtimeKey -Name pv -ErrorAction SilentlyContinue).pv
    return $version -and $version -ne '0.0.0.0'
}

function Test-MediaTools($Folder) {
    foreach ($tool in @('ffmpeg.exe', 'ffprobe.exe')) {
        if (!(Test-Path -LiteralPath (Join-Path $Folder $tool) -PathType Leaf)) { return $false }
    }
    return $true
}

function Invoke-PrerequisiteSetup($File, $Arguments) {
    $process = Start-Process -FilePath $File -ArgumentList $Arguments -WorkingDirectory (Split-Path -Parent $File) -PassThru -WindowStyle Hidden
    if (!$process.WaitForExit(300000)) {
        $process.Kill()
        throw "Prerequisite installer timed out: $(Split-Path -Leaf $File)."
    }
    return $process.ExitCode
}

$windowsBuild = (Get-ItemProperty 'HKLM:\SOFTWARE\Microsoft\Windows NT\CurrentVersion' -Name CurrentBuildNumber).CurrentBuildNumber
if ([int]$windowsBuild -lt 17763) { throw 'Molovox Labs requires Windows 10 (1809) or later.' }
$runtimePath = Join-Path ([Environment]::GetFolderPath('CommonProgramFiles')) 'Molovox Labs'
$toolsPath = Join-Path $runtimePath 'Tools'
$stage = Join-Path $runtimePath ([Guid]::NewGuid().ToString('N'))
$logKey = 'Registry::HKEY_LOCAL_MACHINE\SOFTWARE\MolovoxLabs'
$restartRequired = $false
try {
    if ($env:MOLOVOX_PREREQUISITE -eq 'ffmpeg' -and !(Test-MediaTools $toolsPath)) {
        # Older MSIs remove their own copy during the upgrade.
        $media = Join-Path $runtimePath 'Media'
        $legacyTools = Test-MediaTools $media
        $existing = $null
        if (!$legacyTools) {
            $machinePath = [Environment]::GetEnvironmentVariable('Path', 'Machine')
            $existing = $machinePath -split ';' | ForEach-Object {
                $folder = [Environment]::ExpandEnvironmentVariables($_.Trim('"'))
                if ($folder -match '^[A-Za-z]:\\' -and (Test-MediaTools $folder)) { $folder }
            } | Select-Object -First 1
        }
        if (!$existing) {
            New-Item -ItemType Directory -Path $stage -Force | Out-Null
            if ($legacyTools) {
                Copy-Item -LiteralPath (Join-Path $media 'ffmpeg.exe'), (Join-Path $media 'ffprobe.exe') -Destination $stage
                $media = $stage
            } else {
                $archive = Join-Path $stage 'ffmpeg.7z'
                Get-VerifiedArchive 'https://github.com/GyanD/codexffmpeg/releases/download/9.0.2/ffmpeg-9.0.2-essentials_build.7z' $archive '4705843ccaaf54257c16ad90f3e952ece33c17df964ecf7bfdbb0f49c7171077'
                & (Join-Path ([Environment]::SystemDirectory) 'tar.exe') -xf $archive -C $stage
                if ($LASTEXITCODE -ne 0) { throw 'Could not extract the FFmpeg package.' }
                $media = Join-Path $stage 'ffmpeg-9.0.2-essentials_build\bin'
                if (!(Test-MediaTools $media)) { throw 'FFmpeg package is missing ffmpeg or ffprobe.' }
            }
            New-Item -ItemType Directory -Path $toolsPath -Force | Out-Null
            foreach ($tool in @('ffmpeg.exe', 'ffprobe.exe')) {
                Move-Item -LiteralPath (Join-Path $media $tool) -Destination (Join-Path $toolsPath $tool) -Force
            }
        }
    }

    if ($env:MOLOVOX_PREREQUISITE -eq 'webview2' -and !(Test-WebViewRuntime)) {
        New-Item -ItemType Directory -Path $stage -Force | Out-Null
        $setup = Join-Path $stage 'MicrosoftEdgeWebview2Setup.exe'
        Invoke-WebRequest -UseBasicParsing -Uri 'https://go.microsoft.com/fwlink/p/?LinkId=2124703' -OutFile $setup -TimeoutSec 120
        $signature = Get-AuthenticodeSignature -LiteralPath $setup
        if ($signature.Status -ne 'Valid' -or $signature.SignerCertificate.GetNameInfo('SimpleName', $false) -ne 'Microsoft Corporation') {
            throw 'WebView2 bootstrapper is missing a valid Microsoft signature.'
        }
        $exitCode = Invoke-PrerequisiteSetup $setup @('/silent', '/install')
        if (!(Test-WebViewRuntime)) { throw "WebView2 setup did not install the runtime (exit $exitCode)." }
        if ($exitCode -eq 3010) { $restartRequired = $true }
    }

    if ($env:MOLOVOX_PREREQUISITE -eq 'vbcable' -and !(Get-VirtualCable)) {
        New-Item -ItemType Directory -Path $stage -Force | Out-Null
        $archive = Join-Path $stage 'vbcable.zip'
        $package = Join-Path $stage 'vbcable'
        Get-VerifiedArchive 'https://download.vb-audio.com/Download_CABLE/VBCABLE_Driver_Pack46.zip' $archive 'BC44E8974191A5A2427D6C979E2DAB08DBB09925F3F435DFA2F0BA3E45B6EB90'
        Expand-Archive -LiteralPath $archive -DestinationPath $package
        $setup = Join-Path $package 'VBCABLE_Setup_x64.exe'
        $exitCode = Invoke-PrerequisiteSetup $setup @('-i', '-h')
        if (!(Get-VirtualCable)) { throw "VB-CABLE setup did not register the virtual device (exit $exitCode)." }
        $restartRequired = $true
    }

    Remove-ItemProperty -LiteralPath $logKey -Name 'SetupError' -ErrorAction SilentlyContinue
} catch {
    New-Item -Path $logKey -Force | Out-Null
    Set-ItemProperty -LiteralPath $logKey -Name 'SetupError' -Value "$($env:MOLOVOX_PREREQUISITE) - $($_.Exception.Message)"
    throw
} finally {
    if (Test-Path -LiteralPath $stage -PathType Container) {
        # The stage is a GUID directory under the protected Common Files folder.
        $resolvedStage = (Resolve-Path -LiteralPath $stage).ProviderPath
        if ($resolvedStage -eq $stage) { Remove-Item -LiteralPath $resolvedStage -Recurse -Force }
    }
}
if ($restartRequired) { exit 3010 }
exit 0
