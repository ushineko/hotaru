# Spec 030: one place to make things

**Issue**: [#71](https://github.com/ushineko/hotaru/issues/71)

## Status: COMPLETE

## Context

The navigation had seven entries and two of them were the wrong size.

Pictures, Screen and Scenes are three entries for one job: making something
for the machine to show. Cooling was an entry of its own for four readings
about a device the System section already draws.

So: a **Create** group with Pictures, Screen and Scenes inside it, in that
order -- the order somebody does them in, since a picture becomes a screen and
a screen goes in a scene -- and Cooling folded into System under the machine
it belongs to.

### A group is a section that holds sections

`Create` implements the shell's section interface and forwards `Changed`,
`Busy`, `Detach`, `Arrive` and `Tick` to whichever part is showing. The parts
do not know they are in a group; each is the same section it was, and the
tests that drove them still drive them.

Switching tabs detaches the old part and arrives at the new one, because that
is what navigating between them was before.

### What is loaded

System says what the machine is showing now: the last scene applied, and what
is on the panel. Said as "last applied" rather than "this is the scene",
because the service remembers what it was asked for -- something that changed
the lights another way leaves the label saying what it said, and claiming more
than that is the kind of plausible-looking screen this program has been caught
behind before.

### And it does not rebuild to say it

Those two facts move on the poll, and rebuilding the section to show them
would rebuild every device row twice a minute -- the churn spec 027 spent a
day removing. The cards that follow the machine are held in slots and refilled
by `Tick`: six widgets rather than sixty.

### The tab strip already says the name

Each part drew its own heading from when it was a section of its own. Inside a
tab strip that is the same word twice, a hand's width apart, and the second
one is the one that is not a control.

## Requirements

**R1. Create is one entry** with Pictures, Screen and Scenes inside it, in
that order.

**R2. Cooling is part of System**, not an entry.

**R3. System says what is loaded**: the scene last applied and what the panel
is showing.

**R4. The parts keep their behaviour** -- what each watches, what makes it
busy, what it does on arrival.

**R5. Nothing says its own name twice.**

## Acceptance Criteria

- [x] AC1. The window's sections are Service, System, Create, Appearance,
      About.
- [x] AC2. Create shows Pictures first, and switching parts detaches the old
      one and arrives at the new one.
- [x] AC3. System draws the cooler's numbers and what is loaded.
- [x] AC4. A poll that moves the pump refills the cards without rebuilding
      the device rows.
- [x] AC5. A part's name is on screen once: the tab, not the tab and a
      heading.
- [x] AC6. Verified on the development machine.

## Risks & Assumptions

- **The group is a section to the shell**, so a keyboard shortcut reaches
  Create rather than its parts. Reaching a part is a click on the tab.
- **Rollback** is a revert; the parts are unchanged sections underneath.
