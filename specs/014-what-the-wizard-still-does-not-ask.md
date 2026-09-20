# Spec 014: what the wizard still does not ask

**Issues**: [#20](https://github.com/ushineko/hotaru/issues/20), [#25](https://github.com/ushineko/hotaru/issues/25)

## Status: INCOMPLETE

## Context

The wizard produced a complete, working rules file on the development machine,
and two things were wrong with the result that nobody could have seen at the
time.

**`hotaru light off` leaves the keyboard glowing white.** The board takes the
lighting back when it is sent a frame that rounds to nothing; black is not off
there, it is a dead backlight the firmware then replaces with its own. hotaru
knows about this -- `never_blank` is in the configuration and `DefaultOffModes`
names it in a comment -- and the wizard never writes it, so the file it
generates has no trace of the one correction this device needs.

**The cooler's ring got no name.** The wizard lit it, asked what was lit, and
got a bare return. `examples/hotaru.yml` recorded that as somebody not looking
closely. That was an inference and it was wrong: the ring takes about half a
second to show a colour, and the question is asked in the same breath as the
write. The honest answer to "what is lit now?" was whatever had been there
before.

Both are the same failure in different clothes. The wizard is the only part of
hotaru that can learn a fact about a machine that no protocol reports, and it
is asking too few questions, one of them too early.

peripheral-battery-monitor knows all of this, learned on this same hardware:

	OFF_EXEMPT = ("keychron",)          # black kills the backlight
	BRIGHTNESS_DEVICES = ("keychron",)  # nobody owned brightness; whatever
	DEFAULT_BRIGHTNESS = 100            # was written last stuck, invisibly

	KEYBOARD_EFFECTS = {
	    "splash": ("solid splash", "direct", "solid color"),
	    "solid":  ("direct", "solid color", "static"),
	}

The third is the interesting one. "Off" on that keyboard is not a colour, it is
a **mode**: `Solid Splash` leaves unpressed keys dark and ripples the scene
colour under typing, which is what somebody actually wants from a keyboard that
is off. hotaru can already express it -- a rule's `solid_modes` bypasses the
effect filter -- and never suggests it.

### Questions are demonstrated, not asked

The wizard's method is already settled by spec 008: light one thing, with
everything else dark, and ask what changed. Every question added here follows
it rather than inventing an interview.

"Should this device be excluded from turning off?" is a question about hotaru's
configuration, and answering it requires knowing what `never_blank` means.
"I have turned the keyboard off -- is it dark?" is a question about the room,
and a person answers it by looking up.

That is the difference between a wizard and a form.

### Waiting is part of asking

Nothing announces that a device is slow. The cooler's ring takes half a second;
its fans are immediate; the mousepad is immediate. The only way to find out is
to write, wait, and see -- which is what a person does anyway, and what the
wizard currently does not.

A fixed pause is the whole fix. Long enough for the slowest thing measured on
this hardware, short enough that nobody notices waiting: the same reasoning as
`settleDelay` in the write path, and the same order of magnitude.

## Requirements

**R1. A pause between lighting something and asking about it.** Applied
everywhere the wizard writes and then asks -- the mode probe, the zone round,
the chain split and the bisection -- because a device that lags lags in all of
them.

**R2. Turning off is demonstrated, and the answer is recorded.** For every
device with lighting the wizard has named, it turns the device off, asks
whether it went dark, and writes `never_blank: true` when it did not. Asked in
the room's terms, with no mention of blanking, modes or frames.

**R3. A device that will not go dark is offered the alternative.** Where such a
device advertises a mode that renders mostly dark -- the reactive effects a
keyboard has -- the wizard offers it, demonstrates it, and records it in
`solid_modes` if it is preferred. Where no such mode exists, hotaru says the
device cannot be turned off and moves on; that is a fact about the hardware and
not a failure of the run.

**R4. Brightness is offered only where the device takes one, and by
demonstration.** The wizard turns the device down, asks whether that is better,
and records the answer. It does not ask for a number. A device whose modes
report no brightness is never asked about.

**R5. No jargon, as everywhere else in the wizard.** The existing test that
fails on "zone", "LED" and mode names covers every new line.

**R6. Every question remains skippable**, and skipping writes nothing rather
than writing a default. A file that claims a device was checked when the person
pressed return is worse than one that says nothing about it.

**R7. The additions are re-runnable.** Spec 008's reconfiguration pattern
holds: a second run offers the previous answers as defaults, including these.

## Acceptance Criteria

- [ ] AC1. The wizard pauses between writing and asking, in every round that
      does both, and a test asserts the pause exists on each path.
- [ ] AC2. A device that does not go dark gets `never_blank: true` in the
      written file.
- [ ] AC3. A device that does go dark gets nothing written about it.
- [ ] AC4. A device that will not go dark and has a mostly-dark mode is
      offered it, and accepting writes that mode first in `solid_modes`.
- [ ] AC5. A device that will not go dark and has no such mode is told so in
      one sentence, and the run continues.
- [ ] AC6. Brightness is offered only for devices whose modes report one.
- [ ] AC7. Accepting a dimmer setting writes `brightness`; declining writes
      nothing.
- [ ] AC8. The no-jargon test covers the new questions.
- [ ] AC9. Pressing return at any new question writes nothing for it.
- [ ] AC10. A second run offers the previous answers as defaults.
- [ ] AC11. Verified on the development machine: the Keychron is found to not
      go dark, is offered its reactive mode, and the written file carries both
      corrections; the cooler's ring is named on a first pass.

## Risks & Assumptions

- **More questions is a cost.** Spec 008's whole argument is that a wizard's
  quality is the wording of six or seven questions. This adds up to three per
  device, so they are asked only where they can be acted on -- no brightness
  question without a brightness-capable mode, no effect question without an
  effect.
- **The pause is a guess informed by one machine.** Half a second covers
  everything measured here. A slower device would still be mis-asked, and the
  wizard cannot know; the mitigation is that a person who sees the wrong colour
  can answer "cannot tell", which spec 008 already accepts.
- **Demonstrating "off" turns somebody's lights off mid-run.** They go back:
  the wizard's writes are previews and the run already ends by restoring.
- **Rollback** is to stop asking; the fields are already supported and
  hand-editable, which is where they live today.

## Alternatives Considered

Considered asking these questions as plain questions rather than demonstrating
them -- shorter to implement and shorter to run. Rejected because answering
"should this device be excluded from off?" requires knowing what hotaru means
by off, and the wizard exists precisely so that nobody has to.

Considered asserting a default brightness the way the Python does, with no
question at all. Rejected: the Python could scope it to one device it knew
about, and hotaru is writing a file for a machine it has never seen. Setting a
brightness nobody asked for is the kind of silent change spec 008 forbids.
