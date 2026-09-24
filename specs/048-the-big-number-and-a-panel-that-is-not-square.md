# Spec 048: the big number, and a panel that is not square

**Issue**: [#144](https://github.com/ushineko/hotaru/issues/144)

## Status: COMPLETE

## Context

Two faults found together, both about the size of the headline.

### The big number had no size of its own

Spec 037 gave a dashboard two kinds of text -- the words and the numbers --
and one size each. It says why: "the headline being three times its label is a
relationship arrived at by looking at a panel in a case, and it is worth
keeping when somebody makes everything bigger."

That is a good default and it is not everybody's panel. A desk that wants the
four rows readable wants the headline smaller, and there was no way to say so:
one slider moved both.

**And on a paired headline that slider did nothing at all upward.** Measured
on rendered frames, stacked, units on:

| Readings | paired headline | single value |
|---|---|---|
| 50% | 43 px | 43 px |
| 100% | 79 px | 86 px |
| 125% | 79 px | 108 px |
| 150% | 79 px | 115 px |

A pair with units reserves two runs, two gaps and a separator, which at the
arrangement's own size already fills the room. Everything above 100% was
fitted straight back to what fits. The clamp is the panel's real limit and
spec 047's fit made it stricter, so the slider was honest and inert at once.

So the headline takes its own lettering, over the readings'. **Unset is the
readings'**, which is what every dashboard saved before this says, and what
spec 037 chose. Shrinking always fits, which is the half that works on a pair.

### The frame is square and the panel is not

The 640x640 GIF is displayed through a round bezel. The corners are not shown
at all, and -- this is the part nothing accounted for -- **a band high up the
panel is narrower than one across its middle**.

At y=98, where a stacked headline's digits begin, the circle inscribed in the
frame allows 461 pixels of width. The rectangular inset allowed 576. The fit
was working with a hundred and fifteen pixels the panel cannot draw:

| headline | inked pixels outside the circle |
|---|---|
| `48% · 38°C` | 0 |
| `48% · 100°C` | 127, the worst 17 pixels past it |

Which is why this came and went with the temperature, and why it survived
spec 047: that fix kept the assembly inside the *square*, and nothing was
checking the circle.

The headline is therefore fitted to the width its own band has inside the
circle, taken at the row furthest from the centre. Never wider than the inset,
which somebody chose by looking at a panel in a case; this only takes room
away.

**It costs size.** A paired stacked headline goes from 79 pixels of digit
height to 66. That is the size that was always visible; the rest was behind
the bezel.

## Requirements

**R1. The headline has its own size**, and unset is the readings'.

**R2. The fallback is per field.** Somebody who sets a size has said nothing
about a colour.

**R3. Sizing the headline moves nothing else.**

**R4. Nothing is drawn where the bezel covers it**: no inked pixel outside the
circle inscribed in the frame, in any arrangement, for any reading.

**R5. The headline is never given more room than the inset**, which is a
decision made by looking at a panel.

**R6. The editor offers the size**, and says when a headline is already as
large as the panel takes.

## Acceptance Criteria

- [x] AC1. A headline size of its own draws a smaller headline, and the rows
      under it are pixel-identical.
- [x] AC2. An unset headline draws exactly what the readings' lettering says,
      field by field.
- [x] AC3. No arrangement draws ink outside the circle, for a pair of
      three-digit readings and for the widest pair this machine can report.
- [x] AC4. The editor has a size for the big number, it reaches the draft, and
      it does not write the readings' size.
- [x] AC5. The shipped screen's golden frame is regenerated deliberately: the
      ring headline's room falls from 576 to 565, which moves it by a pixel.
- [x] AC6. Verified on the development machine, on the panel.

## Alternatives Considered

- **A full third block: size, colour and edge.** Rejected for now: three more
  controls in a form that has already lost a card off the bottom of a window,
  for two settings nobody asked for. The fallback is written per field, so
  adding them later is a form change and not a format one.
- **Clipping the text to the circle.** Rejected: a number with its right-hand
  digit shaved off is worse than a smaller number, and it would hide the fault
  rather than fix it.
- **Narrowing the inset instead.** Rejected: the inset is a constant and the
  bezel is a curve, so any constant that fits the headline's band wastes room
  everywhere else.

## Risks & Assumptions

- **Every paired headline gets smaller**, by about a sixth on a stacked
  screen. Nothing is lost -- that part was never visible -- but it is a
  visible change on somebody's panel, and the editor's new slider is the way
  back up if they prefer the old size and the clipping with it.

- **The circle is assumed to be the one inscribed in the frame.** The bezel
  may cover a little more than that. If it does, this is the floor of the
  correction rather than the whole of it, and the same function is where a
  measured radius would go.

- **Rollback** is a revert. The new field is absent from every saved
  dashboard, and absent means what it drew before.
