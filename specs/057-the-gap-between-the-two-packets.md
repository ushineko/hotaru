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

## Alternatives Considered

- **A pause after the frame**, which variant E ruled out: it changes nothing,
  so the frame is not what the device is busy with.
- **300ms**, which worked and is no better than 120ms. The smaller number is
  one already in the code with a reason attached to it.
- **Pausing for every non-per-LED mode**, which is the guard spec 052 used. It
  puts a pause on a plain solid-colour scene, which is most scenes, and
  `TestTheOrdinaryWritePathDoesNotWait` rejects it.
- **Dropping spec 056's re-read** now that the pause makes a stale mode less
  likely. It guards a different failure and costs nothing when it does not
  fire.

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
