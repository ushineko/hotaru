# Spec 049: a pair that does not move

**Issue**: [#144](https://github.com/ushineko/hotaru/issues/144)

## Status: INCOMPLETE

## Context

`8% · 52°C` and `100% · 100°C` put their separator, their units and their
digits in different columns. A panel somebody glances at is a panel whose
numbers are where they were, which is what spec 042 is called.

### What spec 044 traded away, and why it has to come back

Spec 042 gave every value a reservation: a number is drawn in a box as wide as
the characters it can take, so the digits sit in the same columns from one
frame to the next and the assembly's width never depends on the reading.

Spec 044 then took the reservation out of one case. A pair centred on its
separator packs each half outward from the middle, measuring what it draws:

> Nothing here reserves anything, and nothing needs to: an edge that does not
> move is an edge that does not move, and each number has one against the
> divider.

That is true of the *inner* edge and says nothing about the rest. `8` becomes
`100` and the left half grows two digits leftward; `52` becomes `100` and the
right half grows one digit rightward, taking its unit with it. Every glyph but
the separator moves.

The reason for dropping the reservation was real: boxes left a blank column
between a figure and its own unit, because the number was right-aligned in its
box and the unit began at the far side of it. That is a fault of where the
slack was put, not of reserving.

### The slack goes where nothing is drawn

**A number is held against the right of its reservation**, and the unit
follows it. So the last digit and the unit after it stay in their columns, and
the empty part of the reservation falls where there is nothing to see:

- On the left half, outside the pair, where the panel is empty anyway.
- On the right half, between the separator and the figure.

The separator stays where it is, which is spec 044's fixed point. The unit
still hugs its number, which is spec 044's other one. And the digits stop
moving, which is spec 042.

The gap after the separator widens as a reading loses digits. That is the
honest cost and it is the quiet one: empty space changing size is not a number
changing place.

## Requirements

**R1. A pair takes the room it reserved**, not the room it measures.

**R2. The separator does not move** when a reading gains a digit.

**R3. The units do not move** when a reading gains a digit.

**R4. A unit still begins where its number ends.**

## Acceptance Criteria

- [x] AC1. Across `8% · 52°C`, `48% · 52°C`, `100% · 52°C`, `8% · 100°C` and
      `100% · 100°C`, the separator and both units are drawn in identical
      columns, asserted on the rendered pixels.
- [x] AC2. The glyphs the two extremes share are in the same columns, counting
      from the separator outward.
- [x] AC3. A unit's reservation is its gap and its text, so the rows and the
      pair lay out the same boxes.
- [x] AC4. The shipped screen is unchanged: its headline is a single value and
      reserves what it always did.
- [ ] AC5. Verified on the development machine, on the panel.

## Alternatives Considered

- **Left-aligning the number in its reservation.** It keeps the gap after the
  separator constant and makes the unit move instead, which is the thing being
  fixed.
- **Reserving only on the outer side.** The same as this for the left half and
  no different from measuring on the right, so it fixes half the fault.
- **Padding the text with spaces.** The font's digits are tabular but its
  space is not a digit, and a number drawn as `" 52"` is a number whose
  alignment depends on the face.

## Risks & Assumptions

- **The gap after the separator is wider than it was** for a two-digit
  reading, because two digits of reservation sit there empty. It is the same
  arrangement the stacked rows have had since spec 044 and nobody has called
  it wrong there.

- **Rollback** is a revert. Nothing is written to disk and no file format
  changes.
