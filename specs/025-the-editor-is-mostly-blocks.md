# Spec 025: the editor is mostly blocks

**Issue**: [#57](https://github.com/ushineko/hotaru/issues/57)

## Status: COMPLETE

## Context

With spec 024's thread check gone, the scene editor became the window's worst
case. A 25-second profile of it being resized, a large scene loaded:

	18.84s of 25s samples   75% of a core
	14.33s   76%   scrollContainerRenderer.Layout
	 5.36s   28%   runtime.mapaccess2
	 2.69s   14%   buttonRenderer.MinSize

Nothing in it is one slow function. It is breadth. The editor draws a block
per run of lights -- up to twenty-four per zone, across six devices with 1, 48,
65, 8, 3 and 100 lights -- and each block was five objects:

	Stack (tooltip)
	 |- Stack
	 |   |- canvas.Rectangle
	 |   `- widget.Button      <- invisible, catching the tap
	 `- tipArea

A scroller lays out everything it holds rather than what is visible, so every
drag step measured every block on the page; and a button measures itself with
a theme lookup, a padding calculation and a `RichText.MinSize` for a label
that is the empty string.

That is the right amount of work for a button and the wrong amount for a
coloured square.

### The fix belongs upstream

"An invisible control stacked over a drawing" is how anybody builds a palette
out of what Fyne provides, so `widgets.Swatch` went into fynedesygn (its spec
024) rather than here: one widget whose minimum size is the number it was
given. This spec is the consumer, and the measurement.

### What it bought

Same editor, same scene, same drag:

| | before | after |
|---|---|---|
| samples in 25 s | 18.84 s (75% of a core) | 9.16 s (37%) |
| `scrollContainerRenderer.Layout` | 14.33 s | 6.64 s |
| `runtime.mapaccess2` | 5.36 s | 1.99 s |
| `buttonRenderer.MinSize` | 2.69 s | 1.10 s |

Halved. The swatch itself does not appear in the profile at all: its
`MinSize` is a field read and the compiler inlines it away.

The `buttonRenderer.MinSize` that remains is the device and zone buttons,
which have labels and are buttons on purpose.

### What is still there

`scrollContainerRenderer.Layout` is still 72% of what is left. A scroller
lays out its whole content, on screen or not, and the answer to that is
virtualisation of the kind `markdown.Pane` already does for a document --
which is a larger piece of work and not this spec's.

## Requirements

**R1. A light block is one widget.** Not a drawing with a control stacked on
it.

**R2. Nothing about it changes on screen.** Same size, same colours, same
selection stroke, same tooltip, same toggle.

**R3. The measurement is a profile**, before and after, same machine, same
thing being done.

## Acceptance Criteria

- [x] AC1. The blocks are `widgets.Swatch`, and no `widget.Button` is created
      per light.
- [x] AC2. The editor's tests pass unchanged: picking, toggling and the
      range grammar are what they were.
- [x] AC3. A profile of the same drag shows the cost at least halved, with
      the numbers recorded here.
- [x] AC4. Verified on the development machine, by somebody resizing it.

## Risks & Assumptions

- **A swatch has no keyboard path.** Neither did the invisible button in
  practice -- it had no label, so a screen reader had nothing to say and tab
  focus landed on an unlabelled control. The tooltip remains the thing that
  says what a block is. A keyboard route into the editor would be its own
  piece of work.
- **The remaining 72% is Fyne's scroller**, and a fix for it is a change to
  how the editor is built rather than to what it is built from.
- **Rollback** is a revert; the widget it depends on is additive in
  fynedesygn.

## Alternatives Considered

Considered drawing the whole LED grid as a single raster and hit-testing taps
by coordinate, which is one object per zone rather than one per run; rejected
for now because it gives up the tooltip per block and the selection stroke,
and the swatch was enough to halve it.
