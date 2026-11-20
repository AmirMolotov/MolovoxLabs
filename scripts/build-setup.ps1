param(
    [Parameter(Mandatory)][ValidatePattern('^\d+\.\d+\.\d+$')][string]$Version
)
$ErrorActionPreference = 'Stop'
$root = Split-Path -Parent $PSScriptRoot
$stage = Join-Path $root '.build\msi'
$wix = Join-Path $stage 'wix3'
$release = Join-Path $root 'release'
$msi = Join-Path $stage 'MolovoxLabs.msi'
$setup = Join-Path $release "MolovoxLabs-$Version-x64-Setup.exe"
New-Item -ItemType Directory -Path $stage, $release -Force | Out-Null

if (!(Test-Path (Join-Path $wix 'light.exe'))) {
    $archive = Join-Path $stage 'wix314-binaries.zip'
    Invoke-WebRequest -UseBasicParsing -Uri 'https://github.com/wixtoolset/wix3/releases/download/wix3141rtm/wix314-binaries.zip' -OutFile $archive
    if ((Get-FileHash -LiteralPath $archive).Hash -ne '6AC824E1642D6F7277D0ED7EA09411A508F6116BA6FAE0AA5F2C7DAA2FF43D31') {
        throw 'WiX archive checksum mismatch.'
    }
    Expand-Archive -LiteralPath $archive -DestinationPath $wix -Force
}

& (Join-Path $PSScriptRoot 'build-windows.ps1')

Push-Location $root
try {
    & go build -trimpath -ldflags '-H=windowsgui -s -w' -o "$stage\InstallDependencies.exe" ./scripts/setup-dependencies
    if ($LASTEXITCODE -ne 0) { throw 'Prerequisite launcher build failed.' }
} finally {
    Pop-Location
}

$extensions = @('-ext', (Join-Path $wix 'WixBalExtension.dll'), '-ext', (Join-Path $wix 'WixUtilExtension.dll'))
$candle = Join-Path $wix 'candle.exe'
$light = Join-Path $wix 'light.exe'
& $candle -nologo -arch x64 "-dRoot=$root" "-dVersion=$Version" -out "$stage\installer.wixobj" "$PSScriptRoot\installer.wxs"
if ($LASTEXITCODE -ne 0) { throw 'MSI compilation failed.' }
& $light -nologo -spdb -out $msi "$stage\installer.wixobj"
if ($LASTEXITCODE -ne 0) { throw 'MSI build failed.' }
& $candle -nologo @extensions "-dRoot=$root" "-dStage=$stage" "-dVersion=$Version" -out "$stage\setup.wixobj" "$PSScriptRoot\setup.wxs"
if ($LASTEXITCODE -ne 0) { throw 'Setup compilation failed.' }
& $light -nologo -spdb @extensions -out $setup "$stage\setup.wixobj"
if ($LASTEXITCODE -ne 0) { throw 'Setup build failed.' }
Write-Output "Built $setup"
