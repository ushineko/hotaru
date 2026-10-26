# Spec 070: the aula f75

**Issue**: [#193](https://github.com/ushineko/hotaru/issues/193)

## Status: INCOMPLETE

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

Look first at how `SetMode` on the canvas-aware client splits firmware
effects from drawn ones, then at the watcher that notices a power cycle.

## Context

The F75 is on the Windows machine. OpenRGB does not list it, so nothing
lights it today. sanshoku now offers two capabilities on one handle:

- **`lighting.Canvas` and `lighting.Fading`** (sanshoku spec 019). Frames
  through the receiver, at most 37 colours on the board at once, 80 ms floor.
  A frame lasts about a second before the firmware's effect returns, so
  `Hold` is 500 ms and a still frame has to be sent again within it.
- **`lighting.Onboard`** (sanshoku spec 020, PR #57). `Effects`, `Effect`,
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
  the sanshoku release that carries spec 020.
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
- R5.4 Every firmware write is logged with what was written, so a person can
  count them.

### R6. After a power cycle

- R6.1 While the F75's recorded state is a firmware effect, a watcher asks
  the keyboard its model (sanshoku's one-packet question) every 10 s. It
  only notes whether the keyboard answers.
- R6.2 When the keyboard answers after being silent (switched off, out of
  range, or asleep), hotaru reads `Effect`. If the effect or colour differs
  from what hotaru recorded, it calls `SetEffect` and then `SetColour`, and
  logs that it put them back.
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
- R9.4 README: the changelog entry under `### Unreleased` with spec 070 and
  #193.

## Acceptance Criteria

- [ ] The F75 lists with its firmware effects and hotaru's renderers, the
      firmware ones marked stored, with the running effect active.
- [ ] `light set` on a device with stored modes keeps a coloured running
      effect and sets its colour, uses Fixed on where the effect has none,
      and leaves every other device's resolution unchanged.
- [ ] A firmware mode stops the animator and sets the effect, then the
      colour; a renderer draws and does not touch the stored effect.
- [ ] A still frame on a `Fading` canvas is sent again at `Hold`, on a test
      clock; on a canvas that is not `Fading` it is sent once.
- [ ] Reconcile, the reassert interval and previews write no flash, counted
      on the fake. A preview of a colour is drawn.
- [ ] Two firmware writes inside two seconds reach the device as one, the
      later one.
- [ ] After silence, a keyboard that answers on its factory settings gets
      its recorded effect and colour back. One that changed while it kept
      answering does not.
- [ ] `make test`, the GUI tests and `make lint` pass, and
      `make lint-windows`.
- [ ] Live, with someone watching, on the Windows machine: `light set
      purple` keeps the running effect and makes it purple; a scene with
      Rainbow Wave draws it, and a scene with a firmware effect brings the
      effect back.
- [ ] Live, with someone watching: a still drawn colour holds for five
      minutes without flicker.
- [ ] Live, with someone watching: after the keyboard is switched off and
      on, hotaru puts the effect and colour back within 20 s. A change made
      with the keyboard's own keys is kept.
- [ ] The cost in R8.2 is measured and recorded.
- [ ] Docs per R9.

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
- **Depends on #190 and a sanshoku release.** Neither has landed. The code
  waits for both; the spec does not.
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

To be filled in as the work is done.
