# Spec 072: a windows service

**Issue**: [#202](https://github.com/ushineko/hotaru/issues/202)

## Status: INCOMPLETE

## Executive Summary

hotaru runs as a Windows service, `NT SERVICE\hotaru`, installed by a
fynstall installer. It starts with the computer, keeps its files under
`%ProgramData%\hotaru`, and any user signed in at the machine may use it.
`internal/host` is the one place that knows how hotaru was started, and
`internal/services` asks systemd or the Windows service manager about
OpenRGB. Linux behaves as before.

## Context

Spec 067 made hotaru build and run on Windows, from a terminal. Nothing
starts it at boot, and it stops when the terminal closes. The lights belong
to the machine, not to whoever is signed in, and OpenRGB already runs as a
Windows service on the machine this was measured on.

The Windows port plan chose a per-user process at logon, because a service
runs in session 0, cannot register hotkeys, and has no user's files. That
was reconsidered:

- Hotkeys on Linux belong to KDE, which runs the CLI. A Windows hotkey
  client in the user's session would do the same, through the API.
- Lights are shared hardware, so one record for the machine fits better
  than one per user.
- A service hears about sleep, resume and sign-in directly.

Measured on Windows 11 (26200) before this spec: a program run as a service
under `NT SERVICE\<name>` opened the AULA F75's vendor HID collection,
asked it a question, read its effect, reached OpenRGB on `127.0.0.1:6742`,
and made a directory in `%ProgramData%`. fynstall 0.2.7 can register such a
service (fynstall spec 010).

Decided with the maintainer:

- Any user signed in at the machine, locally or over Remote Desktop, may
  control the lights. Services and network sign-ins may not.
- The service runs as its own virtual account, `NT SERVICE\hotaru`, not
  LocalSystem.
- No third-party service library (kardianos/service was considered). Which
  manager started hotaru is hidden behind one small package, so that one
  could replace it later without touching the rest.
- Install and uninstall belong to packaging: the AUR package on Arch,
  fynstall on Windows. The program does not install itself.

## Requirements

### R1 One service layer

- R1.1 A new package, `internal/host`, is the only code that knows how
  hotaru was started. It offers:
  - `Run(name, main)`: runs `main(ctx)` and returns its exit code. Under the
    Windows service manager it answers the manager, reports running, and
    cancels `ctx` on stop or shutdown. Anywhere else it cancels `ctx` on
    Ctrl+C or SIGTERM, as `cmd/hotaru` did itself.
  - `Managed()`: true when this process is a Windows service.
  - `Installed()`: true when Windows has a service named `hotaru`. Any user
    may ask. Always false on Linux.
- R1.2 `cmd/hotaru` and `cmd/hotaru-gui` call `host`; nothing under
  `internal/` but `host` imports `golang.org/x/sys/windows/svc`.
- R1.3 On Linux nothing changes: systemd's unit is `Type=simple`, so
  `Run` is the signal handling that was in `main`.
- R1.4 A stop request is answered within the manager's 30 seconds: `ctx` is
  cancelled, and `Run` waits up to 20 seconds for `main` to return. A `main`
  that returns non-zero without being asked to stop reports a failure, so
  the recovery actions start it again.

### R2 What the machine's service manager says about OpenRGB

- R2.1 `internal/systemd` becomes `internal/services`. Its exported shape
  stays: `Look`, `Facts`, `Remedies`, `RestartCommand`, `Bounce`, `Held`.
  `Facts` gains `Manager`: `systemd`, `scm` or none.
- R2.2 On Windows, `Look` asks the service manager, with any user's rights,
  whether a service named `OpenRGB` exists and runs. `Remedies` and
  `RestartCommand` name PowerShell's `Start-Service OpenRGB` and
  `Restart-Service OpenRGB`, run as an administrator. There is no lingering
  remedy on Windows.
- R2.3 `Bounce` on Windows tries to restart the OpenRGB service. Refused
  access is `ErrNotRestartable`, and the caller offers the command, as for
  a Linux system unit without passwordless sudo.
- R2.4 `Held` stays Linux-only: it reads `/proc`.

### R3 Files of the machine

- R3.1 `config` gains a machine mode, set once at start by `cmd/` with
  `config.SetMachine`. In it, on Windows, the directories are:
  - settings (rules, scenes, dashboards): `%ProgramData%\hotaru`;
  - state: `%ProgramData%\hotaru\state`;
  - pictures: `%ProgramData%\hotaru\data`;
  - the socket: `%ProgramData%\hotaru\run`.
  An XDG variable that is set still wins. On Linux the mode does nothing.
- R3.2 The service sets machine mode when `host.Managed()`. A client (the
  CLI, the window, the setup wizard) sets it when `host.Installed()`, so it
  finds the socket and the rules of the installed service.
- R3.3 The window's own settings (`gui.yml`) stay with the user: a new
  `config.UserDir` is the per-user directory whatever the mode.
- R3.4 The service prepares the directories before it opens anything:
  - It makes `%ProgramData%\hotaru` if it is not there.
  - If it is there, its owner must be SYSTEM, Administrators or the
    service's own account. Otherwise the service stops with a message
    naming the owner and the fix: an administrator removes the directory.
    Any user can make a directory in `%ProgramData%`, so this stops one
    user from preparing hotaru's directory for another.
  - The settings directory gets a protected access list: full control for
    SYSTEM, Administrators and the service; modify for interactive users,
    inherited. Modify lets a signed-in user edit the rules, as on Linux.
  - `state`, `data` and `run` get protected lists: full control for SYSTEM,
    Administrators and the service; read and execute for interactive users.
- R3.5 In machine mode the socket's list is: full control for SYSTEM,
  Administrators and the service; read and write for interactive users
  (`IU`). A terminal `hotaru serve` keeps spec 067's list of one user.
- R3.6 There is no migration of per-user files. hotaru on Windows has had
  no release, so nobody has any.

### R4 The log

- R4.1 A Windows service has no console. In machine mode the service writes
  what it would print to `%ProgramData%\hotaru\state\hotaru.log`. At 1 MiB
  the file is renamed `hotaru.log.1`, replacing an older one, and a new one
  is started.
- R4.2 Interactive users may read the log (R3.4).

### R5 The window and the CLI

- R5.1 The window's "service is not running" card names the command for
  this system: `systemctl --user start hotaru` on Linux,
  `Start-Service hotaru` on Windows.
- R5.2 `--socket`'s help names the socket of this system.

### R6 The installer

- R6.1 A `fynstall.yaml` at the root, for `windows/amd64` only, built with
  fynstall v0.2.7, the release that has `account: own`:
  - `install.scopes: [system]`, into `{programs}/hotaru`.
  - Payload `bin/hotaru.exe` and `bin/hotaru-gui.exe`, both on `PATH`.
  - A service `hotaru`: `bin/hotaru.exe serve`, `account: own`,
    `restart: on-failure`, started.
  - A Start Menu entry for the window.
  - `keep_on_uninstall: {config}/hotaru`, which is `%ProgramData%\hotaru`
    for an install for everyone.
- R6.2 `make installer` builds both programs and the installer into
  `dist/`. It runs fynstall with `go run` at the pinned version, so nothing
  is installed. The icon is rendered from `packaging/hotaru.svg` at build
  time.
- R6.3 The window is built with `-H windowsgui` on Windows, so starting it
  opens no console.
- R6.4 A test holds `fynstall.yaml`'s version to `.tag`.

### R7 Docs

- `docs/architecture.md`: the service layer, and machine mode.
- `docs/packaging.md`: the Windows installer, what it installs, and how to
  remove it.
- `docs/api.md`: where the socket is on Windows.
- README changelog. The port plan's process-model decision is reversed.

## Acceptance Criteria

- [x] `host.Run`'s Windows handler, driven by a test: reports running,
      answers Interrogate, cancels on Stop and on Shutdown, reports a
      failure when `main` returns non-zero by itself.
- [x] `config` in machine mode returns the R3.1 paths; XDG still wins;
      `UserDir` is the user's whatever the mode.
- [x] Preparing the directories sets the R3.4 lists, read back by a test;
      a directory owned by somebody else is refused.
- [x] The socket's list in machine mode is the R3.5 list, read back.
- [x] `services.Facts` on Windows gives the PowerShell remedies; Linux's are
      unchanged (the existing tests pass unchanged).
- [x] The log rolls at its size, on a test with a small limit.
- [x] `make test`, the GUI tests, `make lint` and `make lint-windows` pass
      on Windows and Linux.
- [x] Live, on the Windows machine: the installer installs, the service
      runs as `NT SERVICE\hotaru`, and `hotaru light list` reaches it from
      the signed-in user's session with no flag.
- [ ] Live, with someone watching: the window, opened from the Start Menu,
      shows the devices of the installed service.
- [ ] Live, with someone watching: `hotaru light set` through the
      installed service changes the board and the F75.
- [x] Live: after `Restart-Service hotaru` the lights are put back and the
      log says so.
- [x] Live: the uninstaller removes the service and the programs and keeps
      `%ProgramData%\hotaru`.
- [x] Docs per R7.

## Risks & Assumptions

- **Boot is not tested.** A restart of the computer was not part of the
  live checks: it was not this session's to take. `Restart-Service` is the
  stand-in; the start type is automatic.
- **OpenRGB's order.** OpenRGB and hotaru both start with the computer, in
  no fixed order. hotaru already retries OpenRGB with backoff. #197 is
  unchanged: LibreHardwareMonitor starting after OpenRGB still stops the
  board, and the remedy now names `Restart-Service OpenRGB`.
- **Sleep and resume** are not handled on Windows here. The service
  manager can report them; that is follow-up work.
- **Interactive users can edit the rules**, as the owner can on Linux. A
  rule names a scene's picture by path, which the service reads as
  `NT SERVICE\hotaru`: that account reads what any user may, and no more.
- **A second user** sharing the lights was decided, not tested here.
- **Rollback**: uninstall with the uninstaller, then run `hotaru serve` from
  a terminal as before. Revert the PR to remove the code; Linux is
  unchanged by it.

## Alternatives Considered

- **kardianos/service.** Its value is installing the service from the
  program, which packaging already does twice (the AUR's unit, fynstall).
  The part hotaru needs, answering the service manager, is about 60 lines
  on `x/sys/windows/svc`, which hotaru already depends on.
- **A per-user process at logon** (the port plan's choice). Lights come back
  only once somebody signs in, and each user gets a record of their own for
  shared hardware.
- **LocalSystem.** Simpler, and more than hotaru needs. fynstall gained
  `account: own` for this.
- **A named pipe.** Spec 067 kept AF_UNIX; an access list on the socket's
  file does the same job.

## Verification

2026-11-10, Windows 11 Pro 26200, Go 1.27.2, OpenRGB 1.1.0 as a LocalSystem
service, sanshoku 0.1.19, fynstall 0.2.7. Linux: CachyOS, Go 1.27.3.

- Windows: `make test`, the GUI tests, `make lint` and `make lint-windows`
  pass, 0 issues. Linux, from a fresh copy with the change applied: the
  same, and `make build gui`.
- `make installer`: `dist\hotaru-0.1.29-windows-amd64-installer.exe`,
  121 MB, and its uninstaller. The dry run listed the shortcut, the
  uninstall entry, the `PATH` entry and "service hotaru ... as NT
  SERVICE\hotaru, started".
- Installed with `--cli --yes` from an administrator's shell. `sc qc hotaru`:
  `AUTO_START`, `SERVICE_START_NAME : NT SERVICE\hotaru`, running.
- The service made `%ProgramData%\hotaru` with `state`, `data` and `run`.
  The lists, read back with `Get-Acl`, were R3.4's and R3.5's, all protected,
  and owned by `NT SERVICE\hotaru`.
- Its log, `state\hotaru.log`: listening on the shared socket, connected to
  OpenRGB with protocol 3, "drawing on AULA F75". Nothing was written to any
  device: nothing was recorded.
- A shell started with `runas /trustlevel:0x20000`, whose token had
  Administrators as deny-only and INTERACTIVE enabled, ran `hotaru light
  list` with no flag and listed six devices, the F75's own effects marked.
  The same shell ran `hotaru light set purple`, which set all six. The log
  said "AULA F75: wrote the effect "Respire" in #8000ff to its storage".
- `Restart-Service hotaru` returned at once. The log: "restore incomplete: 5
  devices put back, still waiting for AULA F75", then "restored 6 devices to
  what they were showing" one second later, once the keyboard attached.
- The uninstaller removed the service (`sc query`: 1060, does not exist),
  `C:\Program Files\hotaru`, the `PATH` entry and the shortcut, and kept
  `%ProgramData%\hotaru` with its record and log. Installed again, the
  service accepted the directory it had made, as the same account, and
  restored the six devices.
- Before the spec: a separate program run as a service under `NT
  SERVICE\<name>` opened the F75, asked it a question, read its effect and
  reached OpenRGB.

Not run: the window from the Start Menu, and a watched look at the lights,
because nobody was at the machine. A restart of the computer. A second
signed-in user.
