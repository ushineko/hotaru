# Spec 071: the aula f75

**Issue**: [#193](https://github.com/ushineko/hotaru/issues/193)

## Status: COMPLETE

## Executive Summary

hotaru lights the AULA F75 keyboard through its 2.4 GHz receiver in two ways.
Its own firmware effects are modes like any other: `light set purple` keeps
the effect the keyboard is running and makes it purple, and a scene can name
an effect and a colour. hotaru's own effects, and colours per key, are drawn
as frames, as on the Apex, and a still frame is sent again before the
keyboard takes its lights back.

The effect and its colour are stored in the keyboard's flash, so hotaru
writes them only when someone asks, never for a preview, and at most once
per two seconds. A power cycle puts the keyboard on its factory settings;
hotaru notices it come back and puts the effect and colour back, but does
not undo a change made with the keyboard's own keys.

Look first at `chooseStored` in `internal/service/canvas.go`, which splits
firmware effects from drawn ones and decides when the keyboard's storage is
written, then at `watchStored` in `internal/daemon/stored.go`, which notices
a power cycle.

## Context

The F75 is on the Windows machine. OpenRGB does not list it, so nothing
lights it today. sanshoku now offers two capabilities on one handle:

- **`lighting.Canvas` and `lighting.Fading`** (sanshoku spec 019). Frames
  through the receiver, at most 37 colours on the board at once, 80 ms floor.
  A frame lasts about a second before the firmware's effect returns, so
  `Hold` is 500 ms and a still frame has to be sent again within it.
- **`lighting.Onboard`** (sanshoku spec 023, PR #57). `Effects`, `Effect`,
  `SetEffect` and `SetColour`. The effect and each effect's colour live in
  the keyboard's flash. `SetEffect` writes the config block, which survives
  a power cycle. `SetColour` writes the palette where the colour is new, and
  a new palette colour does not survive a power cycle: the keyboard comes
  back on Neon stream with every effect on its factory settings.

The user's expectation, 2026-10-26: setting a colour keeps the keyboard's
effect. Drawing frames is an option, used when asked for. A power cycle is
rare, and hotaru puts things back afterwards, as it does for the Apex.

hotaru already has the shape for this. Spec 060 lists a canvas device as a
`devices.Device` whose modes are hotaru's renderers, routes `SetMode` and
`SetFrame` to it, and lets scenes, previews and reconcile reach it with no
second path. The F75 adds firmware modes to that list.

## Requirements

### R1. Finding the keyboard

- R1.1 `internal/canvas` adds sanshoku's `aula.Driver` to its drivers. The
  receiver's device is opened as a canvas device as today. hotaru depends on
  the sanshoku release that carries spec 023: 0.1.19.
- R1.2 A device that is also `lighting.Onboard` keeps that capability in the
  adapter, beside the canvas. A canvas device without it behaves as now.
- R1.3 OpenRGB does not list the F75, so it has no twin. The twin rule is
  unchanged for devices that do.

### R2. Two kinds of mode

- R2.1 The F75 lists as one `devices.Device` with one zone, "Keys", of 80
  LEDs. Its modes are the keyboard's effects, in the firmware's order, then
  hotaru's renderers (Static, Breathing, Spectrum, Rainbow Wave). No name is
  shared between the two lists; if a firmware name ever matched a renderer's,
  the firmware's is listed and the renderer's is not.
- R2.2 A firmware mode carries `Stored: true`, a new field on
  `devices.Mode`, meaning "the device's own, and setting it writes the
  device's storage". A firmware effect that takes a colour is `ModeColour`
  with one slot (`ColoursMin` and `ColoursMax` 1) and its stored colour;
  Off and Self-define take none. `ActiveMode` is the running effect while
  nothing is drawn, and the renderer while frames are drawn.
- R2.3 `SetMode` with a firmware mode stops the animator, calls `SetEffect`,
  and then `SetColour` with the style's colour where the effect takes one.
  The firmware's effect shows within about a second of the last frame
  (sanshoku spec 019 E4); the effect and colour are written before that.
- R2.4 `SetMode` with a renderer, and `SetFrame`, draw as spec 060 does. The
  keyboard's stored effect is not touched, and shows again when drawing
  stops.
- R2.5 A frame that names more than one colour cannot be a firmware effect
  and is drawn. A frame of one colour, with no mode asked for, is R3.

### R3. `light set` keeps the effect

- R3.1 On a device with `Stored` modes, a solid colour with no mode asked
  for keeps the running firmware effect and sets its colour. This is the
  first candidate `SolidCandidates` returns for such a device.
- R3.2 Where the running effect takes no colour (Off, Self-define), the
  colour goes to "Fixed on", the firmware's one-colour effect.
- R3.3 `off` on the F75 is the firmware's Off effect, a config block write.
- R3.4 A scene that names a mode gets that mode, firmware or renderer, as on
  every device. A scene that names none is R3.1.
- R3.5 Only devices with `Stored` modes behave this way. Every other device
  resolves a solid colour as it does today.

### R4. Holding a drawn frame

- R4.1 The animator sends a still frame again on a canvas that is
  `lighting.Fading`, at `Hold`, and on no other canvas. A frame identical to
  the last is still sent on this clock, because it is what keeps the frame
  up. The Apex, which is not `Fading`, sends one frame and then nothing, as
  now.
- R4.2 An animated effect on a `Fading` canvas runs at its interval as now.
  Its frames keep the canvas up by themselves.
- R4.3 When drawing stops, nothing is sent. The keyboard returns to its
  stored effect by itself. `light release` on the F75 stops drawing and
  sends nothing (sanshoku's `Release` sends nothing on this device).
- R4.4 Added during the work: a moving effect runs no faster than 120 ms on
  the F75. At sanshoku's 80 ms floor a moving frame flickered, hotaru's
  Rainbow Wave and sanshoku's own bench wave alike, because a frame at the
  message limit can take 110 ms to write. sanshoku 0.1.19 raises the floor
  (sanshoku #68), and hotaru's animator already never goes under it.

### R5. Flash

- R5.1 Firmware writes happen only on an apply: `light set`, `light off`, a
  scene applied by a person, a hotkey or a rule, and the re-assert in R6.
  Reconcile and the reassert interval never write the F75's flash; sanshoku
  writes nothing where the stored value already matches, and hotaru does
  not ask where its record already matches what it last read.
- R5.2 A preview never writes flash. A preview of a firmware effect's
  colour is drawn as a Static frame for the length of the lease, and the
  stored effect shows again when the lease ends. A preview that names a
  different firmware effect is skipped, with the reason that it would write
  the keyboard's storage.
- R5.3 At most one firmware write per device per two seconds. The device's
  single-slot queue already keeps only the latest request; a request inside
  the two seconds waits for the end of them, and a newer one replaces it.
  The queue does not cancel a write that is already waiting, so in a burst
  the first write lands, the second lands two seconds later, and every one
  between them is replaced by the last.
- R5.4 Every firmware write is logged with what was written, so a person can
  count them.

### R6. After a power cycle

- R6.1 While the F75's recorded state is a firmware effect, a watcher asks
  the keyboard its battery question every 10 s: one 20-byte report each way.
  It only notes whether the keyboard answers. Changed from the model
  question, which sanshoku keeps private; the cost is the same.
- R6.2 When the keyboard answers after being silent (switched off, out of
  range, or asleep), hotaru reads `Effect`. If it reads the factory effect,
  Neon stream, hotaru writes the recorded effect and colour back where they
  differ, and logs that it put them back. Refined from "if the effect or
  colour differs": sleep keeps the settings, so a keyboard that wakes on
  anything but Neon stream is one somebody chose, and R6.3 keeps it. A read
  that fails is tried again on the next tick: the first read after a
  power-on lost 2 of 35 packets on the desk.
- R6.2a The same check runs when the keyboard is attached, so a power cycle
  while hotaru was not running is put back when it starts.
- R6.3 While the keyboard keeps answering, a change of effect or colour is
  not undone. A person pressing the keyboard's own lighting keys has asked
  for something, and the latest write wins.
- R6.4 The watcher stops while frames are drawn, since frames keep the
  keyboard's lights hotaru's. It does not run on a device with no `Stored`
  modes.
- R6.5 The interval and the silence rule take an injectable clock, and the
  tests drive them without sleeping.

### R7. Parity and the window

- R7.1 No new route. `light list`, `light set`, `light off`, scenes and the
  window's effect picker reach the F75 through the existing routes, because
  its firmware effects are modes.
- R7.2 `light list` and the window mark a `Stored` mode as the keyboard's
  own, and say that setting it writes the keyboard. `docs/api.md` gains the
  `stored` field.
- R7.3 `light health` reports the F75's watcher: answering or silent, and
  when it last put the effect back.

### R8. Cost

- R8.1 Stated: the watcher is one 20-byte report each way every 10 s, about
  10 ms of radio. A still drawn frame is one message every 500 ms: for one
  colour on every key, 84 body bytes in six packets, about 18 ms.
- R8.2 Measured and recorded in Verification: the service's CPU with the F75
  attached for no scene, a firmware effect, a still drawn frame and Rainbow
  Wave; and the battery level by `0x4A` before and after an hour of a
  firmware effect and an hour of a still drawn frame.

### R9. Documentation

- R9.1 `docs/hardware.md`: the F75 row, its two kinds of mode, that its
  effect and colour are stored in flash, and what a power cycle does.
- R9.2 `docs/architecture.md`: stored modes beside drawn ones, and the
  watcher.
- R9.3 `docs/api.md`: the `stored` field.
- R9.4 README: the changelog entry under `### Unreleased` with spec 071 and
  #193.

## Acceptance Criteria

- [x] The F75 lists with its firmware effects and hotaru's renderers, the
      firmware ones marked stored, with the running effect active.
- [x] `light set` on a device with stored modes keeps a coloured running
      effect and sets its colour, uses Fixed on where the effect has none,
      and leaves every other device's resolution unchanged.
- [x] A firmware mode stops the animator and sets the effect, then the
      colour; a renderer draws and does not touch the stored effect.
- [x] A still frame on a `Fading` canvas is sent again at `Hold`, on a test
      clock; on a canvas that is not `Fading` it is sent once.
- [x] Reconcile, the reassert interval and previews write no flash, counted
      on the fake. A preview of a colour is drawn.
- [x] A firmware write inside two seconds of the last waits for the end of
      them (R5.3 as amended).
- [x] After silence, a keyboard that answers on its factory settings gets
      its recorded effect and colour back. One that changed while it kept
      answering does not. A failed first read is tried again.
- [x] `make test`, the GUI tests and `make lint` pass, and
      `make lint-windows`, on Windows and on Linux.
- [x] Live, with someone watching, on the Windows machine: `light set
      purple` keeps the running effect and makes it purple; a scene with
      Rainbow Wave draws it, and a scene with a firmware effect brings the
      effect back.
- [x] Live, with someone watching: a still drawn colour holds for five
      minutes without flicker.
- [x] Live, with someone watching: after the keyboard is switched off and
      on, hotaru puts the effect and colour back. A change made from outside
      hotaru while the keyboard answers is kept.
- [x] The CPU cost in R8.2 is measured and recorded. The battery half moves
      to [#199](https://github.com/ushineko/hotaru/issues/199): the keyboard
      was charging, which pins its level.
- [x] Docs per R9.

## Out of scope

- The F75 on its cable. sanshoku's cable device has no `Onboard`, and it
  lights as a canvas as spec 060 already handles.
- Stored per-key colours (the Self-define block). The receiver does not
  carry them.
- An effect's brightness or speed through the firmware. Brightness on a
  drawn frame is the renderer's multiply, as on the Apex.
- Other devices with stored modes. The field is general; the F75 is the only
  device that sets it.

## Risks & Assumptions

- **Flash wear.** Each colour change writes one or two sectors. The chip's
  endurance is not known. R5 limits writes to explicit applies, at most one
  per two seconds, and never on reconcile or preview. A rule that switches
  scenes often would still write each time; R5.4 lets that be seen.
- **The re-assert can disagree with a person.** A keyboard switched off and
  on after its owner changed it with its own keys gets hotaru's record
  back. R6.3 keeps changes made while it was answering; a change made and
  then followed by a power cycle is lost. That is the same trade the Apex
  makes.
- **Sleep looks like a power cycle.** The keyboard dozes after a few minutes
  idle and stops answering. Waking, it answers with its settings unchanged,
  so R6.2 reads, finds nothing different and writes nothing. The read costs
  about 200 ms of radio per wake.
- **A drawn frame costs battery.** About two messages a second for as long
  as a drawn scene is up. R8.2 measures it. A firmware effect costs nothing
  between writes.
- **The 37-colour limit.** A drawn frame with more colours is shown with its
  nearest colours merged (sanshoku spec 019).
- **The vendor's program.** AULA's software writes the same flash and
  streams on the same receiver. hotaru cannot arbitrate; `docs/hardware.md`
  says so.
- **Depends on #190 and a sanshoku release.** sanshoku 0.1.19 carries spec
  023 and the 120 ms floor. #190 is the branch this one is built on.
- **The listing can be stale.** hotaru reads the keyboard's effect when it
  attaches, when it writes, and when the keyboard comes back from silence.
  An effect chosen with the keyboard's own keys while it answers shows in
  the keyboard and not in `light list` until then. Reading on every tick was
  rejected for its radio cost (see Alternatives).
- **Rollback**: revert the PR. The keyboard keeps whatever effect and colour
  were last written, until it is changed with its own keys or power-cycled.

## Alternatives Considered

- **Always draw.** One path, already written. Rejected: it replaces the
  keyboard's effect, which is not what the user asked for, and it costs
  radio traffic for as long as a colour is shown.
- **Two devices, "AULA F75" for frames and another for its effects.**
  Simpler routing. Rejected: they are the same lights, and a scene would
  have to light one and leave the other alone.
- **Re-assert on an interval, as rules do for forgetful devices.** Already
  built. Rejected: it would write flash on every tick that found a
  difference, and it would undo a change made with the keyboard's own keys.
- **Read `Effect` on the interval instead of asking the model.** Detects a
  reset without waiting for silence. Rejected: two block reads, about 200 ms
  of radio, every 10 s, and it cannot tell a reset from a person's change.

## Verification

2026-11-10, Windows 11, Go 1.27.2, OpenRGB 1.1.0, sanshoku 0.1.19, the F75
through its receiver on 2.4 GHz with its cable in for charge. Each live check
was watched by the user.

- `go test ./...`, `go test -tags migrated_fynedo ./internal/gui/...`,
  `make lint` and `make lint-windows`: pass, 0 issues. The watcher's tests
  ran 50 times under `-race`.
- Started with nothing recorded for the keyboard: listed with its 20 effects
  marked `*` and the four renderers, on "Ripples shining, its own effect".
  Nothing was written to it.
- `light set purple`: "wrote the effect "Ripples shining" in #8000ff to its
  storage", 1.4 s for the whole command. Seen: purple ripples.
- A scene with Rainbow Wave drew it. At sanshoku 0.1.18's 80 ms floor it
  flickered, and so did sanshoku's bench wave at 80 ms. The bench wave was
  smooth at 120 ms and at 160 ms. With 0.1.19 hotaru's wave ran at 120 ms,
  8.3 frames a second, and was smooth.
- A scene with Reaction in cyan stopped the drawing. Seen: cyan on key press.
- A still drawn orange frame, sent again every 500 ms, held for six and a
  half minutes with no flicker.
- Power cycle. The first try showed the watcher's read failing on the first
  answer ("the palette: no reply: 33 of 35 packets"), and nothing was put
  back. With the retry, the service's start put Reaction back on its own,
  because the keyboard was still on Neon stream. A power cycle on Reaction in
  cyan did not reset the keyboard: no palette colour had been written. On
  Reaction in purple, which is written to the palette, the keyboard came back
  on Neon stream and the log said "came back on its factory settings; put
  "Reaction" back". Seen: Reaction in purple. The time from switch-on was not
  taken with a clock; the watcher asks every 10 s.
- A change from outside hotaru while the keyboard answered: sanshoku's bench
  set Respire. 35 s later the keyboard still read Respire and hotaru had
  written nothing. Seen: breathing. `light list` still said Reaction, as the
  Risks section says.
- CPU of the service, from Windows' process time over 60 s, whose resolution
  is 15.6 ms: 62 ms with the firmware effect and the watcher, 62 ms with a
  still drawn frame, 688 ms (1.2% of one core) with Rainbow Wave at 120 ms.
  "No scene" was not separated from the firmware effect: both are the
  watcher alone.

2026-11-10, Linux: CachyOS, Go 1.27.3 (lint under go1.26.0), golangci-lint v2.12.2, in a fresh clone of the pushed branch.

- `make test`: every package ok. `go test -tags migrated_fynedo
  ./internal/gui/...`: ok.
- `go test -race -count=20` on `internal/daemon`, `internal/canvas` and
  `internal/service`: ok.
- `make lint` and `make lint-windows`: 0 issues. `make build gui`: ok.

Not run: the battery measurement, moved to #199. The F75 was not tried on
Linux: the live criteria name the Windows machine.
