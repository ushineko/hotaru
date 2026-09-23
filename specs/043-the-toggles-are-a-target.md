# Spec 043: the toggles are a target

**Issue**: [#110](https://github.com/ushineko/hotaru/issues/110)

## Status: COMPLETE

## Context

Caps Lock and Num Lock indicate their state on a lit keyboard and you cannot
read it from a desk away. That is why the only usable schemes here have been
the dark ones: against an unlit board an engaged lock is obvious, and against
a lit one it is a key that looks very slightly unlike its neighbours.

This spec was written twice before it was small enough. What it does now is
almost nothing, and that is the finding.

### The firmware owns those keys, on purpose

**The keyboard paints its lock keys white while they are engaged, over
whatever hotaru wrote, and it cannot be overridden.** Both keys confirmed
through a toggle, against a board set entirely to `#ff0000`.

That is correct behaviour and not an obstacle. An indicator software can
switch off is one that will be off while the lock is on, and then the only way
to know whether you are about to type in capitals is to try it.

So the original request -- turn the light off -- is retired rather than built.
It is exactly what the board is built not to allow. And hotaru never needed to
know what the locks are doing, because the hardware has been saying it all
along. What was missing is something for that white to be seen **against**.

### Which is a colour, and a colour is a scene

The first two designs both put the colour somewhere hotaru would apply on
every frame: a sysfs poll that wrote the key itself, then a `held` block in
the rules file. Both were wrong, and the second was wrong for a reason worth
keeping: **a scene may want the toggles left alone.** One scheme wants them
shouting; the next wants them lighting like the rest of the board.

Once the colour belongs to the scene, there is nothing to build. A scene
assignment on a named segment already does it:

    hotaru light set "#ff8000" "Keychron K4 HE/caps=#ff0000"
    → 98 amber, 2 red, in Direct

That worked before any of this. The `held` implementation was deleted.

### What is actually missing: you cannot aim at them

A hundred-key zone is drawn in the editor as twenty-four blocks of four or
five keys each. That is the right answer for a keyboard nobody wants to click
a hundred times, and it means Caps Lock is a fifth of a block somebody would
be guessing at. The CLI can address it and the window cannot.

So: a rule names which segments are switches, and the editor gives them a row
of their own.

```yaml
  - match: "keychron"
    segments:
      caps: {zone: "Keyboard", leds: [55, 55]}
      num:  {zone: "Keyboard", leds: [33, 33]}
    toggles: [caps, num]
```

**Names only.** Not the colour and not whether a scene uses them -- those are
the scene's, which is the whole correction this spec turns on. What belongs in
the rules file is the part that is a fact about the hardware: which lights
those are, and that they are switches rather than decoration.

The editor draws them as ordinary picks, so everything after the click is
machinery that already exists: select, choose a colour, clear, see it in the
scene's own lines. A scene that wants them left alone does not select them.

### The colour

Configurable, with no recommendation, because the experiment did not find one.
Blue, red, green and purple were each held against an amber board and toggled
by hand; they read close enough together that picking a winner would be
inventing a difference.

The theory that got tried and dropped is worth keeping. Distance from *white*
was the obvious axis, since white is the engaged state, and blue was the
prediction on perceived luminance. Red beat it. Once the firmware supplies the
white, the colour's job is not to be far from white -- it is to be
unmistakably **not the board** while the lock is off. Hue separation from the
scene, not luminance.

Which makes the useful advice about the pair rather than the key: amber with
red is the weakest combination tried, because amber is red plus green. Nothing
enforces that; it is what a person sees in ten seconds and no validation
should pretend to know.

## Requirements

**R1. A device rule can name which of its segments are toggles.**

**R2. A toggle names a segment of that same rule**, and one that does not is
reported and ignored.

**R3. The editor offers each toggle as its own target**, and a device with
none gets no row.

**R4. A toggle is recorded by name**, so a scene says `keychron/caps` rather
than the lights behind it.

**R5. The wizard keeps toggles when it rewrites the rules file.**

## Acceptance Criteria

- [x] AC1. A rule naming toggles parses; one naming a segment that is not
      there is reported, ignored, and the rest of the file still loads.
- [x] AC2. The editor draws a row for a device with toggles, naming each.
- [x] AC3. A device with no toggles gets no row, not an empty heading.
- [x] AC4. Choosing a toggle and a colour puts `<device>/<segment>` in the
      scene, not an LED range.
- [x] AC5. A configuration carrying toggles survives a round trip through the
      wizard's renderer.
- [x] AC6. Verified on the development machine: the toggles clicked in the
      window, coloured, saved and applied -- and judged against the reactive
      mode's own modifier lighting, which is what this replaces.

## Alternatives Considered

- **Blank the key.** The original request. Impossible here and undesirable
  everywhere: an indicator software can switch off is one that will be off
  when it matters.
- **Read the lock state from sysfs and write the key.** The first design. A
  poll, a state machine and a dependency on `/sys/class/leds`, in exchange for
  fighting the hardware for a job it already does.
- **A `held` colour in the rules file.** The second design, built and deleted.
  It applied to every scene, which is the one thing a scene should be able to
  decline -- and once the colour moved to the scene there was nothing left,
  because an assignment on a named segment already did it.
- **Convention instead of configuration**: offer a control for any segment
  called `caps` or `num`. Rejected -- magic, and a segment called `capslock`
  would get nothing.

## Risks & Assumptions

- **It may lose to the reactive mode.** That mode lights the modifiers, which
  on an otherwise dark board makes an engaged lock the most obvious thing on
  the desk. If a coloured key with a white override does not read as well, the
  honest outcome is that the dark schemes stay the right answer. AC6 is the
  only criterion that can settle it.

- **The scene's own colour decides whether any of this works.** The engaged
  state is white and hotaru cannot change that, so a white or near-white
  scheme has an indicator that is invisible precisely when it fires. "Fully
  lit" and "white" are easy to reach for together, and that is the one
  combination this cannot serve.

- **Colouring a toggle pins the keyboard to a per-key mode**, so a scene using
  one gives up the reactive typing effect. Accepted: the two want the same
  hundred LEDs, and a scene that would rather have the effect leaves the
  toggles alone.

- **The LED indices are this board's.** 55 and 33 came from OpenRGB's own
  per-key names, which hotaru does not read -- `light map` still lights zones
  and asks. A board whose layout table is offset would name the wrong key and
  nothing would say so. Reading those names is worth its own spec.

- **Rollback** is a revert; a rules file with a `toggles` line loads unchanged
  in the previous version, which ignores unknown keys.
