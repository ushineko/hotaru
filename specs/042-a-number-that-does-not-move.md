# Spec 042: a number that does not move

**Issue**: [#106](https://github.com/ushineko/hotaru/issues/106)

## Status: COMPLETE

## Context

Two things, found by watching the panel rather than by reading the code.

### The headline wobbles

The value is drawn as one string, centred. So when the character count
changes -- `9` to `10`, `99` to `100`, and on a pair whenever either half
does -- every character moves. The panel refreshes every couple of seconds,
and what that reads as is the number shifting about rather than updating.

**The Go fonts are tabular.** Every digit is exactly the same width, in all
three families the dashboard offers: 88 pixels at the headline's 118 point in
the sans face, 94 in mono. So nothing moves because of *which* digits are
drawn. It moves only because of how many.

That is the whole fix. Reserve each number a field as wide as the most
characters it can take, place the fields, and draw each number right-aligned
in its own. The digits stop moving because the field does not, and the
assembly stays put because its total width no longer depends on the reading.

**Padding the string with spaces does not work**, which is the obvious answer
and worth writing down as tried. A space is 44 pixels against a digit's 88 in
the sans face -- exactly half -- so a number padded to a fixed character count
lands half a digit off whenever the padding is odd, and the digits sit between
columns rather than on them. The fields are pixels, not characters.

A field is a **minimum**, not a limit. A coolant that reads 100.0 is a machine
in trouble and it gets the width it needs; what it costs is the one shift the
rest of the range no longer has.

### Two things the panel said

Both found by rendering it and looking, which is the only way either would
have been found.

**A paired headline filled the panel.** Reserving each half three characters
makes the assembly 945 pixels before it is fitted, so it was squeezed to 639
of 640 and touched both edges. The headline gets a 32-pixel inset now. A
single value is unaffected: `37.5` at 170 point is 508 pixels and was already
well inside.

**The separator ended up stranded.** Right-aligning both halves leaves a blank
character on each side of it, and a dot floating in that much space stops
reading as a divider between two numbers and starts reading as a third thing.
So a centred pair hugs its separator -- first half right-aligned, second half
left-aligned -- and the slack goes to the outer edges, where a centred
assembly has it symmetrically and nobody sees it. The numbers grow outward
from the divider, which is also what the eye expects them to do.

A right-aligned column keeps every field right-aligned. What matters there is
that the numbers end where the ones above and below them end, which is spec
041's table, and hugging would make the outer edges ragged again.

### The separator

` · ` rather than ` / `, as the default. The slash was the first guess and the
middle dot is what somebody chose after looking at it on the panel, which is
the only place this question can be answered.

The shipped dashboards' labels follow it: `CPU % · °C` rather than
`CPU % / °C`, because a label whose divider disagrees with the number beneath
it is worse than either divider.

An author who set ` / ` keeps it. Only the ones that never said anything
change, which is what a default means.

## Requirements

**R1. Each number is drawn in a field reserved at its widest**, so the digits
do not move as the reading changes.

**R2. A field is a minimum**: a reading wider than its field is drawn whole.

**R3. The assembly does not move** when a reading changes character count.

**R4. Every arrangement gets it** -- the headline, the columns and the rows.

**R5. The reserved width belongs to the readings package**, beside the format
that decides it.

**R6. ` · ` is the default separator**, and the shipped labels agree with it.

**R7. The headline keeps clear of the panel's edges.**

**R8. A centred pair hugs its separator**; a right-aligned column does not.

## Acceptance Criteria

- [x] AC1. Rendering the same dashboard at `9`, `10` and `100` draws the
      digits at the same pixel columns, for a single value and for a pair.
- [x] AC2. A reading wider than its reserved field is drawn in full rather
      than clipped.
- [x] AC3. A pair whose left half changes width does not move its right half.
- [x] AC4. The headline, a Ring column and a Stacked row all hold still.
- [x] AC5. `readings.Width` gives every source a width, and it is at least as
      wide as that source's own formatting of a plausible reading.
- [x] AC6. A slot with no separator draws ` · `; one that set ` / ` keeps it.
- [x] AC7. The shipped labels use ` · `.
- [x] AC8. The golden frame is regenerated and the shipped screen held to it.
- [x] AC9. Verified on the development machine, on the panel, watching a
      reading cross a digit boundary.
- [x] AC10. A paired headline does not touch the panel's edges, and a single
      value is drawn at the size it was before.
- [x] AC11. A centred pair's separator does not move when either half changes
      width, and the half that did not change is drawn in exactly the same
      columns.
- [x] AC12. A right-aligned column still ends every row at the same place.
- [x] AC13. Nothing in the package still measures a value as a string: the
      helpers spec 041 added for that are gone, and the linter agrees.

## Risks & Assumptions

- **Tabular digits are assumed and asserted.** The fix rests on every digit
  being the same width; a font that changed that would bring the wobble back
  in a way nobody would think to look for. A test measures all ten digits in
  all three families rather than trusting the claim.

- **The reserved widths are judgements.** A coolant gets four characters
  (`37.5`), temperatures and percentages three (`100`), RPM four, gigabytes
  three. Each is the widest plausible reading rather than the widest possible
  one, and being wrong costs a single shift at the extreme rather than a
  broken layout.

- **Fields cost horizontal room.** A headline reserving `100.0` is wider than
  one drawing `37.5`, so the value sits in a larger box and the shrink-to-fit
  from spec 041 engages sooner. Measured on the panel before accepting.

- **The default separator changes saved dashboards** that never set one. The
  changelog says so. An author who set a separator is untouched.

- **A paired headline is drawn smaller than a single one**, because it is
  sized for two reserved fields and a separator. That is the cost of a number
  that does not move, and it was looked at on the panel before being accepted.

- **Rollback** is a revert. No file format change.
