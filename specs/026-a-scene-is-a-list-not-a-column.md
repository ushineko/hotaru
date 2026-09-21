# Spec 026: a scene is a list, not a column

**Issue**: [#59](https://github.com/ushineko/hotaru/issues/59)

## Status: COMPLETE

## Context

Two rounds in, resizing the scene editor still paused. What was left of it was
the scene's own lines.

	9.16s of 25s samples
	6.64s  72%  scrollContainerRenderer.Layout
	2.88s  31%  Container.MinSize
	1.44s  16%  Button.MinSize
	1.19s  13%  Label.MinSize

A scene that names lights individually has 225 assignments -- the development
machine has one, built from a photograph. Each line was:

	VBox
	 |- Label (wrapping)
	 `- HBox
	     |- canvas.Rectangle
	     |- Label (the colour)
	     |- Button "Change"
	     `- Button (remove)

Seven objects a line, about 1,575 in a column inside a scroller. Fyne measures
every widget in a container on every layout, and a wrapping label measures
itself against whatever width it is given -- so a drag re-measured all of them,
repeatedly, to place rows whose height could not change.

`widget.List` builds the rows that are on screen and recycles them. A 225-line
scene now costs what a ten-line scene costs.

### What changed on screen

The name truncates rather than wraps, and the colour moved into the label
beside it. Wrapping was there because "a name, a swatch, a colour and two
buttons in one row needs more width than a pane has" -- which is true, and the
answer that a list can give is that the row is a fixed height and the name
ends in an ellipsis. A device on this desk is called `NZXT Kraken 2024 ELITE
Series RGB/Hue 2 Channel 1[12:23]`, which no pane was ever going to fit.

The controls under the list are affixed rather than scrolling with it, which
is fynedesygn's rule for what acts on a page and was not being followed here.

### What it bought

Same editor, same 225-line scene, same drag:

| | original | swatches (spec 025) | this |
|---|---|---|---|
| samples in 25 s | 18.84 s (75% of a core) | 9.16 s (37%) | **6.10 s (24%)** |
| `scrollContainerRenderer.Layout` | 14.33 s | 6.64 s | 3.13 s |
| `Container.MinSize` | -- | 2.88 s | not in the top ten |
| `Button.MinSize` | 2.69 s | 1.44 s | not in the top ten |

Nothing of hotaru's is in the top ten. What is left is `runtime.cgocall` at
53% -- GL and GLFW -- and harfbuzz shaping text, which is Fyne drawing.

Confirmed by the person resizing it: "pretty good now, maybe very small
hitches but you really have to throw the window around".

## Requirements

**R1. The scene's lines are virtualised.** Only the rows on screen exist.

**R2. A line still does what it did**: shows the target, its colour and a
swatch, changes that colour where it is shown, and removes it.

**R3. The controls under the list do not scroll with it.**

**R4. The measurement is a profile**, before and after, same machine, same
scene, same drag.

## Acceptance Criteria

- [x] AC1. The assignments are a `widget.List` and the rows are built by a
      template filled per index.
- [x] AC2. The editor's tests pass, including the one that reads what a line
      says -- which now gives the section a size, because a list with no size
      has no rows.
- [x] AC3. A profile of the same drag shows the cost down from 9.16 s to
      6.10 s, with the numbers recorded here.
- [x] AC4. Verified on the development machine by somebody resizing it.

## Risks & Assumptions

- **A recycled row must be fully re-pointed.** Every field a row shows, and
  both button callbacks, are set in `fill`; one that was set at build time
  would act on whichever assignment happened to build it. That is the
  characteristic bug of this pattern and the reason `entry` takes no
  arguments -- it cannot capture what it must not.
- **The name is truncated**, so the pane no longer shows the whole of a long
  target. The tip on a light block still names it in full, and the colour is
  beside the name rather than under it.
- **Rollback** is a revert to the column.

## Alternatives Considered

Considered keeping the column and making each row cheaper -- one button
instead of two, no wrapping; rejected because 225 lines times even four
objects is 900 measurements that a list does not make at all.
