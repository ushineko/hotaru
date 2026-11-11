## Validation Report: a windows service (spec 072)

**Date**: 2026-11-11 00:00
**Commit**: on top of 8f0f670, branch `feat/windows-service`
**Status**: PASSED. Two watched checks wait for the maintainer.

### Phase 3: Tests

- Windows: `make test`, `go test -tags migrated_fynedo ./internal/gui/...`,
  `make lint`, `make lint-windows`: pass, 0 issues.
- Linux, a fresh copy with the change: the same, and `make build gui`.
- New tests: the service handler (start, interrogate, stop, shutdown, a
  failing main, a main that will not stop); the log's stamps and roll;
  machine paths, XDG winning, the user's own directory; the directories'
  and socket's access lists read back; the owner check; Windows remedies;
  the window's start command on both systems; `fynstall.yaml`'s version.
- Live on Windows: install, the service's account and directories, a
  non-administrator client, a write, a restart, uninstall and reinstall.
- Coverage: not measured.
- Status: PASSED

### Phase 4: Code Quality

- `internal/systemd` became `internal/services`, with the same exported
  shape; its two callers changed only their import.
- The access-list code is in one file, `config/machine_windows.go`, used by
  the directories and the socket.
- Status: PASSED

### Phase 5: Security Review

- govulncheck under Go 1.27.4: no vulnerabilities. oksvg and rasterx move
  from indirect to direct at the same versions, for the build step
  `scripts/icon` only.
- The service runs as `NT SERVICE\hotaru`, not LocalSystem.
- A directory in `%ProgramData%` that somebody else made is refused, so no
  user can prepare hotaru's directory for it. Subdirectories are checked the
  same way, and a link in their place is refused.
- The socket and the settings are open to interactive users, as decided;
  services and network sign-ins get no entry. State, pictures and the log
  are read-only to them.
- No secrets, personal paths or serials in committed files. The device path
  in Verification is the F75's interface path, with no serial number.
- Status: PASSED

### Phase 5.5: Release Safety

- Change type: code, a Windows installer, and a package rename inside
  `internal/`.
- Linux: no behaviour change; `host.Run` is the signal handling `main` had.
- Rollback: on Windows, the uninstaller, then `hotaru serve` from a
  terminal. In the code, revert the PR.
- Status: PASSED

### Overall

- All gates passed: YES. Watched: the window from the Start Menu, and the
  lights, wait for the maintainer.
