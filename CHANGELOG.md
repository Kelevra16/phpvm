# Changelog

## [Unreleased]

## [0.6.0-rc1] - 2026-09-05

### Added

- User-friendly control center with `phpvm ui`, a compact `dashboard`, and numbered accessible selectors.
- Interactive `use`/`install`, project initialization, guided diagnostics, and safe confirmations for destructive maintenance.
- `phpvm open logs|ini|root`, actionable error hints, installation stage feedback, and Spanish interactive messages through `PHPVM_LANG=es`.

## [0.5.0] - 2026-09-04

### Added

- Ready-to-use development `php.ini` provisioning for new PHP builds, including common bundled extensions, staging validation, metadata, and `phpvm ini defaults` for existing builds.
- Project-aware `ini` and `ext` targeting with explicit `--project`, `--global`, and `--version` overrides.
- Interactive console presentation with color, Unicode status symbols, compact progress bars, readable tables, `NO_COLOR`, `--plain`, `--no-color`, and `--verbose` support.
- Historical Windows PHP catalog back to 5.2, compact/all listings, EOL warnings, lifecycle channels, and build information.
- Versioned historical checksum manifest plus explicit opt-in for archive packages without a maintained hash.
- Post-install executable/runtime validation and Visual C++ diagnostics.
- Reproducible `phpvm lock` and `phpvm restore` environments.
- External PECL package search, HTTPS ZIP installation, and source-based updates.
- Verified managed Composer installation and execution.
- Safe copying and registration of existing PHP installations with `phpvm import`.
- `phpvm self-update` with GitHub Release checksum verification and deferred executable replacement.
- `phpvm which [build]`.
- `phpvm cache dir|clear`.
- Project environment summaries with `phpvm status` and Composer platform checks with `phpvm check`.
- Content-sensitive project trust, `PHPVM_SAFE_MODE`, and guarded project execution workflows.
- Development server launcher, safe `doctor --fix`, and framework-oriented INI presets.
- Attestation-verified PIE setup and execution against the project-selected PHP build.
- Content-addressed PHP archive cache, strict `--offline` installation, cache verification, and portable checksum-manifest bundles.
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

[Unreleased]: https://github.com/Kelevra16/phpvm/compare/v0.6.0-rc1...HEAD
[0.6.0-rc1]: https://github.com/Kelevra16/phpvm/compare/v0.5.0...v0.6.0-rc1
[0.5.0]: https://github.com/Kelevra16/phpvm/compare/v0.1.0...v0.5.0
[0.1.0]: https://github.com/Kelevra16/phpvm/releases/tag/v0.1.0
