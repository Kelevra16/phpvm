[CmdletBinding()]
param(
    [string]$Repository = "Kelevra16/phpvm",
    [string]$Version = "latest",
    [string]$InstallDir = "$env:LOCALAPPDATA\phpvm\bin"
)

$ErrorActionPreference = "Stop"
$architecture = if ([Environment]::Is64BitOperatingSystem) { "amd64" } else { "386" }
$release = if ($Version -eq "latest") {
    Invoke-RestMethod "https://api.github.com/repos/$Repository/releases/latest"
} else {
    Invoke-RestMethod "https://api.github.com/repos/$Repository/releases/tags/$Version"
}
$tag = $release.tag_name
$plainVersion = $tag.TrimStart("v")
$assetName = "phpvm_${plainVersion}_windows_${architecture}.zip"
$asset = $release.assets | Where-Object name -eq $assetName | Select-Object -First 1
$checksums = $release.assets | Where-Object name -eq "checksums.txt" | Select-Object -First 1
if (-not $asset -or -not $checksums) { throw "Release assets for $assetName are incomplete" }

$temporary = Join-Path ([IO.Path]::GetTempPath()) ("phpvm-install-" + [guid]::NewGuid())
New-Item -ItemType Directory -Path $temporary | Out-Null
try {
    $archive = Join-Path $temporary $assetName
    $checksumFile = Join-Path $temporary "checksums.txt"
    Invoke-WebRequest $asset.browser_download_url -OutFile $archive
    Invoke-WebRequest $checksums.browser_download_url -OutFile $checksumFile
    $expected = (Get-Content $checksumFile | Where-Object { $_ -match [regex]::Escape($assetName) } | Select-Object -First 1).Split()[0]
    $actual = (Get-FileHash $archive -Algorithm SHA256).Hash
    if (-not $expected -or $actual -ne $expected) { throw "SHA-256 verification failed" }
    New-Item -ItemType Directory -Force -Path $InstallDir | Out-Null
    $extracted = Join-Path $temporary "extracted"
    Expand-Archive $archive -DestinationPath $extracted -Force
    Copy-Item (Join-Path $extracted "phpvm.exe") (Join-Path $InstallDir "phpvm.exe") -Force
} finally {
    Remove-Item -LiteralPath $temporary -Recurse -Force -ErrorAction SilentlyContinue
}

$userPath = [Environment]::GetEnvironmentVariable("Path", "User")
$entries = @($userPath -split ";" | Where-Object { $_ })
$phpvmRoot = if ($env:PHPVM_ROOT) { $env:PHPVM_ROOT } else { Join-Path $HOME ".phpvm" }
foreach ($entry in @($InstallDir, (Join-Path $phpvmRoot "bin"))) {
    if ($entries -notcontains $entry) { $entries += $entry }
}
[Environment]::SetEnvironmentVariable("Path", ($entries -join ";"), "User")
Write-Host "Installed phpvm $tag to $InstallDir"
Write-Host "Open a new terminal, then run: phpvm use 8.4"
