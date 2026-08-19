# phpvm

`phpvm` is a small PHP version manager inspired by [gobrew](https://github.com/kevincobain2000/gobrew). It installs PHP without administrator privileges and switches versions without requiring a shell rehash.

> Current MVP: Windows x64/x86 using the official non-thread-safe PHP builds. The storage and provider layers are separated so Linux and macOS providers can be added next.

## Build

```powershell
go build -o phpvm.exe ./cmd/phpvm
```

Put `phpvm.exe` somewhere on `PATH`, then add `%USERPROFILE%\.phpvm\bin` to `PATH` once. The stable wrapper in that directory dispatches to the active PHP version.

## Usage

```text
phpvm use latest       # install and activate the latest stable PHP
phpvm use 8.4          # resolve and activate the newest PHP 8.4 patch
phpvm install 8.3      # install only
phpvm ls               # installed versions; * is active
phpvm ls-remote        # official versions available for this platform
phpvm current
phpvm uninstall 8.3.20
phpvm prune
```

Create a `.php-version` file containing `8.4`, `8.4.10`, or `latest`. Running `phpvm` with no arguments searches the current directory and its parents, then installs and activates that version.

Set `PHPVM_ROOT` to change the default `~/.phpvm` storage directory.

## Design

```text
~/.phpvm/
  bin/php.cmd              stable PATH entry
  current                  active version marker
  versions/<version>/      extracted PHP distributions
```

Downloads are verified against the SHA-256 value published by the official Windows PHP release registry. ZIP extraction rejects paths that escape the target directory.

## Roadmap

- Linux and macOS binary/build providers
- Composer constraint and `composer.json` detection
- prerelease selectors and cached remote metadata
- PowerShell, Bash, and Zsh completions
- self-update and signed release artifacts
