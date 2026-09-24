# Spec 047: a scene shows what it draws

**Issue**: [#126](https://github.com/ushineko/hotaru/issues/126)

## Status: INCOMPLETE

## Context

A scene is two things at once on this machine: what the lights show, and what
the panel shows. The list says the first in colour and the second in words.

`screen: berserk-slide` is a name somebody has to remember the look of, beside
a swatch that shows a colour without asking them to remember anything. The
picture is the look, and there is room for it on the line.

### Where it goes, and how big

Beside the swatch, aligned with it, in the same column pair on every row.
Thirty-two pixels square: larger than the eighteen-pixel colour, because a
colour is one fact and a picture is a photograph somebody has to recognise,
and smaller than the buttons on the same line, so that no row grows.

The pictures are the ones the Pictures and Screen sections already draw,
through the same cache and the same fetch the screen chooser warms on arrival.
A thumbnail on a scene's line therefore costs nothing that opening the chooser
does not already cost.

### When there is nothing to show

Four cases, and each one is a row that would otherwise carry a picture of
something that is not there:

- **The scene says nothing about the screen**, or asks for the cooler's own
  readout. Neither is a picture hotaru drew.
- **It names a picture or a dashboard that is gone.** Deleting either leaves
  the scene naming it, which is already how the list reads.
- **This machine has no panel**, or cannot open the one it has. A scene is
  still worth having here -- it is a file, and it travels to a machine with a
  panel -- but a thumbnail of what it would show there is a promise this desk
  cannot keep. The Screen section says the same thing in its own words.
- **The list of screens has not arrived**, which is the first build after a
  cold start.

`dashboard` on its own means whichever dashboard is set, so the picture is of
that one. It follows the setting rather than the scene, which is what the
words beside it already say.

## Requirements

**R1. A scene's row shows a thumbnail** of the picture or dashboard it puts on
the panel, beside the colour swatch.

**R2. Nothing is drawn** when the scene puts nothing on the panel, when what
it names is gone, or when the machine has no panel to draw on.

**R3. No row grows.** The thumbnail is smaller than the buttons already on the
line.

**R4. It costs no fetch of its own**, using the list and the cache the screen
chooser already warms.

## Acceptance Criteria

- [x] AC1. A scene naming a dashboard, a scene saying `dashboard`, and a
      scene naming a stored picture each draw a thumbnail on their row.
- [x] AC2. A scene with no screen, one asking for the readout, one naming a
      deleted picture and one naming a deleted dashboard draw none.
- [x] AC3. A machine with no cooler, a cooler with no display, and a display
      that cannot be opened all draw none.
- [x] AC4. The thumbnail is drawn at the scene shot's size rather than the
      Pictures section's, so the row is unchanged in height.
- [ ] AC5. Verified on the development machine, in the window.

## Alternatives Considered

- **A thumbnail in place of the words.** Rejected: the words say which
  dashboard, and two dashboards on the same background are two similar
  squares at this size.
- **The Pictures section's own thumbnail size.** Rejected: 96 pixels is taller
  than the row, and every line in the list would grow to hold it.

## Risks & Assumptions

- **A picture the library lists and the disk does not have** draws "(cannot
  read it)" in its square, which is the Pictures section's own behaviour
  through the shared helper. In a 32-pixel square that text is clipped. It is
  a state somebody reaches by deleting a file behind hotaru's back.

- **The `dashboard` thumbnail follows the setting**, so two scenes that both
  say `dashboard` show the same picture, and it changes when the active
  dashboard does. That is what the scene does, and the words beside it say so.

- **Rollback** is a revert. Nothing is written to disk and no file format
  changes.
