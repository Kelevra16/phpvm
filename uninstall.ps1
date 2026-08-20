[CmdletBinding(SupportsShouldProcess)]
param(
    [string]$InstallDir = "$env:LOCALAPPDATA\phpvm\bin",
    [switch]$RemoveData
)

$ErrorActionPreference = "Stop"
if ($PSCmdlet.ShouldProcess($InstallDir, "Remove phpvm executable")) {
    Remove-Item -LiteralPath (Join-Path $InstallDir "phpvm.exe") -Force -ErrorAction SilentlyContinue
}
$userPath = [Environment]::GetEnvironmentVariable("Path", "User")
$entries = @($userPath -split ";" | Where-Object { $_ -and $_ -ne $InstallDir })
[Environment]::SetEnvironmentVariable("Path", ($entries -join ";"), "User")
if ($RemoveData) {
    $data = if ($env:PHPVM_ROOT) { $env:PHPVM_ROOT } else { Join-Path $HOME ".phpvm" }
    if ($PSCmdlet.ShouldProcess($data, "Remove all installed PHP versions and configuration")) {
        Remove-Item -LiteralPath $data -Recurse -Force -ErrorAction SilentlyContinue
    }
}
Write-Host "phpvm was uninstalled. Existing PHP versions were retained unless -RemoveData was supplied."
