## Validation Report: builds and passes on windows, make targets (spec 067)

**Date**: 2026-11-10 16:00
**Commit**: on top of afd4c52, branch `feat/windows-build`
**Status**: PASSED on Windows. Linux not run.

Run on Windows 11 Pro 26200, Go 1.27.2 (lint under go1.26.0), MSYS2 UCRT64
gcc, OpenRGB 1.1.0. Make was run from PowerShell; see the spec for why not
from Git Bash.

### Phase 3: Tests

- `go test -count=1 ./...`: every package ok.
- `go test -count=1 -tags migrated_fynedo ./internal/gui/...`: ok.
- `make lint`: 0 issues. `make lint-windows`: 0 issues (it failed before
  this change).
- `GOOS=linux CGO_ENABLED=0 go vet` on every package but the window's: clean.
- `make build gui`: `bin\hotaru.exe` and `bin\hotaru-gui.exe`.
- Live: the service, the CLI and the window ran against each other and
  OpenRGB. The board's fault is outside hotaru (#197).
- Not run: anything on Linux. ssh to the Linux machine refused this
  machine's key.
- Status: PASSED on Windows, INCOMPLETE on Linux.

### Phase 4: Code Quality

- The change is two Makefile lines and documentation. Nothing to refactor.
- Status: PASSED

### Phase 5: Security Review

- govulncheck under Go 1.27.2: two standard-library findings in called code
  (GO-2026-6859 `net/http`, GO-2026-6809 `crypto/x509`), fixed in Go 1.27.4.
  Two in `golang.org/x/net` 0.60.0, not called (GO-2026-6866, GO-2026-6860).
- Fix: `golang.org/x/net` moves to 0.62.0, with `x/sys` 0.49.0 and `x/text`
  0.43.0. Under `GOTOOLCHAIN=go1.27.4`: no vulnerabilities, for Windows and
  for Linux without the window.
- A release binary should be built with Go 1.27.4 or later.
- OWASP: no new input, route, file access or subprocess. No secrets.
- Status: PASSED

### Phase 5.5: Release Safety

- Change type: build tooling, documentation, a dependency patch bump.
- Rollback: revert the commit. On Linux `go env GOEXE` is empty, so build
  output names do not change.
- Status: PASSED

### Overall

- All gates passed on Windows: YES. The Linux run stays open in the spec.
