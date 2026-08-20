# Contributing to phpvm

Thanks for helping improve phpvm. The current supported runtime platform is Windows x64/x86 and the project itself builds with Go 1.20 or newer.

## Development

```powershell
go test ./...
go vet ./...
go build -o phpvm.exe ./cmd/phpvm
```

Use an isolated data directory while testing commands that install PHP:

```powershell
$env:PHPVM_ROOT = Join-Path $PWD ".test-root"
.\phpvm.exe use 8.4
```

Do not commit downloaded PHP builds, generated executables, credentials, personal paths, or logs. New behavior should include focused tests. Run `gofmt` on Go files and keep PowerShell compatible with Windows PowerShell 5.1.

## Pull requests

Explain the user-visible problem, the chosen behavior, relevant security implications, and how the change was tested. Keep unrelated refactors separate. Changes to installation, checksums, archive extraction, PATH handling, or self-update require an explicit end-to-end test plan.

