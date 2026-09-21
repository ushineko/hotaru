# Spec 032: the display, and a machine without one

**Issue**: [#73](https://github.com/ushineko/hotaru/issues/73)

## Status: COMPLETE

## Context

Everything this program does with the panel -- a dashboard, a picture, a scene
that sets one -- is drawn on a screen it has to have found first, and nothing
said whether it had. A machine with no display learned it by pressing "Show
it" and watching nothing happen.

### What was found, said once

`Known` carries the panel with the model now: an Elite V2 has a 640x640 LCD,
and that is a fact about the hardware rather than something to ask the device.
Asking would mean claiming the interface, and a program that claimed somebody's
screen to find out whether they had one would take it off liquidctl to answer a
question nobody asked.

So System has a Display row, and `hotaru cooling` a display line: the panel
found, "none" for a cooler without one, or the panel and the reason when it
cannot be reached.

### A screen that will not open is an absence, not a fault

A panel is not openable on every machine -- another program holds the
interface, or this user may not open the usbfs node. Every attempt failed with
whatever the kernel said, which meant:

- every scene reported "the screen: claim interface 0: permission denied",
  permanently, on a machine whose lights had just done exactly what was asked.
  All nine shipped scenes name a screen state, so this is every scene.
- the dashboard loop tried again every two seconds for the life of the
  service, writing the same line to the journal forty thousand times a day and
  drawing nothing either way.

`ErrNoScreen` wraps the cause, and callers treat it the way they already treat
`ErrNoCooler`: spec 012's degradation rule, applied to the panel rather than
the device. The scene applies, the lights change, nothing is drawn, and the
reason is on screen in System once.

The dashboard loop stops after one refusal. It is not a transient: neither an
interface held by something else nor a node this user may not open changes
while hotaru runs.

### And the Screen section stays open

A screen is a file. It travels to a machine that has a panel, and editing one
previews it in the window, so a cooler without a display is a reason to say so
at the top rather than a reason to close the section.

## Requirements

**R1. The panel found is reported**, in the window and at the terminal.

**R2. A panel that cannot be opened is an absence**: the scene still applies
and reports no problem.

**R3. The dashboard loop stops** rather than failing forever.

**R4. Screens can still be made** on a machine with nowhere to draw them, and
it says so once.

## Acceptance Criteria

- [x] AC1. System names the display, says "none" for a cooler without one,
      and says it is not reachable, with the reason, when it will not open.
- [x] AC2. A scene naming a picture applies with no problems reported on a
      machine whose panel refuses.
- [x] AC3. A refusing panel is pushed to once, and reported once.
- [x] AC4. The Screen section lists and offers to make screens with no cooler
      at all, above a note saying why nothing can be shown.
- [x] AC5. Verified on the development machine, which has a panel: `hotaru
      cooling` says `display   640x640 LCD`.

## Risks & Assumptions

- **The panel comes from the model table**, so a cooler added later needs its
  screen named there. A model with none is the honest default.
- **The loop stops until the service restarts.** A screen freed by another
  program in the meantime is not picked up; restarting is the recovery, and
  the alternative is retrying forever.
- **Not verified on a machine without a panel**, which is the case this is
  about. The tests cover it; the hardware check is somebody else's machine.
- **Rollback** is a revert. The API field is additive and an older window
  ignores it.
