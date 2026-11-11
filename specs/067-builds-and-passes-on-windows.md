# Spec 067: builds and passes on windows

**Issue**: [#190](https://github.com/ushineko/hotaru/issues/190)

## Status: INCOMPLETE

## Executive Summary

hotaru is written for Linux. This is the first step of a Windows port: the
service, the CLI and the window build there, the tests pass there, and
`hotaru serve` in a terminal drives lights through OpenRGB.

Nothing changes on Linux. New behaviour is in `_windows.go` files or behind a
check of the system. Readings from the machine, running in the background,
hotkeys, the cooler, canvas devices and packaging are later specs.

Look first at `internal/config/paths_windows.go` for where files go, then at
`internal/api/socket_windows.go` for who may connect, then at
`internal/openrgb/gone_windows.go` for how a lost server is recognised.

## Context

Measured on Windows 11 (26200), Go 1.27.0, with the MSYS2 UCRT64 gcc for the
window:

- At the pinned sanshoku 0.1.5 nothing builds: its `usbfs` and `hidraw`
  packages call `unix.Syscall` with no build constraint. sanshoku has built on
  Windows since its spec 012.
- With sanshoku 0.1.14 and fynedesygn 0.1.96 both binaries build with no
  source change.
- 16 tests fail, for five reasons:

| Reason | Where |
|---|---|
| Paths assume XDG and `HOME`, and `/tmp/cfg` is not absolute on Windows | `internal/config` |
| A socket path over the limit: `t.TempDir()` is named for the test, under a long profile path | `internal/api`, `internal/cli`, `internal/gui` |
| A file mode asserted as `0600`; Windows reports `0666` for any file | `internal/api`, `internal/desktop` |
| `syscall.Socketpair` does not exist | `internal/openrgb` |
| `/etc/hostname` and `/dev/null` as stand-in files | `internal/cli` (hidden behind the socket failure) |

- AF_UNIX sockets work through Go's `net` on Windows: listen, dial, the stale
  socket check and the events stream.
- A reset TCP connection reads as `WSAECONNRESET`, which does not match
  `syscall.ECONNRESET`. A test written for this spec showed it: the client did
  not count the connection as gone, so it would not have redialled.
- This checkout has `core.autocrlf=true` and no `.gitattributes`, so every
  text file is CRLF in the working tree and `gofmt -l` lists all of them.

## Requirements

### R1. Dependencies

- R1.1 sanshoku moves to 0.1.14 and fynedesygn to 0.1.96.
  `main` later moved sanshoku to 0.1.16 on its own, and the merge keeps that.

### R2. Where files go

- R2.1 An XDG variable that names an absolute directory wins on every system,
  as now.
- R2.2 Without one, on Windows: rules, scenes and dashboards are in
  `%APPDATA%\hotaru`. State is in `%LOCALAPPDATA%\hotaru\state`, added files in
  `%LOCALAPPDATA%\hotaru\data`, and the socket in `%LOCALAPPDATA%\hotaru\run`.
- R2.3 With the profile variable unset there is nowhere to put a file, and the
  error names the variable.
- R2.4 Linux and every other system keep their paths and their errors,
  byte for byte.

### R3. The socket

- R3.1 The transport stays an AF_UNIX socket on Windows.
- R3.2 After listening, the socket is made this user's alone. Elsewhere that
  is mode `0600`. On Windows the socket file's access list is replaced with
  one entry, full control for the user the service runs as, and protected
  from inheritance.
- R3.3 A failure to restrict the socket closes the listener and fails
  `Listen`, as a failed `chmod` does now.

### R4. A lost OpenRGB server

- R4.1 `WSAECONNRESET` and `WSAECONNABORTED` count as the connection having
  gone, on Windows only.
- R4.2 `openrgb.PeerGone` is what each system reports for a peer that has
  gone, so a test that makes the fake vanish says the same thing everywhere.

### R5. Hotkeys

- R5.1 Off Linux the service does not look for a session bus. It records that
  hotkeys are not available on this system yet, and `hotaru keys` says so.
  Before this the D-Bus library tried to run `dbus-launch`.
- R5.2 `internal/desktop` still compiles on Windows and is not called by the
  service there. Moving it behind an interface belongs to the hotkeys spec.

### R6. Tests

- R6.1 `go test ./...` and `go test -tags migrated_fynedo ./internal/gui/...`
  pass on Windows and open no device.
- R6.2 `internal/testsock` gives a test a socket path short enough to listen
  on. Tests that listen use it.
- R6.3 Mode assertions are in files Windows does not build, or skipped there
  with the reason. The Windows test of the socket reads its access list.
- R6.4 The socket-pair tests use a loopback TCP connection on Windows.
  `TestNothingInTheModuleOpensANetworkListener` no longer reads test files:
  a test file is not in the program.
- R6.5 A test resets a real connection and requires `isGone` to say so.

### R7. The working tree

- R7.1 `.gitattributes` pins LF for text files.
- R7.2 `make lint-windows` lints the files only Windows builds, without the
  window, from any system.
- R7.3 `restartPlan` moves into the Linux file that calls it, since nothing
  else does and the Windows lint reports it unused.
- R7.4 `make build` and `make gui` name their output with `go env GOEXE`, so
  Windows gets `hotaru.exe`. Elsewhere the suffix is empty.
- R7.5 `make lint-windows` passes golangci-lint package directories, not
  import paths, which it cannot read.

### R8. Documentation

- R8.1 `docs/api.md` says where the socket is on Windows.
- R8.2 The README's changelog has the entry. The README still describes a
  Linux program, which is what a person can install today.
- R8.3 `docs/hardware.md` says the OpenRGB server has to include the fix for
  [#191](https://github.com/ushineko/hotaru/issues/191): any release except
  1.0. OpenRGB 1.1 (2026-10-16) has it. When this was first written no
  release did, and the text asked for a build from `master`.
- R8.4 `docs/hardware.md` says that LibreHardwareMonitor starting after
  OpenRGB stops an MSI board's strip, and what brings it back
  ([#197](https://github.com/ushineko/hotaru/issues/197)).

## Acceptance Criteria

- [x] `CGO_ENABLED=0 go build ./cmd/hotaru` and
      `go build -tags migrated_fynedo ./cmd/hotaru-gui` succeed on Windows.
- [x] `go test ./...` and the GUI tests pass on Windows.
- [x] `go vet ./...` passes on Windows, and for `GOOS=linux` on the packages
      that need no cgo.
- [x] The pinned golangci-lint reports 0 issues on Windows, and for
      `GOOS=linux` on the packages that need no cgo.
- [x] `make test` and `make lint` pass on Linux, and `make lint-windows`
      reports 0 issues there.
- [x] The reset test fails without R4.1 and passes with it.
- [x] On Windows the socket's access list is one entry, for the current user,
      protected.
- [ ] A second local user on Windows cannot connect to the socket.
- [x] `hotaru serve` starts on Windows with nothing recorded, connects to
      OpenRGB and writes nothing.
- [x] One colour applied through OpenRGB on Windows with someone watching.
- [x] Docs per R8.

## Out of scope

- CPU, memory and temperature readings on Windows.
- Running without a console, logging to a file, stopping at logoff, autostart.
- Hotkeys on Windows.
- The cooler, the Apex and the AULA F75.
- An installer and a release artefact.
- The words hotaru prints about `systemctl` when the service is not running.

## Risks & Assumptions

- **Linux was checked once, by hand.** hotaru has no CI. The branch was
  built on Windows, and its tests, lints and build were then run on Linux
  from the pushed branch. The Linux paths are the old code moved into
  `paths_unix.go`.
- **The access list is asserted, not attacked.** The test reads the list. No
  second user tried to connect. The default directory is inside the user's
  profile, which other users cannot open in any case.
- **A window between listen and restrict.** The socket exists for an instant
  with the directory's inherited access list, as it exists for an instant
  with the default mode on Linux. On Linux the directory is `0700`. On
  Windows the default directory inherits the profile's list.
- **The listener guard is narrower.** It no longer reads test files. A test
  that opened a port would not be caught. The program's files are all still
  read.
- **Line endings.** `.gitattributes` makes a Windows checkout LF. A checkout
  made before this needs its working tree converted once.
- **Rollback**: revert the PR. No file on disk changes format, and on Linux no
  path changes.

## Alternatives Considered

- **A named pipe with a DACL instead of AF_UNIX.** A second transport to
  write and test, and `curl --unix-socket` would stop working. AF_UNIX works
  as it is.
- **Restricting the directory instead of the socket.** `--socket` can name a
  path in any directory, and changing the access list of a directory hotaru
  did not make is not its business. The socket file is hotaru's.
- **A loopback pair for the socket-pair tests on every system.** The Linux
  tests would change for no gain there.
- **Moving `internal/desktop` behind a build tag now.** The CLI and the
  service both read claims from it. It belongs with the Windows hotkeys,
  which need the interface anyway.

## Verification

2026-10-11, Windows 11 Pro 26200, Go 1.27.0 (lint under Go 1.26.0), MSYS2
UCRT64 gcc.

- `go test -count=1 ./...`: every package ok.
  `go test -count=1 -tags migrated_fynedo ./internal/gui/...`: ok.
- `go vet ./...`: clean. `GOOS=linux go vet` on `internal/config`,
  `internal/api`, `internal/openrgb`, `internal/systemd` and
  `internal/daemon`: clean.
- golangci-lint v2.12.2: 0 issues on Windows. With `GOOS=linux` and no cgo,
  0 issues on every package but the window's two. `make lint-windows` was
  checked with `make -n`; the MSYS make on this machine does not pass the
  profile variables through, so the recipe was run by hand.
- Before R4.1, `TestAConnectionResetOnWindowsIsGone` failed: "a reset read as
  ... wsarecv: An existing connection was forcibly closed by the remote host.
  is not counted as gone". After, it passes.
- The socket's access list read back as `D:PAI(A;;FA;;;<the user's SID>)`.
- `hotaru serve` in a terminal, nothing recorded:

  ```
  no hotkeys: no session bus: exec: "dbus-launch": executable file not found in %PATH%
  hotaru dev listening on C:\Users\<user>\AppData\Local\hotaru\run\hotaru.sock
  connected to the OpenRGB server at 127.0.0.1:6742 (protocol 3)
  no cooler telemetry yet: no supported liquid cooler
  no device to draw on yet: no device to draw on: absent
  ```

  The first line is what R5.1 then removed. `hotaru status`, `light list`,
  `light health`, `scene list`, `readings` and `keys` answered. `light list`
  showed OpenRGB 1.0.0's five devices, all in scope.
- `hotaru light set red`, watched: both memory sticks and the Razer dock
  turned red. The Razer mouse did not, and the board's strip did not. hotaru
  reported the board as unconfirmed, in its own words: "took the mode and
  does not report the colours it was sent".
- The board is explained, and is issue #191. The OpenRGB 1.0 server reads a
  protocol 3 client's device position as an id. A controller had left the
  list, so the board was at position 4 with id 5, and the write was dropped.
  A protocol 3 write of `fffffd` left the recorded colour at `ffffff`; a
  protocol 6 write of `fffffe` to id 5 was recorded.
- With OpenRGB `master` at `4d17bac5`, after a rescan that moved every id to
  5–9, hotaru's write path turned the strip red and then blue, watched.
- The mouse is not explained. Its position and id matched. It may have been
  asleep, and it was not retested.

2026-10-26, the OpenRGB 1.1.0 release from winget, at `afd4c52`. The
installer runs OpenRGB as a Windows service (LocalSystem, automatic start),
and its window attaches to that service as a client.

- `live_test.go` passed: protocol 3, five devices, the memory sticks
  included.
- `hotaru serve` connected at protocol 3 and restored the five devices to
  what it had recorded.
- After a rescan from OpenRGB's window, a protocol 6 read of the server named
  it "OpenRGB 1.1" and listed six controllers at positions 0–5 with ids
  6–11. The sixth was the DualSense over Bluetooth.
- `hotaru light set red`, then `blue`, watched: all six devices changed each
  time, the board's strip and the Razer mouse included, and nothing else
  changed. hotaru reported every device written, none unconfirmed. On 1.0
  every one of these writes would have gone to another position or nowhere.

2026-11-10, Go 1.27.2 (lint under go1.26.0), OpenRGB 1.1.0 as a service.

- From PowerShell, `make build gui` built `bin\hotaru.exe` and
  `bin\hotaru-gui.exe`, both reporting `0.1.26-afd4c52-dev`. Before R7.4 they
  were `bin/hotaru` and `bin/hotaru-gui`, which Windows does not run by name.
- `make lint`: 0 issues. `make lint-windows` failed before R7.5, with
  `stat ...\github.com\ushineko\hotaru\internal\api: directory not found`, and
  reported 0 issues after it.
- The 2026-10-11 note blamed MSYS make for losing the profile variables. The
  cause is Git Bash starting MSYS2's make: two different MSYS runtimes, and
  every variable but `PATH` and `HOME` is dropped. Make run from PowerShell
  passes them through.
- `go test -count=1 ./...` and the GUI tests: every package ok.
- `hotaru serve`, the CLI (`status`, `light list`, `light health`, `scene
  list`, `keys`, `readings`) and `hotaru-gui.exe` ran against each other.
- The board did not follow `scene apply purple`. A read of the server showed
  the colour recorded. Watched: restarting OpenRGB brought the board back,
  starting LibreHardwareMonitor stopped it again, and restarting OpenRGB after
  that kept it working. Not hotaru's fault; documented per R8.4.
- Restarting OpenRGB showed the first request after it fail on the dead
  connection, after which the service redialled and restored. That is the
  design on every system, filed as
  [#198](https://github.com/ushineko/hotaru/issues/198).

2026-11-10, Linux: CachyOS, Go 1.27.3 (lint under go1.26.0), golangci-lint v2.12.2, in a fresh clone of the pushed branch.

- `make test`: every package ok. `go test -tags migrated_fynedo
  ./internal/gui/...`: ok.
- `make lint` and `make lint-windows`: 0 issues.
- `make build gui`: `bin/hotaru` and `bin/hotaru-gui`, without `.exe`.

Not run: a second Windows user.
