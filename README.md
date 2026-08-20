# phpvm

[![CI](https://github.com/Kelevra16/phpvm/actions/workflows/ci.yml/badge.svg)](https://github.com/Kelevra16/phpvm/actions/workflows/ci.yml)
[![Release](https://github.com/Kelevra16/phpvm/actions/workflows/release.yml/badge.svg)](https://github.com/Kelevra16/phpvm/releases)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE)

`phpvm` is a PHP version and environment manager inspired by [gobrew](https://github.com/kevincobain2000/gobrew). It installs official PHP builds without administrator privileges and exposes a stable wrapper that does not require a shell rehash when versions change.

> **Release candidate:** the binary manager currently targets official Windows x64/x86 builds. Linux and macOS are not supported yet.

## Install

After the first GitHub release is published, install or update the latest version from PowerShell:

```powershell
irm https://raw.githubusercontent.com/Kelevra16/phpvm/main/install.ps1 | iex
```

Install a specific release:

```powershell
& ([scriptblock]::Create((irm https://raw.githubusercontent.com/Kelevra16/phpvm/main/install.ps1))) -Version v0.1.0
```

The installer:

- selects the correct Windows x64/x86 artifact;
- verifies it against the release SHA-256 manifest;
- installs `phpvm.exe` under `%LOCALAPPDATA%\phpvm\bin`;
- adds both `phpvm` and its managed PHP wrapper directory to the user `PATH`.

Open a new terminal if necessary, then run:

```powershell
phpvm use 8.4
php --version
phpvm doctor
```

To choose another destination or leave `PATH` unchanged, download the installer and run:

```powershell
.\install.ps1 -InstallDir C:\Tools\phpvm -NoPathUpdate
```

If no release exists yet, build from source as described below.

## Build from source

```powershell
go test ./...
go build -o phpvm.exe ./cmd/phpvm
```

Go 1.20 or newer is supported.

Put `phpvm.exe` on `PATH`, then add `%USERPROFILE%\.phpvm\bin` to `PATH` once. Set `PHPVM_ROOT` to use a different storage root.

## Version management

```text
phpvm use latest                 install and activate the latest PHP
phpvm use 8.4                    newest patch in the 8.4 branch
phpvm use --ts 8.4               thread-safe build
phpvm use --arch x86 8.3         32-bit build
phpvm install 8.4                install without activating
phpvm ls [--json]                installed builds
phpvm ls-remote [--ts] [--json] available official builds
phpvm current [--json]           active build and metadata
phpvm uninstall <build>
phpvm prune                      retain only the active build
```

Use `--no-progress` for non-interactive environments or `--quiet` to suppress status output.

A build identity includes all compatibility dimensions, for example `8.4.24-nts-x64`. This allows TS/NTS or x64/x86 builds of the same version to coexist.

## Project environments

Running `phpvm` without arguments searches the current directory and its parents. Resolution order is:

1. `.php-version`
2. `phpvm.toml`
3. `composer.lock` platform PHP
4. `composer.json` platform or PHP requirement

`phpvm.toml` can define PHP and its INI settings:

```toml
version = "8.4"
variant = "nts"
arch = "x64"

[ini]
memory_limit = "1G"
display_errors = "On"
```

Apply the complete project environment with:

```text
phpvm sync
```

Projects can keep their PHP errors locally:

```toml
[logs]
scope = "project"
path = ".phpvm/php-error.log"
```

Composer constraints are currently reduced to their first explicit major/minor selector. Complex constraint solving is not yet supported.

## Configuration, profiles, and extensions

```text
phpvm ini get memory_limit
phpvm ini set memory_limit 1G
phpvm ini path
phpvm ini show
phpvm ini diff
phpvm ini reset

phpvm profile create laravel
phpvm profile set laravel memory_limit 1G
phpvm profile set laravel display_errors On
phpvm profile use laravel

phpvm ext ls
phpvm ext enable curl
phpvm ext disable curl
```

Extension management currently enables or disables DLLs already included in an official PHP distribution. Downloading and resolving external PECL packages is a separate future provider.

## Error logs

PHP error logging is configured automatically for the active build. The default location is `~/.phpvm/logs/<build>/php-error.log`; `phpvm.toml` can override it per project.

```text
phpvm logs path
phpvm logs show
phpvm logs show --lines 200
phpvm logs tail --lines 50
phpvm logs tail --follow
phpvm logs open
phpvm logs doctor
phpvm logs clear --force
```

`logs open` uses the operating system's default application. `tail --follow` continues until interrupted with Ctrl+C. Clearing a log requires `--force` so scripts cannot erase it accidentally.

## Reproducible commands and matrices

Run a command with a selected version without changing the globally active build:

```text
phpvm exec 8.3 -- php --version
phpvm exec -- composer test
```

Run the same command against several PHP branches:

```text
phpvm matrix 8.2 8.3 8.4 -- php vendor/bin/phpunit
```

Named aliases are stored independently from installed builds:

```text
phpvm alias set legacy 7.4
phpvm alias set stable 8.4
phpvm use stable
phpvm alias ls
phpvm alias remove legacy
```

## Integrity and diagnostics

```text
phpvm doctor [--json]
phpvm verify [build]
phpvm repair [build]
phpvm clean
```

Installations are transactional:

1. An inter-process lock serializes modifications.
2. The archive is downloaded to a temporary file.
3. Its official SHA-256 checksum is verified.
4. It is extracted into a staging directory with ZIP traversal protection.
5. `php.exe` is validated and hashed.
6. The completed directory is atomically published.

Each build contains `phpvm.json` with its version, variant, architecture, source URL, checksums, and installation timestamp. `verify` detects later changes to `php.exe`; `repair` replaces a build from its recorded official source while preserving the active identity.

The official release registry is cached for six hours. Set `PHPVM_CACHE_TTL` to a Go duration such as `30m` or `24h`. Set `PHPVM_TIMEOUT` to bound a command, for example `PHPVM_TIMEOUT=10m`.

Exit codes are stable: `0` success, `1` operational failure, `2` invalid usage, and `124` timeout. Child commands executed through `phpvm exec` retain their own non-zero exit code.

## Uninstall

Run the repository's uninstall script to remove the executable while retaining installed PHP versions:

```powershell
.\uninstall.ps1
```

Pass `-RemoveData` only when all managed PHP versions, profiles, logs, and configuration should also be removed.

## Storage layout

```text
~/.phpvm/
  aliases.json
  profiles.json
  current
  bin/php.cmd
  versions/
    8.4.24-nts-x64/
      php.exe
      php.ini
      phpvm.json
```

## Remaining roadmap

- Linux and macOS providers
- complete Composer constraint resolution
- external PECL extension installation and dependency resolution
- prerelease selectors and cached remote metadata
- PowerShell, Bash, and Zsh completions
- self-update and signed release artifacts
- opt-in lifecycle hooks
