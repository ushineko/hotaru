# Spec 041: two readings in one slot

**Issue**: [#102](https://github.com/ushineko/hotaru/issues/102)

## Status: COMPLETE

## Context

A dashboard slot has said one thing since spec 013: a source, a label above it
and a unit below it. Three lines to carry one number.

Two of those three are worth reconsidering together.

### The pair

CPU load and CPU temperature are one thought. So are memory used and memory
free, pump RPM and pump duty, GPU load and GPU temperature. Splitting them
across two slots spends twice the room to say a thing that reads better as
`11 / 59` under a label that says `CPU % / C`.

So a slot takes an optional second source. The value becomes the two readings
joined by a separator; the label says what they are. Both halves of the label
are the author's words, which is the point -- `%` and `C` are what *this*
author calls them, and the next one may want `pct` and `deg`.

Every slot, not only the headline. A rule that applies to the big number and
not to the four under it is a rule somebody has to remember, and the editor
would need two row shapes to express it.

### The unit line

It goes, everywhere.

It was already the odd one out. The label is the dashboard's own word, taken
from the author and falling back to the reading's; the unit was **only** ever
the reading's, because the editor offers no way to change it. `Slot.Unit`
existed in the model, the API type and the file format, and the only thing
that ever wrote it was a hand-edited file.

With the label free to say `CPU % / C` there is nothing left for it to do that
the author cannot do better. So the line is not drawn, `Slot.Unit` goes, and
the default label carries the unit instead: a slot whose author has said
nothing now reads `CPU °C` rather than `CPU` with `°C` beneath it. Nothing is
lost by never touching the editor.

### Which one grades

A dual value draws in the colour of its **first** source, and a headline ring
that tracks the headline grades on the first source too.

Whichever-is-worse was the other candidate and is wrong on this screen: the
number goes red for a reason that is not visible in it, and `11 / 59` gives no
clue which half went hot. The author chooses the order, so the author chooses
what the colour means.

### The rows are a table, so they are set like one

Found by looking at the panel with the pairs on it. The stacked rows put the
label and the value centred in their own bands, which is invisible on one row
and ragged down four: "2709 / 90" is fifty pixels wider than "12 / 63", so
every row started and ended somewhere different. A table has two edges, not
eight.

Words hard left, numbers hard right. Two things went wrong on the way there
and both are worth writing down, because each looked like the finished
article:

- **A fixed label band collides once it is justified.** "PUMP RPM / %" is 208
  pixels in a 160-pixel band. Centred it overflowed both ways and nobody
  noticed; hard left it overflows one way, into the number, leaving three
  pixels. So the words column is as wide as the *widest* label and the
  numbers take what is left, with a gap that cannot be eaten.

- **Per-row shrinking is worse than the rag it replaced.** Letting each row
  fit its own value put 34, 34, 34, 30 and 31 point down one column. So the
  column is measured once: the size is the one the widest number needs, and
  every row is drawn at it. It costs the short rows four points and buys a
  column that reads as one thing.

Stacked only. A Ring or Grid column is a label with one value under it -- its
own unit, with nothing beside it to line up with -- so those stay centred.

### It has to fit

`centred` draws at the point size it is given and lets the string overflow its
box. That is already a hazard -- `column`'s own comment records a four-digit
pump reading colliding with its neighbour, fixed at the time by making the
font smaller for everybody. A pair makes it certain: `1450 / 1450` in a
Stacked row's 200-pixel value band overruns at any size that row would want.

So a value that does not fit its width is drawn smaller, stepping down to a
floor. Measured, not guessed: `font.MeasureString` already runs on every draw
to centre the string, so the width is in hand before the first pixel.

Labels are left alone. A label that is too long is the author's sentence and
theirs to shorten; a value is the machine's and cannot be edited.

### Memory

`/proc/meminfo`, the way utilisation is `/proc/stat`. `MemTotal` and
`MemAvailable`, which is the kernel's own estimate of what a workload could
have without swapping -- the number `free -h` calls "available" and the one
worth putting on a panel. `MemTotal - MemAvailable` is used.

Two sources rather than one, because the pair is what makes it readable:
`mem_pct` is the percentage and `mem_gb` the gigabytes, and `MEM % / GB`
reading `43 / 27` says both of the things somebody wants from a memory gauge.

Not a rate, so unlike the processor there is no sampler to keep: every call
reads the file.

### The shipped screen moves

`TestTheShippedDashboardIsUnchanged` holds the `coolant` dashboard to
`testdata/spec013.gif`, pixel for pixel, and spec 023's promise with it: a
machine that upgrades and touches nothing sees exactly what it saw.

**That promise is broken here, deliberately.** Removing a line changes the
panel, and there is no version of this change that leaves the default screen
alone. Hiding it -- keeping the unit line alive for the four shipped
dashboards so the golden still matches -- would keep the promise by keeping
the redundancy, in the four dashboards most people are looking at.

So the golden is regenerated from this renderer and the test is rewritten to
say what it now holds: the shipped screen is what spec 041 made it, and it
does not move again without somebody meaning it. The changelog says the panel
changes on upgrade, because somebody's screen will.

## Requirements

**R1. A slot takes an optional second source**, drawn as the two values joined
by a separator.

**R2. The separator is the author's**, defaulting to ` / `.

**R3. The unit line is not drawn**, in any arrangement, for any slot.

**R4. `Slot.Unit` is gone** from the model, the API type and the CLI.

**R5. The default label carries the unit**, so a slot nobody has edited says
what it is measured in.

**R6. A dual value grades on its first source**, colour and ring alike.

**R7. A value too wide for its band is drawn smaller**, down to a floor,
rather than over its neighbour.

**R8. Memory usage is two readings**, `mem_pct` and `mem_gb`, from
`/proc/meminfo`.

**R9. The editor sets all of it**: the second source, the separator and the
label, for the headline and for every small slot.

**R10. The shipped dashboards still say what they measure**, their labels
carrying the units their unit lines used to.

**R11. The stacked rows are set as a table**: words left, numbers right, one
left edge and one right edge for the whole arrangement.

**R12. A wide label pushes the numbers rather than meeting them.**

**R13. One point size for the column**, settled by the widest number in it.

## Acceptance Criteria

- [x] AC1. A slot with a second source draws both values joined by the
      separator; with none it draws exactly what it drew before.
- [x] AC2. An empty separator draws ` / `; a set one draws itself.
- [x] AC3. No arrangement draws a unit line, and nothing in the tree reads
      `Slot.Unit`.
- [x] AC4. A slot with no label drawn from `cpu_c` says `CPU °C`; from
      `cpu_pct` and `cpu_c` together, `CPU % / °C`; from `cpu_pct` and
      `gpu_c`, `CPU % / GPU °C`.
- [x] AC5. A dual headline whose first source is a hot coolant is drawn in the
      critical colour, and one whose *second* source is hot is not.
- [x] AC6. A value wider than its band is drawn at a smaller point size, and
      one that fits is drawn at the size it was given.
- [x] AC7. `1450 / 1450` in a Stacked row stays inside its band.
- [x] AC8. `mem_pct` and `mem_gb` are read from a `/proc/meminfo` fixture, and
      a machine without the file reports absence rather than zero.
- [x] AC9. Both appear in `readings.All`, in `hotaru readings`, and in the
      editor's source list.
- [x] AC10. The editor sets a second source, clears it back to none, and sets
      a separator; the preview redraws for each.
- [x] AC11. The editor still fits a default window.
- [x] AC12. The shipped four carry their units in their labels.
- [x] AC13. The golden frame is regenerated and the shipped screen is held to
      it.
- [x] AC14. Verified on the development machine, on the panel.
- [x] AC15. In a stacked dashboard whose rows are different widths, the drawn
      ink starts at the same column in every row and ends at the same column
      in every row, at the row's own edges. Asserted on the rendered pixels,
      not on the arithmetic that places them -- the first version of this
      recomputed the geometry and checked its own answer, and went on passing
      when the alignment was flipped back to centred.
- [x] AC16. A label wider than the words column keeps the gap before the
      numbers.
- [x] AC17. The column is drawn at one size, and it is the size the widest
      number needs.
- [x] AC18. Ring and Grid columns are still centred.

## Risks & Assumptions

- **Every existing dashboard changes on upgrade.** The unit line disappears
  from saved dashboards as well as shipped ones, and a saved one whose author
  wrote a label without a unit will say less than it did. The changelog says
  so. There is no migration: rewriting somebody's labels for them is a worse
  answer than a line in the changelog.

- **`Slot.Unit` is dropped from the file format.** A saved dashboard carrying
  `unit:` keeps it in the file and it is ignored, which is how every other
  unknown key in this format behaves. Nothing fails to load.

- **Shrink-to-fit changes existing renders.** Any value that was already
  overflowing is now drawn smaller. That is the fix, and it is why the golden
  frame is regenerated rather than merely accepted.

- **`MemAvailable` is Linux 3.14 and later.** Older kernels do not have the
  field; the reading reports absence, which the panel already draws as `--`.

- **Two more sources in the editor's list**, which is a `Select` of eleven
  entries now. Still a list somebody reads rather than scrolls.

- **The stacked column is drawn smaller than it was** whenever one row is
  wide: 30 point rather than 34 on the development machine's own dashboard.
  That is the price of one size for the column, and it was checked on the
  panel before being accepted.

- **Rollback** is a revert. The file format only gains keys, so a dashboard
  saved by this version loads in the previous one minus its second source.
