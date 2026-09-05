# phpvm

**Languages:** English · [Español](README.es.md)

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
& ([scriptblock]::Create((irm https://raw.githubusercontent.com/Kelevra16/phpvm/main/install.ps1))) -Version v0.6.0
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

Go 1.26 or newer is supported. The module currently prefers the Go 1.26.8 toolchain.

Put `phpvm.exe` on `PATH`, then add `%USERPROFILE%\.phpvm\bin` to `PATH` once. Set `PHPVM_ROOT` to use a different storage root.

## Version management

```text
phpvm use latest                 install and activate the latest PHP
phpvm use 8.4                    newest patch in the 8.4 branch
phpvm use --ts 8.4               thread-safe build
phpvm use --arch x86 8.3         32-bit build
phpvm install 8.4                install without activating
phpvm ls [--json]                installed builds
phpvm ls-remote [--ts] [--json] newest official patch per branch
phpvm ls-remote --all            every archived patch
phpvm current [--json]           active build and metadata
phpvm which [build]              path to the selected php.exe
phpvm uninstall <build>
phpvm prune                      retain only the active build
```

Use `--no-progress` for non-interactive environments or `--quiet` to suppress status output.

Use `phpvm install --offline 8.4` after the release registry and matching verified archive have been cached. Offline mode never falls back to the network.

### Historical and EOL releases

`phpvm` indexes the official Windows archive back to PHP 5.2. A minor selector resolves to its final patch:

```powershell
phpvm ls-remote --arch x86
phpvm use --arch x86 --allow-unverified-archive 5.4
phpvm install --allow-unverified-archive 5.6.40
```

PHP 5.2–5.4 Windows builds are x86-only; old builds may also require their matching Microsoft Visual C++ runtime. The archive does not publish SHA-256 checksums for these ZIPs, so installation requires the explicit opt-in. `phpvm` records the observed archive hash and still hashes `php.exe`, but this does not authenticate the initial download. Supported releases retain normal official checksum verification.

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

Common Composer constraints are resolved against the available official branches, including `^8.3`, `~8.3.2`, `>=8.2 <8.5`, `8.4.*`, and `||` alternatives. Composer stability flags, hyphen ranges, and every edge case of Composer's complete solver are not yet supported.

## Configuration, profiles, and extensions

New installations create an active development `php.ini` instead of leaving PHP unconfigured. phpvm starts from `php.ini-development`, points `extension_dir` at the managed build, sets practical local-development limits, and enables these extensions when the official build includes them:

```text
curl, fileinfo, mbstring, openssl, intl,
mysqli, pdo_mysql, gd, zip, sodium
```

The resulting PHP is executed with `--version`, `--ini`, and `-m` while still in staging. A configuration that cannot start PHP is never published. Existing installations can opt into the same profile—this replaces their current `php.ini`:

```text
phpvm ini defaults --project
phpvm ini defaults --global
phpvm ini defaults --version 8.4
```

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

`ini` and `ext` modify the effective PHP for the current directory by default—the same build reported by `phpvm resolve` and used by `php -v`. Override the target explicitly when needed:

```text
phpvm ext enable mbstring --project
phpvm ext enable mbstring --global
phpvm ext enable mbstring --version 8.5
phpvm ini path --project
phpvm ini set memory_limit 1G --version 8.5
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

## Concurrent projects and isolated shells

Open independent PowerShell sessions for projects that require different PHP versions:

```powershell
# Terminal A
cd C:\projects\legacy-app
phpvm shell 7.4
php --version

# Terminal B
cd C:\projects\modern-app
phpvm shell 8.5
php --version
```

The child terminal sets `PHPVM_ACTIVE` and prepends only its selected PHP directory to `PATH`. Type `exit` to return to the parent terminal. The global version is not changed.

```text
phpvm shell 8.4        install if needed and open an isolated shell
phpvm shell            resolve the current project and open a shell
phpvm shell --current  use the global build in an isolated shell
```

The stable `php.cmd` wrapper also resolves a build on every invocation, allowing different project directories to use different installed versions concurrently. Resolution priority is:

```text
PHPVM_ACTIVE → project configuration → global current build
```

Project configuration means `.php-version`, `phpvm.toml`, or the supported Composer constraint. The shim never downloads PHP implicitly; when a project build is missing it asks you to run `phpvm sync`.

Inspect the selected build or executable without running PHP:

```text
phpvm resolve
phpvm resolve --path
phpvm resolve 8.4
```

Named aliases are stored independently from installed builds:

```text
phpvm alias set legacy 7.4
phpvm alias set stable 8.4
phpvm use stable
phpvm alias ls
phpvm alias remove legacy
```

## Daily maintenance

Locate the active executable, inspect the registry cache, or update phpvm itself:

```text
phpvm which
phpvm which 8.3
phpvm cache dir
phpvm cache clear
phpvm self-update
phpvm self-update v0.6.0
```

`self-update` downloads the selected GitHub Release, verifies its published SHA-256 checksum, stages the new executable, and replaces the running binary after the command exits.

## PowerShell completion

Generate the native argument completer:

```powershell
New-Item -ItemType Directory -Force (Split-Path $PROFILE) | Out-Null
phpvm completion powershell | Add-Content $PROFILE
. $PROFILE
```

Completion covers commands, subcommands, and installed builds for common version operations.

## Laragon

`phpvm` detects Laragon from `LARAGON_ROOT`, `C:\laragon`, or `C:\tools\laragon`:

```text
phpvm laragon detect
phpvm laragon link
phpvm laragon link 8.3
phpvm laragon unlink 8.3
```

`link` creates a directory junction under Laragon's `bin\php` directory. Select the resulting `phpvm-<build>` entry from Laragon's PHP version menu and reload Laragon. Removing the junction does not remove the managed PHP build.

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
3. Its official SHA-256 checksum is verified (except explicitly opted-in EOL archives, whose observed hash is recorded).
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

## Troubleshooting installation

If `phpvm version` shows `dev` after installing a release, inspect every matching executable:

```powershell
Get-Command phpvm -All
where.exe phpvm
```

The canonical release location is `%LOCALAPPDATA%\phpvm\bin\phpvm.exe`. Current installers move an older `~\.phpvm\bin\phpvm.exe` to `phpvm.exe.legacy` and put the canonical directory first in `PATH`. Open a new terminal after installation so PowerShell reads the updated user environment.

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

## Advanced environment tools

Interactive terminals use status colors, Unicode symbols, tables, and an in-place download bar. Redirected output and CI automatically use stable plain text. Decoration can also be controlled explicitly:

```powershell
phpvm --plain ls-remote
phpvm --no-color doctor
phpvm --verbose install 8.4
$env:NO_COLOR = "1"
```

JSON output is never decorated, and arguments following `--` are passed to child commands unchanged.

```text
phpvm info 5.6 --json       availability, EOL status, architecture, and compiler runtime
phpvm supported             branches still receiving PHP security support
phpvm use latest-8.3        newest patch in one branch
phpvm use supported         newest supported release
phpvm use legacy            newest EOL release
phpvm lock                  capture PHP, INI, and enabled extensions in phpvm.lock
phpvm restore               reproduce phpvm.lock
```

EOL selections display a warning. `doctor` executes PHP and reports its expected Windows compiler runtime (VC6 through VS17), helping diagnose missing runtime DLLs. The historical checksum manifest is maintained in the repository; entries verified by phpvm no longer need the unverified-archive opt-in, while packages without a manifest entry still do.

External extension and Composer workflows:

```text
phpvm ext search redis
phpvm ext install https://example.test/php_redis-compatible-build.zip
phpvm ext update
phpvm composer setup
phpvm trust project
phpvm composer -- install
phpvm composer -- --version
phpvm composer 7.4 -- install
```

External extension sources are recorded and refreshed from the same URL. Packages must already match PHP version, TS/NTS, architecture, and compiler runtime; phpvm does not guess binary compatibility or PECL dependencies. Composer is downloaded from its official stable channel and verified with its published SHA-256. Composer arguments always follow `--`; without an explicit version, phpvm uses the project-selected or globally active PHP build. A successful Composer run automatically records its expected `composer.lock` change, while changes to other trusted project configuration still require review.

Existing PHP directories can be copied into managed storage without changing their source:

```text
phpvm import C:\laragon\bin\php\php-8.3.0
phpvm import C:\tools\custom-php --name 8.3.0-custom --ts --arch x64
```

## Version 0.5 workflow tools

Inspect the effective project environment and validate Composer platform requirements:

```text
phpvm status
phpvm status --json
phpvm check
phpvm doctor --fix
```

Apply practical INI presets with `phpvm ini preset development|production|testing|codeigniter|laravel|wordpress`. The selected build still receives only extensions included in its official PHP distribution.

Commands that execute project-controlled configuration require an explicit, content-sensitive trust decision. Changing `.php-version`, `phpvm.toml`, `phpvm.lock`, `composer.json`, or `composer.lock` invalidates it:

```text
phpvm init --version 8.4 --preset development
phpvm trust project
phpvm trust status
phpvm serve --port 8080
phpvm serve --port 8080 --public public
phpvm trust revoke
```

`trust project` requires an initialized project because it fingerprints the project configuration. The normal first-run order is `phpvm init`, `phpvm trust project`, and then `phpvm serve` or another command that executes project-controlled configuration.

`serve` uses the project root (`.`) as its document root by default. Frameworks that expose a dedicated web directory can pass `--public public`. The built-in server enables colored HTTP status and error output in an interactive terminal; global `--plain` disables those colors too.

Set `PHPVM_SAFE_MODE=1` to block command execution and external package workflows. PIE support uses the official PHAR and requires GitHub CLI to verify its artifact attestation before installation:

```text
phpvm pie setup
phpvm pie install vendor/extension
phpvm pie path
```

Downloaded PHP archives are retained under the content-addressed cache and checked again before reuse. Cache bundles include a checksum manifest and reject unsafe paths, making it possible to transfer previously cached official metadata and verified archives to another machine:

```text
phpvm cache list
phpvm cache verify
phpvm bundle create phpvm-offline.zip
phpvm bundle import phpvm-offline.zip
phpvm install --offline 8.4
```

Treat imported bundles as files from their sender: the manifest detects corruption but does not establish who created the bundle.

## Friendly interface

Open the interactive control center when you prefer a guided workflow:

```powershell
$env:PHPVM_LANG = "es" # optional: Spanish interactive messages
phpvm ui
```

The control center provides a live workspace summary, descriptive action cards, and keyboard-navigable selectors. Use Up/Down and Enter, type to filter, or press Escape to return. Every nested selector includes a visible Back option; numbers remain available as a fallback. The screen refreshes between actions and pauses after results so output remains readable. It is enabled only in a real terminal; redirected output, JSON commands, CI, and `--plain` retain deterministic behavior.

Decorated terminals use a branded banner, color-coded sections, emoji navigation, searchable choices, and subtle animated loading indicators. `phpvm help` keeps one command per entry and groups commands by workflow so it remains easy to scan. Animation and decoration are automatically disabled for redirected output and automation.

The same features are available as focused commands:

```text
phpvm dashboard
phpvm use                         interactive installed-version selector
phpvm install                     interactive remote-version selector
phpvm init --version 8.4 --preset codeigniter
phpvm doctor --interactive
phpvm open logs
phpvm open ini
phpvm open root
```

Interactive terminals request confirmation before `uninstall`, `prune`, and `repair`. Automation can pass `--yes`; non-interactive scripts continue without prompts. Installation output now identifies download, checksum, extraction, configuration, runtime validation, and publication stages. Common failures include a concise command-oriented hint.

## Remaining roadmap

- Linux and macOS providers
- complete Composer constraint resolution
- external PECL extension installation and dependency resolution
- prerelease selectors and cached remote metadata
- PowerShell, Bash, and Zsh completions
- self-update and signed release artifacts
- richer Laragon automation and automatic reload
- opt-in lifecycle hooks
