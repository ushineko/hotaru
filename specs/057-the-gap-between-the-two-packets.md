# Spec 057: the gap between the two packets

**Issue**: [#162](https://github.com/ushineko/hotaru/issues/162)

## Status: COMPLETE

## Context

Spec 056 put the frame ahead of the two mode packets and the colour still did
not stick. This is the last variable, found by doing what should have been
done three specs earlier: sending hotaru's exact packets from outside hotaru,
one variant at a time, and looking at the keyboard after each.

### The packets hotaru sends

Dumped from a replay of the real scene against the real device catalogue:

```
SetFrame(100 LEDs, all #fffffc)
SetMode("Solid Reactive Multinexus", colour=#fffffc, speed=89)
SetMode("Solid Reactive Multinexus", colour=#fffffc, speed=89)
```

### The same packets, sent by hand

| | packets | keyboard |
|---|---|---|
| A | exactly the above, back to back | **wrong colour** |
| B | the same, 300ms between each | right |
| E | the gap **only between the two mode packets** | right |
| F | the same gap at 120ms | right |

A reproduced the fault with hotaru out of the loop, which is what made the
rest meaningful. E is the finding: a gap after the frame does nothing, and the
device needs time to finish *entering* the mode before the packet that colours
it. F says hotaru's existing `settleDelay` is long enough.

The speed field was a suspect and is innocent: every variant carried it.

### What else was ruled out

Three further variants, each sent by hand and looked at:

| | change | keyboard |
|---|---|---|
| G | the scene's own mixed colours in the buffer, not the effect's | right |
| H | the original order -- mode, then frame -- with the gap | right |
| READ | the mode read back immediately after the second packet, five times | correct every time |

So the buffer's contents do not matter, the frame's position does not matter,
and the device does not need asking twice once there is a gap. Spec 055's
rewrite of the buffer and spec 056's reordering and re-read were all reverted
on the strength of this, leaving the gap as the only change.

**Reverting spec 055 exposed one thing it had been masking.** A mode carrying
its own colour was checked for "is it showing what it was sent" against the
*frame's* uniform colour, which an effect with a colour of its own never
matches -- so every apply of such a scene concluded the device was not showing
its frame and paid for a settle: a sleep and a second frame write. It is
checked against the colour that was asked for now, which is what the device is
actually displaying.

### Why spec 056 read it wrong

Spec 056 measured three sequences and concluded a pause was needed only before
the read-back. That measurement asked the device **which mode it was in**, and
never looked at the keyboard. The mode sticks without a gap; the colour does
not. Two symptoms, one measurement, and the generalisation was taken from the
wrong one.

The re-read spec 056 added is kept. It guards a different thing -- a device
reporting a mode it has taken, which is how the effect got abandoned for
Direct -- and the pause here makes it fire less often rather than making it
wrong.

### Who pays for the pause

A pause per device is paid by every device on the machine, and spec 010 AC7
makes keeping the write path fast a requirement rather than a nicety. So it is
scoped by what the measurement actually says:

- **Only a mode carrying a colour of its own.** A mode that is neither
  per-LED nor colour-carrying has no colour to commit, so the second packet is
  already doing nothing for it.
- **Only when arriving.** A device already in the mode took the colour on the
  first packet, which is spec 052's original finding. Re-applying the scene
  the machine is already showing is the common case and stays fast.

## Requirements

**R1. The second mode packet waits `settleDelay` after the first**, so it
arrives once the device has finished entering the mode.

**R2. Only a mode that carries a colour of its own waits**, because only it
has a colour to commit.

**R3. Only a device arriving at the mode waits.** One already in it took the
colour on the first packet.

**R4. The ordinary write path still does not wait** (spec 010 AC7).

## Acceptance Criteria

- [x] AC1. Arriving at a colour-carrying mode waits before the second packet
      (`TestOnlyAModeArrivingWithAColourOfItsOwnWaits`, first case).
- [x] AC2. Re-applying a scene the machine is already showing does not wait
      (same test, second case).
- [x] AC3. A scene of plain colours does not wait (same test, third case),
      alongside the existing `TestTheOrdinaryWritePathDoesNotWait`.
- [x] AC4. Every path to the hardware still writes the same order
      (`TestEveryPathToTheHardwareWritesTheSameOrder`): applied, previewed,
      changed under a lease, applied while the window holds one, and
      re-asserted after one is released.
- [x] AC5. Reproduced outside hotaru: variant A, hotaru's exact packets sent
      back to back from OpenRGB, shows the wrong colour.
- [x] AC6. Verified by looking at the keyboard: `attackiq` then `custom1`
      through hotaru leaves it in Solid Reactive Multinexus showing `#fffffc`,
      which is what the scene asks for.
- [x] AC7. Nothing from the superseded attempts is left in the code: the
      buffer holds the scene's own colours
      (`TestTheBufferHoldsTheScenesOwnColours`), the mode goes again after
      the frame rather than before it
      (`TestAModeWithAColourOfItsOwnIsWrittenAgainAfterTheFrame`), and the
      fake's slow-reporting knob is gone with the re-read it existed for.
- [x] AC8. A device already in the mode is not written twice and does not
      wait (`TestEveryPathToTheHardwareWritesTheSameOrder`, the re-assert and
      draft cases), so the re-assert loop and an editor's keystrokes cost
      nothing.

## Alternatives Considered

- **A pause after the frame**, which variant E ruled out: it changes nothing,
  so the frame is not what the device is busy with.
- **300ms**, which worked and is no better than 120ms. The smaller number is
  one already in the code with a reason attached to it.
- **Pausing for every non-per-LED mode**, which is the guard spec 052 used. It
  puts a pause on a plain solid-colour scene, which is most scenes, and
  `TestTheOrdinaryWritePathDoesNotWait` rejects it.
- **Keeping spec 056's re-read** as insurance. With the gap the device
  reported correctly five times out of five, and a device slower than the gap
  would lose its colour rather than its mode -- so the re-read guards a case
  that can no longer arise while looking like it guards the reported one.
  Removed rather than left to be believed.
- **Keeping spec 055's buffer rewrite** because it was part of the sequence
  first confirmed on the hardware. Variant G isolated it and the buffer turned
  out not to matter, so keeping it would mean the device no longer holding
  what the scene says for no reason at all.

## Risks & Assumptions

- **One `settleDelay` per colour-carrying mode per device, when arriving.**
  On the development machine that is one device and one pause on the scenes
  that use an effect, and nothing at all on the scenes that do not.
- **The measurements are from one keyboard**, as spec 056's were. What
  generalises is the shape -- a device needs time between entering a mode and
  being told its colour -- not the number.
- **120ms may not be enough for a slower device.** It would show as the
  colour not sticking, which is the symptom this spec started from, and the
  fix would be the constant rather than the structure.
- **Rollback** is a revert. Nothing is stored differently.

## Executive Summary

An effect's colour still did not stick after spec 056. Sending hotaru's exact
packets by hand, one variant at a time, showed the device needs a gap between
the two mode packets -- not after the frame, and not the speed field that was
suspected. 120ms is enough. The pause is scoped to a mode carrying its own
colour and to a device arriving at it, so the ordinary write path still does
not wait.

Reviewers should look at `writeFrame` in `internal/service/service.go`, and at
`TestOnlyAModeArrivingWithAColourOfItsOwnWaits` for who pays for the pause.
