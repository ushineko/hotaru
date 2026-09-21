# Spec 018: pointing at a fan

**Issue**: [#5](https://github.com/ushineko/hotaru/issues/5)

## Status: INCOMPLETE

## Context

`kraken/ring[12:23]` is a thing somebody can type. It is also a thing they have
to **work out first**, by lighting LEDs and looking at the case, and the only
tool for that so far asks questions in a terminal and writes down the answers.

This is the editor: a device drawn, a zone clicked, a colour chosen, the
hardware showing it a second later, and a scene saved with a name. The window
from spec 017 already draws the machine and is already known to show the truth
about it; this makes the picture answer back.

### Three states, not two

A scene being edited is a **draft**: it lives in the window and nothing has been
written anywhere. Pushing it to the hardware is a **preview**: the devices show
it and it is still not what anybody wants, it is what they are looking at.
**Saving** is what makes it a scene.

The distinction is what makes a colour picker usable at all. Without it, a
dragged slider is sixty writes a second and sixty recorded intentions; with it,
the draft moves freely and the hardware is written when somebody asks.

### The service cannot preview a draft yet

`POST /v1/scenes/{name}/apply` previews a scene **that has been saved**, which
a draft by definition has not. Saving a scratch scene to preview it would put a
half-finished thing in somebody's scene list and make "saved" mean nothing.

So the preview route takes a scene inline: `POST /v1/preview` with the scene in
the body, answering with the same lease the named form does. The lease
machinery is unchanged -- it already ends with its holder, already suspends
re-assertion for the devices it covers, and already reverts.

### The lease is the connection the window holds

Spec 015 built two ways to hold a preview: an open request, or a renewed
expiry. The window uses the first, because it can: a goroutine sits on the
request, and the window closing, crashing or being killed drops the socket and
the service puts the lights back without anybody noticing it had to.

A preview is therefore visible as a preview in exactly one place -- the window
says the hardware is showing a draft and offers the way back -- and true
whatever happens to the window.

### What the editor must not lose

A scene carries colours, an effect per device, and a screen state. This editor
edits colours. **It must carry the rest through untouched**: editing the colour
of a scene that puts the keyboard in Solid Splash and a GIF on the panel, and
saving it, must not quietly drop either. Round-tripping is the whole risk of an
editor that understands part of a format.

## Requirements

**R1. A Scenes section.** Every scene, what it does, and the two things worth
doing to one: apply it, or look at it.

**R2. An editor over a draft.** Pick a device or a zone by clicking the
picture, give it a colour, see the draft update. Nothing is written.

**R3. Preview is explicit, and holds a lease.** A draft reaches the hardware
only when asked. The window holds the lease on an open connection, so closing
or killing the window restores the lights without the service being told.

**R4. A preview is visible as one**, with the way back beside it. Somebody who
closes the editor and later wonders why the mouse is green has been handed a
puzzle by the program.

**R5. Saving names it.** A draft becomes a scene when it is given a name, and
an existing name replaces that scene.

**R6. An edited scene keeps what the editor does not edit** -- effects, the
screen state -- byte for byte.

**R7. The service gains one route.** `POST /v1/preview`, carrying a scene
rather than naming one, answering with a lease. The CLI reaches it too, because
parity is symmetrical.

**R8. The editor writes nothing to disk.** Scenes are saved through the
service, which owns that file. The window's own file keeps view state and
nothing else.

## Acceptance Criteria

- [ ] AC1. The Scenes section lists every scene, marks the shipped ones, and
      applies one.
- [ ] AC2. A device or zone is selected by clicking it in the editor, and the
      selection is visible.
- [ ] AC3. Setting a colour changes the draft and writes nothing.
- [ ] AC4. Previewing sends the draft and holds a lease; the devices it covers
      report the preview through the API.
- [ ] AC5. Closing the window ends the preview and the lights go back, with no
      release call made.
- [ ] AC6. The editor shows that a preview is up and offers to end it.
- [ ] AC7. Saving a draft creates a scene the CLI can apply.
- [ ] AC8. Editing a scene that carries an effect and a screen state and saving
      it preserves both.
- [ ] AC9. `POST /v1/preview` takes a scene and returns a lease, with a test
      that a draft never saved can be previewed and reverted.
- [ ] AC10. The CLI can preview an unsaved scene, so the route is not
      GUI-only.
- [ ] AC11. Tests run headless, with no display and no hardware.
- [ ] AC12. Verified on the development machine: a fan clicked, a colour
      chosen, the hardware showing it, the scene saved, and `hotaru scene
      apply` lighting it afterwards.

## Risks & Assumptions

- **A draft on the hardware is state somebody can forget about.** The lease is
  the answer and it is already built; this spec's job is to use it rather than
  to invent a second mechanism.
- **Clicking a zone is not clicking an LED.** Selecting a range within a zone
  is what the segment-naming workflow needs and it is not in this spec; a zone
  is the unit here, and the finer selection follows.
- **Two clients could edit at once.** The lease refuses the second: one device,
  one preview, and the second caller is told who holds it.
- **Rollback** is not shipping the section; the route is additive and the CLI
  is unaffected.

## Alternatives Considered

Considered previewing by saving a scratch scene; rejected because it puts a
half-finished thing in somebody's scene list and makes "saved" stop meaning
anything.

Considered making the colour picker write continuously and relying on
coalescing; rejected for the reason staging exists -- the hardware would be
written sixty times a second, and the last frame of a drag would be recorded as
an intention.
