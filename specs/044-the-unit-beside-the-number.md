# Spec 044: the unit beside the number

**Issue**: [#116](https://github.com/ushineko/hotaru/issues/116)

## Status: COMPLETE

## Context

Two changes to the panel, both about reading a number without having to work
out what it is.

### The unit belongs next to the number

Spec 041 took the unit line out from under the value and put it in the label,
which was right: it was a line saying something the author never chose, drawn
from the readings package and editable nowhere.

What it left is a label reading `CPU % · °C` above a value reading `12 · 63`,
and an eye that has to travel between them and pair the halves off in order.
That works and it is not reading; it is decoding.

So a dashboard can show units beside the numbers. `12% · 63°C`, one per half,
because a pair is exactly the case where the two halves are measured
differently -- CPU load with CPU temperature is the shape the feature was
wanted for.

**Small, and centred on the dot's line.** At the headline's 170 point a
full-size `%` is as large as the number it qualifies and reads as a second
number. Drawn at a fraction of the value's size and centred in the same band,
it sits on the line the separator sits on, which is where the eye already is.

**And it hugs its number**, which took a second attempt. The first laid units
into reserved boxes beside the values, the way the numbers are laid out, and
drew `59   %` and `63    °C` -- a blank column between a figure and its own
unit. A reservation exists to stop digits moving and a unit is not a digit.

So a centred pair is anchored on its separator: the dot sits at the middle of
the space and each half packs outward from it, reserving nothing. Each number
has a fixed edge against the divider, which is all the stability it ever
needed, and value and unit are drawn as one run. In the right-aligned rows the
numbers stay right-aligned in their column and the units are left-aligned in
theirs, so `%` and `RPM` start at the same place and the dot still lands in
one.

**And the generated label stops saying it.** A slot with no label of its own
reads `CPU` rather than `CPU °C` when units are on, because the unit is beside
the number and saying it twice is the redundancy spec 041 removed the unit
line for. An author's own label is never touched -- somebody who typed
`CPU % · °C` gets exactly that, and can delete it themselves.

Per dashboard rather than per slot. It is a decision about how this screen
reads, made once, the way `theme` and `lettering` are -- and a control on
every slot row is vertical space in an editor that has already fought for it
once (spec 038).

### The rows are a table and the dot is a column

Spec 041 set the stacked rows as a table: words hard left, numbers hard right,
one size for the column. It gave them one left edge and one right edge.

The dot is the third column, and it is ragged -- each row sizes its own first
number, so `2709 ·` and `12 ·` put their separators in different places. Four
rows of that is a table with a bend in it.

**Every field is as wide as the widest of that field across every row.** The
pump's four digits set the first column, so `12` is right-aligned in a
four-digit box and its dot lands under the pump's. One dot column, one right
edge, one size.

It costs the short rows some empty space to the left of their numbers, which
is what a table of numbers looks like and why right-alignment exists.

**A row with fewer fields aligns from the left.** A single-value row among
pairs puts its number in the first column, under the other rows' first
numbers, and leaves the rest of the line empty. Aligning it from the right
instead would put it under the *second* numbers, which is a lie about what it
is. That means such a row does not share the right edge, and that is the
honest cost: a screen mixing pairs and singles cannot have both edges.

## Requirements

**R1. A dashboard can show units beside its numbers**, off by default.

**R2. Each half of a pair carries its own unit.**

**R3. A unit is drawn smaller than the number** and centred on the same line.

**R4. The generated label drops the unit when units are shown**, and an
author's own label is left alone.

**R5. The stacked rows align every field**, so the dots land in one column.

**R6. A row with fewer fields aligns from the left.**

**R7. The editor offers the option**, and the preview redraws.

## Acceptance Criteria

- [x] AC1. With units off, every arrangement draws exactly what it drew
      before -- asserted against the shipped screen's golden frame.
- [x] AC2. With units on, a single value draws its unit and a pair draws one
      per half.
- [x] AC3. The unit is drawn at a smaller point size than the number beside
      it, and the two are centred on the same line.
- [x] AC4. A slot with no label of its own drops the unit from its label when
      units are on, and an author's own label is unchanged either way.
- [x] AC5. In a stacked dashboard whose rows are different widths, the dots
      are drawn in the same column, asserted on the rendered pixels.
- [x] AC6. A stacked row with one value aligns it with the other rows' first
      numbers.
- [x] AC7. Units survive the round trip through the API and the editor, and
      the editor's checkbox reaches the draft.
- [x] AC8. The golden frame is not regenerated: units default off, so the
      shipped screens are untouched and the frame still matches.
- [x] AC9. Verified on the development machine, on the panel.

## Alternatives Considered

- **Units per slot.** Rejected: a control on every slot row, in an editor that
  has already lost a card off the bottom of a window once.
- **One unit at the end of a pair.** Rejected: it is wrong for the case pairs
  exist for, and nothing would say what the first number is measured in.
- **Aligning the dots only where the reserved widths fit.** Rejected: a screen
  whose dots sometimes line up and sometimes do not reads as a fault.

## Risks & Assumptions

- **Units cost width, and the column is already fitted.** Reserving a unit
  beside every number makes the assembly wider, so the shrink from spec 042
  engages sooner and a stacked column of pairs with units may come out
  noticeably smaller. Measured on the panel before accepting; if it is bad,
  the honest answer is that units and pairs together do not fit a stacked row
  and the option is for the arrangements that have room.

- **The unit's size is a guess.** A fraction of the value's, picked by eye and
  named as a constant. It is the sort of number that wants looking at on the
  panel rather than reasoning about.

- **Aligning every field shrinks the column further.** The first box is now
  the widest first number rather than each row's own, so the assembly is wider
  and fits at a smaller size. That is the cost of the dots lining up and it is
  the trade being bought.

- **A screen mixing pairs and singles loses its right edge.** Stated above and
  accepted; the alternative misrepresents which number a lone value is.

- **Rollback** is a revert. `units` defaults to off, so a dashboard saved
  without it is unchanged and one saved with it loads in the previous version
  with the key ignored.
