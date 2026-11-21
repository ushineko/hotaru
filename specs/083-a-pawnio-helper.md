# Spec 083: a pawnio helper

**Issue**: [#234](https://github.com/ushineko/hotaru/issues/234)

## Status: INCOMPLETE

## Executive Summary

On Windows, an MSI board's RGB header and Corsair memory are reached
through PawnIO, which serves only SYSTEM and administrators. A second
service, `hotaru-pawnio`, runs as LocalSystem, holds those devices, and
draws what hotaru's service sends it over a pipe only `NT SERVICE\hotaru`
and administrators may open. The service keeps its own account. Review
`internal/pawnhelper` (the handler's checks, `listen`, `dial`) and the
service entry in `fynstall.yaml` first.

## Context

Spec 082 registered sanshoku 0.6.0's drivers. On Windows, `msi.Board` and
`corsairdram` open PawnIO, and the installed service, as
`NT SERVICE\hotaru`, is refused (sanshoku #110). The maintainer chose a
separate helper, as hayami did for the processor's temperature (hayami spec
103), over running the whole service with more rights (2026-11-20).

## Requirements

### The helper

- R1 `hotaru pawnio-helper [--allow SID]`, dispatched before the CLI, runs
  as the service `hotaru-pawnio` through `internal/host`. It shares no file,
  log or socket with the service; what goes wrong goes to the event log.
- R2 It limits library loads to System32, checks the three PawnIO modules
  beside it (`LpcIO.bin`, `SmbusPIIX4.bin`, `SmbusI801.bin`) against their
  SHA-256 before anything loads them, and scans `msi.Board` and
  `corsairdram.Driver`, keeping what is a `lighting.Canvas`.
- R3 It serves `\\.\pipe\hotaru-pawnio`, duplex, first instance its own,
  remote clients refused, with the access list
  `D:P(A;;GA;;;SY)(A;;GA;;;BA)(A;;GRGW;;;<NT SERVICE\hotaru>)`.
- R4 JSON lines, one request and one answer, version checked, a line at
  most 64 KiB: `list` (scans only when it holds nothing), `frame` (a listed
  device; only that device's key IDs; at most 1024 lights), `release`. Any
  other request is refused. A device that went is dropped and reported gone.
- R5 Stopping it closes the devices and hands none back.

### The service

- R6 On Windows the canvas drivers have the helper's driver in place of
  `msi.Board` and `corsairdram`; elsewhere those two are scanned directly.
- R7 It opens the pipe anonymously and believes it only when the process
  serving it is the one the service manager started for `hotaru-pawnio`. A
  helper that is not installed is absence.
- R8 The helper's text reaches a person without control characters, cut to
  256 bytes.

### Packaging

- R9 `fynstall.yaml` installs the modules and their licence beside the
  program, and declares `hotaru-pawnio` with the service manager's own
  account (LocalSystem).

## Acceptance Criteria

- [ ] `make test`, `make lint`, `make lint-windows`, GUI tests.
- [ ] Tests (`internal/pawnhelper`): the service finds, draws on and
      releases the helper's devices through the pipe, and only canvases are
      kept; frames naming another device's light or too large are refused;
      unknown requests and versions are refused; a device that went is
      gone; no helper is absence and the helper's words are cleaned; a line
      too long ends the connection; stopping leaves the last frame; the
      committed modules match the pinned hashes.
- [ ] Live on the Windows test machine, installed from an installer built
      from this branch: both services run, the board's header and both
      memory modules take a colour, with the maintainer watching.
- [ ] The architecture guide (rule and diagram), the project guidelines,
      `docs/hardware.md`, credits and the changelog say what changed.

## Out of scope

- Installing PawnIO itself.
- Starting the helper only while the service reads it: it waits on its pipe,
  idle, and loads nothing until asked.

## Risks & Assumptions

- **A LocalSystem service** that takes requests. It answers three, each
  checked against the devices it listed, offers no bus or register, and its
  pipe opens for one account and administrators. Whoever runs as
  `NT SERVICE\hotaru` can light the board and memory, which that account
  could ask of the service anyway.
- **The whole hotaru program runs as LocalSystem** as the helper, though
  only this path is reached.
- **PawnIO**: a review on 2026-11-20 (hayami spec 103) found no published
  vulnerability and no malware or blocklist report; one maintainer, and the
  newest signed driver predates fixes made in August 2026.
- **LibreHardwareMonitor** holds the bus mutexes for seconds and stalls
  these devices; hayami no longer needs it.
- **Cost:** the helper idles on its pipe; frames cross it only when the
  animator sends one, at the devices' floors (50 ms board, 200 ms memory).
- **Rollback:** revert. Uninstalling removes both services; the modules go
  with the program.

## Alternatives Considered

- Running the service as LocalSystem: rejected; the service serves an HTTP
  API to every signed-in user, and that API would become a path to SYSTEM.
- A helper that offers SMBus and Super I/O transactions: rejected; it would
  hand the service's account the memory's SPD and every register.

## Verification

Pending.
