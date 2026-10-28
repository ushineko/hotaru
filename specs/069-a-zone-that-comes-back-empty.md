# Spec 069: a zone that comes back empty

**Issue**: [#194](https://github.com/ushineko/hotaru/issues/194)

## Status: COMPLETE

## Executive Summary

OpenRGB 1.1 did not read the zone sizes 1.0 had saved, so the ASUS board's
four addressable headers came up at 0 LEDs. hotaru wrote the recorded colours
to the one LED left and reported the board restored.

hotaru now records each zone's size with the colours. A restore resizes a
resizable zone that came back smaller, within its range, before writing.
Restore, `hotaru reconcile`, the service log and `light health` name a device
whose colours still do not fit. Scene apply resizes nothing.

Look first at `restoreOne` and `regrow` in `internal/service/reconcile.go`,
then `Conn.ResizeZone` in `internal/openrgb/conn.go`.

## Context

After an upgrade from OpenRGB 1.0 to 1.1 and a reboot, the ASUS ROG MAXIMUS
Z790 HERO came up with its four addressable headers at 0 LEDs. The board
showed its firmware rainbow. hotaru logged `restored 5 devices`, `light
health` said healthy, and a scene apply lit the board's one mainboard LED.

### What was measured

On 2026-10-27, OpenRGB 1.1, through the SDK protocol from a test program:

| Zone | 1.0 entry in `Configuration.json` | Live after boot | 
|---|---|---|
| Aura Mainboard | 1 (min 1, max 1) | 1 |
| Addressable RGB Header 1-4 | 16 each (min 0, max 120) | 0 each (min 0, max 500) |

- Correcting the hidraw node in the 1.0 entry and restarting OpenRGB did not
  bring the sizes back. OpenRGB 1.1 reports `leds_max` 500 where 1.0
  reported 120, and it did not use the 1.0 entry.
- One `RESIZE_ZONE` (packet 1000) per header, size 16, brought the device to
  65 LEDs. OpenRGB 1.1 then wrote a 1.1 entry to `Configuration.json`.
- After that, `scene apply` lit 6 of 6 devices and the board showed the
  scene.

### Why hotaru said nothing

- Restore writes the recorded frame (65 colours) to the device (1 LED). The
  write succeeds, so the device counts as restored.
- State records colours, not the zone layout, so restore cannot tell which
  zone lost its LEDs.
- Health checks the server and scope, not whether a recorded device still
  has the LEDs it was recorded with.
- Scene apply does report targets that run off a zone, as problems. That
  part works.

## Requirements

**R1. Zone limits are read.** `devices.Zone` carries the zone's minimum and
maximum LED count from the server. A zone is resizable when its maximum is
greater than its minimum.

**R2. hotaru can resize a zone.** `openrgb.Client` gains
`ResizeZone(ctx, device, zone, size)`. `openrgb.Conn` sends `RESIZE_ZONE`
for the zone's index. `openrgb.Fake` changes the zone's count, the zone
offsets after it, the device's LED count and its colours, as the server
does.

**R3. Zone sizes are recorded.** When a write is remembered, state records
each zone's LED count by name (`zones:`), next to the colours.

- R3.1. The key is optional. A state file without it reads as before, and
  restore then resizes nothing.

**R4. Restore puts a zone's size back.** Before restore writes a device, each
recorded zone that the device reports with fewer LEDs than recorded is
resized to the recorded size, where:

- R4.1. the zone is resizable, and the recorded size is within its minimum
  and maximum;
- R4.2. a zone is never made smaller;
- R4.3. the device is read again after a resize, and the frame is written to
  what it reports then;
- R4.4. each resize is in the restore's result (`resized`), and in the
  service log.

**R5. A restore that cannot fit the frame says so.** Where, after R4, a
device has a different number of LEDs from the recorded frame, restore still
writes and reports a problem naming both counts. The service log carries it,
and `hotaru reconcile` prints it.

**R6. Health names the mismatch.** `light health` and the window's health
note list each recorded device whose LED count differs from its recorded
frame, and offer `hotaru reconcile` and OpenRGB's zone sizing as remedies.
The state stays healthy: hotaru can still drive the device.

**R7. Scene apply does not resize.** A scene apply records the zones as they
are. A zone made smaller on purpose in OpenRGB, then lit by a scene, stays
that size.

## Acceptance Criteria

- [x] AC1 (R1): `convert` reads `ZoneLedsMin` and `ZoneLedsMax`; a zone with
  max > min is resizable, a fixed zone is not (unit test).
- [x] AC2 (R2): `Fake.ResizeZone` on a middle zone moves the zones after it,
  changes the LED count and keeps the colours of the LEDs that stay (unit
  test).
- [x] AC3 (R3): a scene apply records every zone's size; a state file with no
  `zones` key loads, and restore of it resizes nothing (unit tests).
- [x] AC4 (R4): restore of a device whose resizable zones report 0, with 16
  recorded, resizes each to 16, writes a full frame, and reports the resizes
  (service test against the fake).
- [x] AC5 (R4.1, R4.2): restore leaves alone a zone that is not resizable, a
  recorded size past the zone's maximum, and a zone larger than recorded
  (service test).
- [x] AC6 (R5): restore of a device that still has fewer LEDs than its frame
  reports a problem naming both counts (service test).
- [x] AC7 (R6): health with a recorded device whose LED count differs names
  the device and offers remedies, and stays healthy (service test).
- [x] AC8 (R7): a scene apply onto a short resizable zone sends no resize
  (service test).
- [x] AC9 (live): with the 1.0-format `Configuration.json` put back and
  OpenRGB restarted (`hotaru light rescan`), the service resizes the ASUS
  headers to 16, logs it, and the board shows the scene, with somebody
  watching.
- [x] AC10: `make test` and `make lint` pass; docs/api.md, docs/hardware.md
  and the changelog are updated.

## Risks & Assumptions

- **A resize is a write to OpenRGB's configuration.** OpenRGB 1.1 saves the
  new size to `Configuration.json`. hotaru resizes only to a size it
  recorded from the same zone, so the saved file ends up as it was before
  the size was lost.
- **A deliberate shrink between a scene and a restart is undone.** A person
  who makes a header smaller in OpenRGB and restarts the server before
  applying a scene gets the recorded size back, because the recorded frame
  is laid out for it. Applying any scene records the new size (R7).
- **OpenRGB 1.0.** `RESIZE_ZONE` and the zone limits are in every protocol
  version hotaru speaks. On 1.0 the saved sizes load, the zones match the
  record, and nothing is resized. Not checked: whether 1.0 saves a resize
  sent over the network. If it does not, restore repeats it after each
  server start.
- **Cost.** One comparison per recorded zone at restore. A resize happens
  only when a zone is short; none in steady state.
- **Rollback.** Revert the commit. The `zones` key in state is then ignored.

## Alternatives Considered

- Considered resizing during scene apply to fit the scene's targets;
  rejected because it would undo a deliberate resize in OpenRGB, and a typo
  in a target (`[99]`) would grow a zone.
- Considered inferring zone sizes from the recorded frame's length; rejected
  because a device with several resizable zones gives no way to split it.
- Considered editing OpenRGB's `Configuration.json`; rejected because the
  format changed between 1.0 and 1.1, and hotaru does not own that file.

## Verification

Unit and service tests, against `openrgb.Fake`:

| AC | Test |
|---|---|
| AC1 | `TestAZoneIsReadWithTheSizesItCanBeGiven` (internal/openrgb) |
| AC2 | `TestResizingAZoneMovesTheZonesAfterIt` (internal/openrgb) |
| AC3 | `TestASceneRecordsTheSizeOfEveryZone`, `TestZoneSizesSurviveARestart`, `TestAStateFileFromBeforeZoneSizesHasNone`, and the "state file from before zone sizes" case below |
| AC4 | `TestARestoreGivesEmptiedHeadersBackTheirSize` |
| AC5, AC6 | `TestARestoreLeavesAloneWhatItCannotOrShouldNotResize` |
| AC7 | `TestHealthNamesADeviceItsColoursNoLongerFit` |
| AC8 | `TestASceneDoesNotResizeAZone` |

`make test`, `go test -tags migrated_fynedo ./internal/gui/...` and
`make lint` pass. `internal/queue`'s
`TestClosingStopsEveryQueueAndFreesAnyoneWaiting` fails now and then on
`main` as well (1 run in 20 with `-count=20`); this change does not touch it.

### Live (AC9), 2026-10-27, OpenRGB 1.1, with somebody watching the board

1. Development build under the user service, `scene apply nighttime1`. State
   recorded `zones` for the board: Aura Mainboard 1, Addressable RGB Header
   1-4 16 each.
2. The 1.0-format `Configuration.json` put back, then `hotaru light rescan`
   at 21:32:15. The service log:

   ```
   the OpenRGB server at 127.0.0.1:6742 came back (protocol 3); putting the lights back
   ASUS ROG MAXIMUS Z790 HERO: resized Addressable RGB Header 1 from 0 to 16 LEDs
   ASUS ROG MAXIMUS Z790 HERO: resized Addressable RGB Header 2 from 0 to 16 LEDs
   ASUS ROG MAXIMUS Z790 HERO: resized Addressable RGB Header 3 from 0 to 16 LEDs
   ASUS ROG MAXIMUS Z790 HERO: resized Addressable RGB Header 4 from 0 to 16 LEDs
   ```

3. `light list`: the board at 65 LEDs, in Direct. `light health`: healthy,
   with no mismatch. OpenRGB wrote a 1.1 entry to `Configuration.json` with
   16 LEDs per header.
4. The board and its header strips showed the scene, with no firmware
   rainbow.
