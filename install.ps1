[CmdletBinding()]
param(
    [string]$Repository = "Kelevra16/phpvm",
    [string]$Version = "latest",
    [string]$InstallDir = "$env:LOCALAPPDATA\phpvm\bin",
    [switch]$NoPathUpdate
)

$ErrorActionPreference = "Stop"
$ProgressPreference = "SilentlyContinue"
[Net.ServicePointManager]::SecurityProtocol = [Net.SecurityProtocolType]::Tls12

function Get-PhpvmRelease {
    param([string]$Repository, [string]$Version)

    $headers = @{ "User-Agent" = "phpvm-installer"; "Accept" = "application/vnd.github+json" }
    $endpoint = if ($Version -eq "latest") {
        "https://api.github.com/repos/$Repository/releases/latest"
    } else {
        $tag = if ($Version.StartsWith("v")) { $Version } else { "v$Version" }
        "https://api.github.com/repos/$Repository/releases/tags/$tag"
    }

    try {
        return Invoke-RestMethod -Uri $endpoint -Headers $headers
    } catch {
        throw "Could not find release '$Version' in https://github.com/$Repository/releases. Publish a release first or build phpvm from source. $($_.Exception.Message)"
    }
}

function Set-PhpvmPathOrder {
    param([string[]]$PreferredEntries)

    $preferred = @($PreferredEntries | ForEach-Object { [IO.Path]::GetFullPath($_).TrimEnd("\") })
    $userEntries = @([Environment]::GetEnvironmentVariable("Path", "User") -split ";" | Where-Object { $_ })
    $processEntries = @($env:Path -split ";" | Where-Object { $_ })

    foreach ($entry in $preferred) {
        $userEntries = @($userEntries | Where-Object {
            try { [IO.Path]::GetFullPath($_).TrimEnd("\") -ine $entry } catch { $_ -ine $entry }
        })
        $processEntries = @($processEntries | Where-Object {
            try { [IO.Path]::GetFullPath($_).TrimEnd("\") -ine $entry } catch { $_ -ine $entry }
        })
    }
    [Environment]::SetEnvironmentVariable("Path", (($preferred + $userEntries) -join ";"), "User")
    $env:Path = (($preferred + $processEntries) -join ";")
}

$architecture = if ([Environment]::Is64BitOperatingSystem) { "amd64" } else { "386" }
$release = Get-PhpvmRelease -Repository $Repository -Version $Version
$tag = [string]$release.tag_name
$assetPattern = "^phpvm_.+_windows_${architecture}\.zip$"
$asset = @($release.assets | Where-Object { $_.name -match $assetPattern })
$checksums = @($release.assets | Where-Object { $_.name -eq "checksums.txt" })
if ($asset.Count -ne 1) { throw "Expected one Windows $architecture archive in release $tag; found $($asset.Count)." }
if ($checksums.Count -ne 1) { throw "Release $tag does not contain checksums.txt." }
$asset = $asset[0]
$checksums = $checksums[0]

$temporary = Join-Path ([IO.Path]::GetTempPath()) ("phpvm-install-" + [guid]::NewGuid())
New-Item -ItemType Directory -Path $temporary | Out-Null
try {
    $archive = Join-Path $temporary $asset.name
    $checksumFile = Join-Path $temporary "checksums.txt"
    Invoke-WebRequest -Uri $asset.browser_download_url -OutFile $archive -Headers @{ "User-Agent" = "phpvm-installer" }
    Invoke-WebRequest -Uri $checksums.browser_download_url -OutFile $checksumFile -Headers @{ "User-Agent" = "phpvm-installer" }

    $checksumLine = Get-Content $checksumFile | Where-Object { $_ -match "\s+$([regex]::Escape($asset.name))$" } | Select-Object -First 1
    if (-not $checksumLine) { throw "No checksum was published for $($asset.name)." }
    $expected = ($checksumLine -split "\s+")[0]
    $actual = (Get-FileHash $archive -Algorithm SHA256).Hash
    if ($actual -ine $expected) { throw "SHA-256 verification failed for $($asset.name)." }

    $extracted = Join-Path $temporary "extracted"
    Expand-Archive $archive -DestinationPath $extracted -Force
    $candidate = Join-Path $extracted "phpvm.exe"
    if (-not (Test-Path -LiteralPath $candidate -PathType Leaf)) { throw "The release archive does not contain phpvm.exe." }

    New-Item -ItemType Directory -Force -Path $InstallDir | Out-Null
    $destination = Join-Path $InstallDir "phpvm.exe"
    $staged = Join-Path $InstallDir "phpvm.exe.new"
    Copy-Item -LiteralPath $candidate -Destination $staged -Force
    Move-Item -LiteralPath $staged -Destination $destination -Force
} finally {
    Remove-Item -LiteralPath $temporary -Recurse -Force -ErrorAction SilentlyContinue
}

$phpvmRoot = if ($env:PHPVM_ROOT) { $env:PHPVM_ROOT } else { Join-Path $HOME ".phpvm" }
$legacyExecutable = Join-Path $phpvmRoot "bin\phpvm.exe"
$destination = [IO.Path]::GetFullPath((Join-Path $InstallDir "phpvm.exe"))
if ((Test-Path -LiteralPath $legacyExecutable -PathType Leaf) -and ([IO.Path]::GetFullPath($legacyExecutable) -ine $destination)) {
    $legacyBackup = "$legacyExecutable.legacy"
    Move-Item -LiteralPath $legacyExecutable -Destination $legacyBackup -Force
    Write-Warning "Moved legacy executable to '$legacyBackup' so it cannot shadow the installed release."
}
if (-not $NoPathUpdate) {
    Set-PhpvmPathOrder -PreferredEntries @($InstallDir, (Join-Path $phpvmRoot "bin"))
}

Write-Host "Installed phpvm $tag to $InstallDir" -ForegroundColor Green
if ($NoPathUpdate) {
    Write-Host "PATH was not changed. Add '$InstallDir' and '$(Join-Path $phpvmRoot "bin")' manually."
} else {
    Write-Host "PATH was updated for this session and future terminals."
}
Write-Host "Next: phpvm use 8.4"
