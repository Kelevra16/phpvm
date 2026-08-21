# Changelog

## [Unreleased]

### Added

- `phpvm self-update` with GitHub Release checksum verification and deferred executable replacement.
- `phpvm which [build]`.
- `phpvm cache dir|clear`.
- Native PowerShell completion generation.
- Laragon detection and safe junction management.
- Composer constraint selection for caret, tilde, comparisons, wildcards, AND, and OR expressions.
- Public installer validation in CI.
- Issue templates, contribution guidance, and a security policy.
- `phpvm shell [version|--current]` for terminal-local PHP sessions that can run concurrently.
- Dynamic `php` and `phpize` shims with session, project, and global resolution priority.
- `phpvm resolve [--path] [version]` for inspecting the effective build.

### Fixed

- The installer now puts its canonical binary directory before the managed PHP wrapper directory and preserves a legacy `~/.phpvm/bin/phpvm.exe` as `phpvm.exe.legacy`, preventing old development binaries from shadowing a release.

## [0.1.0] - 2026-08-20

- First Windows release with version installation, TS/NTS and x64/x86 builds, project configuration, profiles, extensions, logs, diagnostics, integrity verification, aliases, execution matrices, and transactional storage.

[Unreleased]: https://github.com/Kelevra16/phpvm/compare/v0.1.0...HEAD
[0.1.0]: https://github.com/Kelevra16/phpvm/releases/tag/v0.1.0
