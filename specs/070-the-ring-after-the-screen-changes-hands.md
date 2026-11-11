# Spec 070: the ring after the screen changes hands

**Issue**: [#24](https://github.com/ushineko/hotaru/issues/24)

## Status: COMPLETE

## Executive Summary

The Kraken's firmware can turn ring LED 22 green some seconds after the
cooler's screen goes to its own readout, and only a later lighting frame puts
it right. hotaru now sends the cooler's ring its recorded frame again 12 s
after the service takes the screen, and 12 s after each requested switch to
the readout. sanshoku 0.1.16 stops the dashboard passing through the readout
every 8.5 minutes, which was the most frequent switch.

The reboot case is inferred, not tested: it is checked on the machine's
ordinary reboots. Look first at `Retouch` in `internal/service/handback.go`
and `retouch` in `internal/daemon/serve.go`.

## Context

Spec 068 fixed the ring losing frames when a mode change raced a frame. The
same LED then went green with no write from hotaru. Measured on 2026-10-12
with someone watching:

- Switching the screen from a bucket to the firmware readout, and staying
  there, turned LED 22 green, 3 times of 3. A later frame cleared it.
- A service restart left it green. Stopping hands the screen back (sanshoku
  `Close`). A frame 0.3 s after the restart did not prevent it; one 12 s
  after did.
- With the dashboard on, sanshoku's placement climbed through memory and
  cleared it through the readout about every 8.5 minutes. Green appeared
  after some of those clears, not all.

sanshoku 0.1.16 (sanshoku spec 021, #60) stops that climb; a two-hour watched
run kept the screen on the dashboard and the ring clean. A week-long probe
build with the same placement also kept the ring clean.

On 2026-10-27 the user saw the green after a reboot, and it stayed after the
machine was up and hotaru had restored the lights. The restore sends the frame
within a second of start, which is the 0.3 s case above.

A restart into the dev build on 2026-10-27 did not leave the ring green, so
the handback does not do it every time.

## Requirements

**R1. sanshoku 0.1.16.** hotaru moves from sanshoku v0.1.5 to v0.1.16. hotaru
uses its `nzxt`, `steelseries`, `cooling`, `screen`, `lighting`, `hidraw` and
`hwmon` packages. The changes since v0.1.5 in those packages are the
placement fix, the Windows backend for `hidraw` (Linux unchanged), and the
`steelseries` battery retry. The AULA canvas is not scanned: hotaru's canvas
scan names `steelseries` only.

**R2. The ring is sent its frame again after the screen changes hands.**

- R2.1: 12 s after the service takes the cooler, and 12 s after each
  requested switch to the readout, the service reconciles the cooler's own
  lights.
- R2.2: the cooler's own lights are the OpenRGB devices whose location is the
  node the cooler is read at (`HID: /dev/hidrawN`).
- R2.3: a switch inside the wait starts the wait again, so a run of switches
  costs one frame.
- R2.4: the inert rule holds. A ring with nothing recorded, or no cooler, is
  not written.

**R3. The record.** The 0.1.27 changelog says that release stopped the ring
"sticking on one green LED". It did not; it is corrected. Spec 068 records its
multi-day check.

**R4. Cost.** One frame to one device, at start and after a requested readout
switch. Nothing on a timer.

## Acceptance Criteria

- [x] AC1 (R1): hotaru builds, tests and lints against sanshoku v0.1.16.
- [x] AC2 (R2.1, R2.2): `Retouch` writes the recorded frame to the device on
  the cooler's node, and not to a device on another node (unit test).
- [x] AC3 (R2.4): `Retouch` writes nothing with no cooler, or with nothing
  recorded for the ring (unit test).
- [x] AC4 (R2.1): asking for the readout signals a handback (unit test).
- [x] AC5 (R2.1, R2.3): with the daemon attached, the ring is sent its frame
  once after the cooler is taken, and once more after a switch to the
  readout, and not before the wait (unit test).
- [x] AC6 (R3): the 0.1.27 changelog entry is corrected, and spec 068's
  multi-day criterion is recorded.
- [x] AC7: `make test`, the GUI tests and `make lint` pass; the README
  changelog and `docs/hardware.md` say what changed.
- [x] AC8 (live): a build of this branch runs as the service, and its log shows
  the ring sent again after start.

Not an acceptance criterion: the ring clean after a reboot. The maintainer
chose to check that on the machine's ordinary reboots, about every two weeks,
and the result goes on #24.

## Risks & Assumptions

- **Inferred, not tested: the reboot green is the handback green.** The
  12 s delay comes from the restart measurement. A reboot may differ: the
  cooler is powered off between the handback and the start.
- **Assumed: OpenRGB's location names the hidraw node.** Measured for the
  Kraken 2024 Elite on OpenRGB 1.1 (`HID: /dev/hidraw7`, the node sanshoku
  opens). A different location string finds no device and writes nothing.
- **The readout clear in sanshoku stays as the last resort.** It does not
  signal a handback. It needs memory with no room beside the image on
  screen, and the 2-hour run did not reach it.
- **Rollback**: revert the merge. No configuration or state changes.

## Alternatives Considered

- Keep the screen on hotaru's last frame when the service stops. Rejected:
  the screen is handed back so a stopped hotaru does not leave its last frame
  on the cooler while the machine is off (`serveCooler`).
- Re-assert the ring on an interval. Rejected: the maintainer does not want a
  timer to hide the cause, and the project's cost rule asks for the least a
  device needs.
- Send the frame again after every dashboard image. Rejected: it writes every
  three seconds for a switch that happens at start and on request.

## Verification

Tests, 2026-10-28:

- `TestRetouchSendsTheCoolersLightsTheirRecordedFrame` and
  `TestRetouchWritesNothingItWasNotAskedFor` (`internal/service`): AC2, AC3.
  With the node check removed from `Retouch`, both fail.
- `TestAskingForTheReadoutSaysTheScreenWasHandedBack` (`internal/service`):
  AC4.
- `TestTheCoolersLightsAreSentAgainAfterTheScreenChangesHands`
  (`internal/daemon`): AC5. Passes three times under `-race`.
- `make test` passes against sanshoku v0.1.16 (AC1). The first run failed
  `TestClosingStopsEveryQueueAndFreesAnyoneWaiting` in `internal/queue`,
  which is flaky on `main` too; five reruns and the next full run passed.
- `go test -tags migrated_fynedo ./internal/gui/...` and `make lint`
  (0 issues) pass. govulncheck finds nothing the code calls, as on `main`.

Live, 2026-10-28, a build of this branch under the user's service (AC8):

```
01:26:27 hotaru-dev: reading NZXT Kraken Elite V2 at /dev/hidrawN
01:26:27 hotaru-dev: restored 6 devices to what they were showing
01:26:39 hotaru-dev: sent the cooler's lights their colours again after the screen changed hands
```

The switch to the readout was not run live: it is covered by AC5. The ring
after a reboot is checked on the machine's ordinary reboots and reported on
#24.
