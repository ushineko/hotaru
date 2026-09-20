# Spec 009: writes that mean what they say

**Issue**: [#16](https://github.com/ushineko/hotaru/issues/16)

## Status: COMPLETE

## Context

`hotaru light set purple` turned three radiator fans red.

Nothing in hotaru noticed. The device accepted the write, reported back the
mode it had been asked for, and returned an LED buffer holding exactly the
colours that had been sent. `Applied: true`. The fans were red because red was
what NZXT had last stored in the device's Static mode, and Static on that
device takes its colour from the mode rather than from the buffer.

The mistake is not the mode choice. It is that hotaru writes half of what a
mode needs and treats the result as a write.

An OpenRGB mode declares where its colour comes from: per LED, from the mode's
own colour slots, or nowhere. hotaru reads that flag, uses it to decide whether
a mode *can* carry a frame (`Frame.PerLED`, `SolidCandidates`), and then writes
the per-LED buffer regardless. For a mode-specific mode the buffer is ignored
by the hardware and the mode's colour -- which hotaru never sets -- is what the
user sees.

peripheral-battery-monitor never had this failure, for a reason worth recording:
it drives the OpenRGB command line, and `--mode static --color 8000FF` sets both
halves in one call (`rgb_openrgb.py:363`). Using the binary protocol directly is
the right choice for a service, and it is the choice that made this hotaru's
problem to solve rather than OpenRGB's.

### Why it stayed invisible

`showing()` is the function that verifies a write landed. Its comment says:

> A device in Static reports its mode's colour rather than its buffer, and a
> device that reports nothing at all is simply not saying -- neither is
> evidence of a failed write, and treating them as one would fail every write
> to hardware that works.

That reasoning is sound and the conclusion was wrong by one step. It is true
that a mode-specific device's buffer proves nothing. It does not follow that
nothing can be checked: the mode's colour can be read back, and it is the thing
that matters. The one place that could have caught this excused it instead.

The result is a class of write that hotaru cannot distinguish from a working
one, and it is reported as `Applied`. That is the part this spec treats as the
defect. A wrong colour is a bug; a wrong colour reported as success is a
promise hotaru should not have made.

### Scope

Lighting only. The cooler's LCD (spec 003) has its own write path and is not
touched here.

## Requirements

**R1. A mode's colour is set when the mode takes one.** Where the chosen mode
declares mode-specific colour, hotaru sets the mode's colour slots as part of
applying the frame, from the frame's uniform colour.

**R2. A frame that is not uniform never lands in a mode that cannot show it.**
Already true via `Frame.PerLED` and `SolidCandidates`; this spec must not
regress it, and a test says so.

**R3. Per-LED modes are preferred where a device has one.** A per-LED mode is
exactly controllable and its result is verifiable by read-back; a mode-specific
mode is neither, even once R1 lands, because the hardware may hold more state
than the protocol exposes. The static-first default exists for a real reason --
an RTX 4090 rejects Static loudly while an ASUS board fails silently -- so the
preference is expressed as "a mode whose colour hotaru fully controls, first",
not as a reordering of named modes. A rule's own `solid_modes` still wins.

**R4. What hotaru can check, it checks; what it cannot, it leaves alone.**
Once R1 and R5 land, a mode-specific write is confirmable and a disagreement is
reported through `Unconfirmed`, as a per-LED disagreement already is. A mode
that reports neither a buffer nor a colour of its own is left unremarked, which
is the existing deliberate behaviour: attaching a caveat to every write on such
a device is how working hardware was made to look broken once already, and
`TestADeviceInAModeThatCannotReportColoursIsNotDoubted` guards it. Report, do
not judge.

**R5. The mode's colour is read back where the device reports it.** A
mode-specific write is verified against the mode's colour, the way a per-LED
write is verified against the buffer.

**R6. Nothing is written to a device that the device keeps.** Setting a mode's
colour is device state that can persist across reboots on some hardware.
hotaru sets it as part of a write it was asked to make and never calls the
save-mode request, so a machine that has never run hotaru looks the same after
hotaru stops as before it started.

## Acceptance Criteria

- [x] AC1. `Mode` carries whether it takes a mode-specific colour, read from
      the mode's flags, alongside the existing `PerLED`.
- [x] AC2. Applying a uniform frame in a mode-specific mode sets the mode's
      colour to the frame's colour.
- [x] AC3. Applying a frame in a per-LED mode writes the buffer and does not
      touch the mode's colour.
- [x] AC4. A non-uniform frame is never applied in a mode that is not per-LED.
- [x] AC5. Where a device advertises both, the mode hotaru chooses by default
      is the per-LED one; a rule naming `solid_modes` still wins.
- [x] AC6. A mode-specific write is verified against the mode's colour read
      back, and mismatch is reported.
- [x] AC7. A mode-specific write whose colour reads back wrong reports
      `Unconfirmed`, and is still `Applied` -- the device took the mode.
- [x] AC8. A mode reporting neither a buffer nor a colour of its own is not
      doubted, and the existing test saying so still passes unchanged.
- [x] AC9. The save-mode request is never sent.
- [x] AC10. A fake device whose Static mode holds a colour different from its
      buffer is a test fixture, and the test fails on the pre-fix behaviour.
- [x] AC11. The live test reports, read-only, which modes on the attached
      hardware carry their own colour and what colour each holds. It still
      writes nothing: a test that changed somebody's lighting is the
      discourtesy that test already refuses.
- [x] AC11a. Verified by hand on the development machine, with somebody
      looking at it: `hotaru light set <colour>` with the Kraken's
      `solid_modes` override removed lights the radiator fans and the ring in
      the colour asked for, not NZXT's red.
- [x] AC12. The development machine's rules file no longer needs the
      `solid_modes: [direct]` workaround added for the Kraken, confirmed by
      removing it and looking at the machine. (`examples/hotaru.yml` never
      carried it; its own `solid_modes` is the ASUS board's separate quirk,
      which this spec does not touch.)

## Verified on hardware

Development machine, the morning after the defect was found.

The G502 turned out to be the same defect as the cooler, not the separate
"wakes up having forgotten" problem it had been filed as. Its Static mode held
`#ff0000`; it had been showing the mouse's vendor red for as long as anyone had
looked, and the sleep story fitted well enough that nobody checked. The live
test's read-only mode report is what made it obvious:

	G502 X PLUS: mode "Static" carries its own colour, holding #ff0000

With the fix, both the mouse and the cooler take Direct, and both show the
colour asked for. The Kraken's `solid_modes: [direct]` correction was removed
from the development machine's rules file and the fans and ring stayed right,
which is AC12.

**A caution for whoever reads this next.** The same session produced four
devices where "the buffer says one colour and the hardware shows another" meant
a real defect, and then a fifth where it meant nothing at all: the mousepad's
logo has no blue channel, so a request for `#8000ff` shows as dim red. It was
diagnosed as drift, then as a dropped frame, then correctly -- and confirmed
only by asking OpenRGB's own command line for the same colour and getting the
same dark logo. A read-back that disagrees with the hardware is not evidence of
a bug in hotaru. Sometimes the light simply cannot make that colour, and hotaru
has no way to know which zones those are.

## Risks & Assumptions

- **Verification on real hardware is the acceptance test, not the unit tests.**
  This defect passed every unit-level check that existed. AC11 is the one that
  matters and it needs the machine.
- **Setting a mode's colour writes device state.** R6 keeps it transient by
  never saving, but a device whose firmware persists mode colour regardless
  will keep the last colour hotaru set. That is the same thing the vendor's own
  software does, and it is why the save request is the line.
- **Rollback** is a revert: the change is additive (one flag, one write, one
  verification) and the existing `solid_modes` override covers affected
  machines in the meantime.
- **R3 changes the default mode chosen on hardware that works today.** The ASUS
  board is the case to re-test: its headers render only in Direct, which R3
  happens to favour, but the GPU's loud Static rejection must still fall
  through cleanly.

## Alternatives Considered

Considered leaving the mode choice alone and only setting the mode's colour
(R1 without R3); rejected because it leaves every solid write unverifiable when
a verifiable path exists on the same device.

Considered driving the OpenRGB command line the way the monitor does; rejected
because a service that shells out per write gives up the queue, the latency
(0.6 ms per device against ~180 ms) and the error reporting that the binary
protocol provides.
