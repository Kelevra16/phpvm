# phpvm

`phpvm` is a PHP version and environment manager inspired by [gobrew](https://github.com/kevincobain2000/gobrew). It installs official PHP builds without administrator privileges and exposes a stable wrapper that does not require a shell rehash when versions change.

> Platform status: the binary manager currently targets official Windows x64/x86 builds. The provider boundary is intentionally isolated for future Linux and macOS support.

## Build and setup

```powershell
go test ./...
go build -o phpvm.exe ./cmd/phpvm
```

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

Composer constraints are currently reduced to their first explicit major/minor selector. Complex constraint solving is not yet supported.

## Configuration, profiles, and extensions

```text
phpvm ini get memory_limit
phpvm ini set memory_limit 1G

phpvm profile create laravel
phpvm profile set laravel memory_limit 1G
phpvm profile set laravel display_errors On
phpvm profile use laravel

phpvm ext ls
phpvm ext enable curl
phpvm ext disable curl
```

Extension management currently enables or disables DLLs already included in an official PHP distribution. Downloading and resolving external PECL packages is a separate future provider.

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
- self-update, signed release artifacts, and CI release automation
- opt-in lifecycle hooks

