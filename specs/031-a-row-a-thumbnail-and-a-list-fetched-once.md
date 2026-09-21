# Spec 031: a row, a thumbnail, and a list fetched once

**Issue**: [#72](https://github.com/ushineko/hotaru/issues/72)

## Status: COMPLETE

## Context

Three faults in the lists this window draws, reported one after another while
using it.

### The buttons were at the window's edge

A row built as `Border(nil, nil, columns, actions, nil)` pins its buttons to
the right-hand edge however wide the window is. With twenty rows on screen the
eye has to track from the end of a line, across a hand's width of nothing, to
the button belonging to it -- and which button belongs to which line stops
being obvious exactly when there are enough lines for it to matter.

Every column is a fixed width now and the actions follow the last one, so the
columns line up down the list, the buttons form a band beside them, and the
empty space is on the outside where nobody has to cross it. Measured at a wide
window: four pixels from the content against 1,471.

The same shape put the scene's facts in line, which meant saying `screen:
nebula` rather than the forty characters of a path that are identical on every
row.

### And the rows did not line up with each other

The key is the first thing on a scene's row and `Shift+1` is wider than `6`,
so every shipped scene's columns started nine pixels left of every bound
scene's, and the offset carried all the way along to the buttons. The key
holds a width like the other columns now.

### The screen chooser asked the service once per option

It fetched the dashboards and the pictures to build its list, and then, for
every line in it, called both routes *again* to find the one thumbnail that
line needed. Eighteen options meant thirty round trips over the socket on the
thread drawing the window, and it grew with the number of pictures somebody
had kept.

The list is fetched once and kept between openings, dropped and re-warmed off
the UI thread when the navigation arrives at Scenes -- so a picture added in
the Pictures tab is in the list by the time the Scenes tab is on screen, which
is the only moment it can have changed.

### And the pictures drifted out of line with the options

Fyne's radio group draws text and nothing else, so a thumbnail can only sit
beside its option. A VBox puts padding between its children and the group puts
none between its options: by the tenth option the picture was beside the wrong
name. The column is laid out at the group's own pitch -- its height over the
number of options -- which also sizes the thumbnails.

## Requirements

**R1. A row's buttons sit beside its content**, not at the window's edge.

**R2. Every row's columns start in the same place**, whatever the row says.

**R3. The chooser's list is fetched once**, not once per option, and is
refreshed when the section is arrived at.

**R4. A thumbnail is level with the option it belongs to.**

## Acceptance Criteria

- [x] AC1. A row's button is nearer the content than a bordered row's is, by
      a factor of four or more at a wide window.
- [x] AC2. Two rows whose keys differ in width put their buttons in the same
      place.
- [x] AC3. Opening the chooser with twelve pictures asks for the pictures at
      most once.
- [x] AC4. The thumbnail column's pitch equals the radio group's.
- [x] AC5. Verified on the development machine.

## Risks & Assumptions

- **Fixed column widths are this desk's widths.** A name longer than the
  column is truncated rather than allowed to move everything after it.
- **The kept list is refreshed on arrival**, so a picture added by another
  program while the Scenes tab is on screen is not offered until somebody
  navigates. The alternative is a fetch per opening, which is what this
  replaced.
- **Rollback** is a revert; nothing is stored.
