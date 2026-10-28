# Spec 068: a mode change races the frame

**Issue**: [#24](https://github.com/ushineko/hotaru/issues/24)

## Status: COMPLETE

## Executive Summary

The Kraken's ring lost writes and stuck on one green LED because hotaru sent
a mode packet before every frame. OpenRGB 1.0 writes a mode change from
another thread, and the frame behind it interleaved with it on the device.

A device already in its per-LED mode now takes the frame alone. After a mode
change, the connection holds that device's next frame for 120 ms. Spec 067's
bounce is not needed and is not merged.

Look first at `holds` in `internal/openrgb/settle.go` and its use in
`Conn.SetFrame`, then at `unchanged` in `internal/service/service.go`.

Merged and released before the multi-day check, at the maintainer's
request, so the release is what gets tested. The spec stays INCOMPLETE until
that check is recorded.

## Context

The NZXT Kraken 2024 Elite's ring loses writes, and after a while its 11
o'clock LED (zone "Hue 2 Channel 1", index 22) turns green and stays green.
While it is stuck, the channel takes frames but does not show them. A mode
change shows them again. Spec 067 bounced the mode to repair it. This spec
removes the cause.

### What was measured

On 2026-10-12, OpenRGB 1.0, with somebody watching the ring. Each run blinked
index 22 between blue and its scene colour once a second for a minute,
writing to OpenRGB directly from a test program.

| Run | Each write | Other traffic on the cooler | Ring |
|---|---|---|---|
| A | frame | none, hotaru stopped | 60 of 60 |
| B | frame | hotaru, a still picture on the screen | 60 of 60 |
| C | frame | hotaru, the dashboard | 60 of 60 |
| D | Direct mode, then frame | hotaru, the dashboard | stuck green |

Before these runs, through hotaru's own write path, a single-LED preview
landed in three of five tries with the dashboard on and three of five with a
still picture. One write moved LED 5 to blue and turned LED 22 green, and
then the channel stopped showing frames. Channel 2, the fans, kept taking
writes. The frames sent while it was stuck appeared when a mode change
cleared it.

So the screen and the status requests are not the cause. The mode packet
hotaru sends with every frame is.

### Why

In OpenRGB 1.0 the two requests reach the device on different threads:

- `UpdateZoneLEDs` from the SDK writes to the device on the server's network
  thread, before it answers.
- `UpdateMode` sets a flag. The controller's own thread then calls
  `DeviceUpdateMode`, which in Direct writes every zone's LEDs.

Both hold the controller's `AccessMutex` shared, so nothing keeps them apart.
A Hue 2 channel is written as three reports: LEDs 1-20, LEDs 21-24, then an
apply. Two such sequences on one channel interleave. The SDK does not say
when the controller's thread has finished.

hotaru sends a mode packet before every frame, even to a device already in
the mode (spec 010). So every write to the cooler starts both threads.

Spec 010 found that the cooler ignored frames sent without the mode packet.
That was measured when hotaru wrote one array for the whole device. Since
spec 011 it writes one request per zone, and runs A to C show the cooler
taking frames with no mode packet at all.

## Requirements

- **R1** After a mode packet to a device, the connection holds that device's
  next frame until a settle period has passed. The hold is per device:
  writes to other devices do not wait.
  - **R1.1** The hold is a timer that closes a channel when it fires. The
    waiter selects on that channel and on its context. It takes the clock
    from the connection, so tests drive it without sleeping.
  - **R1.2** The settle period is `settleDelay`'s 120 ms, moved to one place
    both packages read.
  - **R1.3** A write after the period has passed does not wait. A cancelled
    context ends the wait with the context's error.
  - **R1.4** Every caller of `SetMode` gets the hold: apply, reconcile,
    probe, and spec 057's second mode packet.
- **R2** A write to a device already in the mode asked for, where the mode
  takes a colour per LED and none of its own, sends the frame and no mode
  packet. A write that changes the mode, or a mode's brightness, speed or
  colour, still sends the mode packet.
- **R3** Spec 067 is not merged. Its branch stays as it is until this spec is
  verified.
- **R4** A report of the race goes to OpenRGB, drafted here and posted by the
  maintainer.
- **R5** `docs/hardware.md` describes the behaviour, `docs/credits.md` needs
  no change, and the README changelog has the entry.

## Acceptance Criteria

- [x] A frame written straight after a mode packet to the same device is
      held until the settle period has passed, measured on a test clock.
- [x] A frame to another device is not held.
- [x] A frame written after the settle period has passed is not held.
- [x] A cancelled context ends the hold with an error, and nothing is
      written.
- [x] A write to a device already in Direct sends no mode packet.
- [x] A write that changes the mode, or the brightness of the current one,
      sends the mode packet.
- [x] `make test`, the GUI tests and `make lint` pass.
- [x] Live, with someone watching: through hotaru, LED 22 is previewed blue
      and back 30 times. Every change shows and the ring does not stick.
- [x] Live, with someone watching: a mode packet, then the frame after the
      settle period, once a second for a minute, from the test program. The
      ring does not stick.
- [x] Live, with someone watching: two or three scenes applied with the
      development build. Every lit device in the case changes.
- [x] Live: a few days of ordinary use with the dashboard on, and the ring
      not stuck. The result goes on #24.

## Risks & Assumptions

- **Assumed: 120 ms covers the controller's thread.** For a Hue 2 device in
  Direct it sends about six reports. Spec 057 measured 120 ms as enough for a
  keyboard to finish changing mode.
- **R2 changes what every device receives.** Spec 010 recorded the cooler
  ignoring frames without the mode packet. The live scene check covers the
  rest of the machine.
- **Cost**: a write that follows a mode change waits up to 120 ms. A write
  of a new frame to a device already in Direct sends one request fewer than
  before.
- **Rollback**: revert the merge. No configuration or state changes.

## Alternatives Considered

- Keep the mode packet on every write and always wait after it. Rejected:
  every write on every device pays the wait, for a packet that changes
  nothing.
- Write the frame before the mode, so the controller's thread sends it.
  Rejected: specs 056 and 057 found the order matters on the keyboards, and
  the order there was settled by measurement.
- Keep spec 067's bounce. Rejected as the fix: a bounce is a mode change
  followed by a frame, the same race. It repaired the ring only when it did
  not lose.

## Verification

Tests, in `internal/openrgb/settle_internal_test.go` unless named:

- `TestAFrameAfterAModeChangeWaitsForItToSettle`: held until the clock
  fires, and the timer asked for is `SettleDelay`.
- `TestAFrameToAnotherDeviceIsNotHeld` and
  `TestAWriteToAnotherDeviceGoesAheadWhileOneIsHeld`: the second reaches the
  server through `Conn.SetFrame`.
- `TestAFrameAfterTheModeHasSettledDoesNotWait`.
- `TestTheHoldRunsFromTheLastModeChange`.
- `TestACallerThatGivesUpIsNotHeld` and
  `TestAHeldFrameThatIsGivenUpOnWritesNothing`: the server is asked for
  nothing.
- `TestADeviceAlreadyInItsPerLEDModeTakesTheFrameAlone`
  (`internal/service`), which replaces spec 010's
  `TestTheModePacketIsSentEvenWhenTheDeviceIsAlreadyInThatMode`.
- `TestBrightnessIsAssertedOnEveryWrite` (`internal/service`), its mode now
  given a brightness: the packet still goes to a device already in the mode.
- `TestAPerLEDModeTakesTheModeBeforeTheFrame` (`internal/service`): a device
  arriving in Direct still takes the mode first.

Each was checked against a mutation that undoes the fix: no wait in
`SetFrame`, no skip in the write path, a skip despite a brightness, and a
zero settle period. Each made a test fail. Not covered by a unit test:
`SetMode` starting the hold. Testing it needs a server that answers a
controller listing, and the live checks cover it.

`make test`, `go test -tags migrated_fynedo ./internal/gui/...` and
`make lint` (0 issues) pass.

Live, 2026-10-12, development build of this branch under the user's
service, the dashboard on the screen, `bounce` removed from the rules:

- Through hotaru, LED 22 previewed blue and reconciled back, 30 cycles at
  1.5 s. Every change showed, and the ring ended on the scene colour.
- The test program, a mode packet, 120 ms, then the frame, once a second for
  a minute. The first run ended with LED 22 green, but nobody was watching
  and the time it happened is not known. The next write corrected it, so the
  channel had not stuck. The second run, watched throughout and for a minute
  after, blinked every second and ended on the scene colour.
- Scenes `cyan`, `forest1` and `red-sunset` applied ten seconds apart. Every
  device changed at each step, and nothing was left wrong.

Open: the green LED after the first unwatched run is not explained. The
multi-day check is where it shows again, if it does.

Multi-day check, 2026-10-12 15:53 to 2026-10-27 21:32: a probe build of
0.1.27 under the user's service, with the dashboard on. The probe build also
carried a sanshoku change that kept the screen off the firmware readout when
memory filled. The user reported the ring clean for the whole run. The one
readout switch in its log was at shutdown before a reboot.

The check ran with that sanshoku change, so it shows this spec's fix with the
readout switches removed. About 20 minutes after 0.1.27 was installed without
it, LED 22 went green with no write from hotaru. That second cause, the
screen's switch to the readout, is spec 070, and it likely explains the
unwatched green above.

## Upstream report (draft)

To be posted to OpenRGB by the maintainer.

> **NZXT Hue 2: a mode update and a zone LED update from the SDK race on the
> device**
>
> OpenRGB 1.0, NZXT Kraken 2024 Elite (0x3012).
>
> An SDK client sends `UPDATEMODE` (Direct) and then `UPDATEZONELEDS` for
> each zone. `ProcessRequest_RGBController_UpdateZoneLEDs` calls
> `UpdateZoneLEDs`, which writes the zone on the network thread under
> `AccessMutex.lock_shared()`. `UpdateMode` sets `CallFlag_UpdateMode`, and
> `DeviceCallThreadFunction` calls `DeviceUpdateMode` under the same shared
> lock, which in Direct writes every zone. The two writes interleave on the
> device.
>
> On the Kraken, `SetChannelLEDs` sends three reports per channel. With the
> sequences interleaved, frames are lost, and the ring's channel ends up
> showing a wrong colour on one LED and ignoring Direct writes until the mode
> changes. With only `UPDATEZONELEDS`, 180 frames in a row landed. With
> `UPDATEMODE` before each, the channel stuck within a minute.
>
> Taking the lock exclusively in the device thread and in `UpdateZoneLEDs`,
> or routing `UpdateZoneLEDs` through the device thread, would keep them
> apart.
