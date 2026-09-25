# Spec 052: the mode that did not stick

**Issue**: [#153](https://github.com/ushineko/hotaru/issues/153)

## Status: COMPLETE

## Context

Spec 051 gave an effect a colour of its own, and it worked between scenes that
both name one. Coming from a scene that does not -- an all-lit scene, every
light its own colour, the keyboard in Direct -- the effect was entered and the
colour was not.

Nothing hotaru could read said so. The mode read back as active, its colour
slot read back as the colour it had been given, the frame was written, and the
keyboard showed the keys.

### What the machine was asked, and what it answered

Two scenes with one difference apart, watched a quarter-second at a time
through four cycles:

```
attackiq → Direct                     buffer0=#5a3db5
custom1  → Solid Reactive Multinexus  modecolour=#8c8c8c buffer0=#00708c
```

Identical every cycle. So the fault was not in what hotaru sent, and not in
anything a read-back could reach.

**A scene built to disagree with itself settled it.** Every key blue in the
buffer, green in the mode's colour slot, applied to a keyboard already in that
mode: the keyboard went green. The mode's colour is what it displays, the
buffer is not, and a frame written to a device already in the mode changes
nothing it shows.

That leaves the transition. The packet carries the mode and its colour
together, and the keyboard takes them as two operations: entering the mode, and
colouring it. It honours the colour when it is already in the mode and loses it
when it is arriving -- which is exactly the asymmetry that was reported, and
the reason two preset scenes in a row were always right.

### The packet goes twice

So the mode is asserted once more after the frame, and the second one is the
already-in-the-mode case.

This is spec 010 from the other side. There the mode packet was sent even
though the device was already in that mode, because on an NZXT it is what
commits the frame; here it is sent again after the frame, because on a
keyboard it is what commits the colour. Both are the same sentence: a packet
that looks redundant from the protocol is load-bearing on the hardware, and
only the hardware can say which.

Only for the modes that cannot show a frame. A device in Direct is showing the
frame, and sending its mode twice would be two writes where one says
everything.

## Requirements

**R1. A mode that shows one colour of its own is written again after the
frame**, so its colour survives arriving from a per-LED mode.

**R2. A per-LED mode is written once.**

**R3. The extra write is a mode packet and nothing else**: no second frame, no
change to what is remembered, and no change to what the result reports.

## Acceptance Criteria

- [x] AC1. Applying a scene whose effect shows one colour, from a device
      sitting in a per-LED mode, writes that mode twice
      (`TestAModeThatShowsOneColourIsAssertedAgainAfterTheFrame`).
- [x] AC2. Applying a scene that lands on a per-LED mode writes it once
      (`TestAPerLEDModeIsWrittenOnce`).
- [x] AC3. Verified on the development machine: `attackiq` then `custom1` --
      the transition that lost it -- leaves the keyboard showing the effect's
      colour, as preset-to-preset always did.

## Alternatives Considered

- **Not writing the frame at all for such a mode.** The first guess, and the
  disagreeing scene ruled it out: a frame written to a device already in the
  mode changes nothing it shows, so the buffer was never the cause. It would
  also throw away the colours a scene keeps for the moment the effect comes
  off.
- **A settle between the packets**, as the per-LED path has for DDR5 over
  SMBus. Nothing here needed a delay; the second packet on its own was enough,
  and a sleep nobody can justify is a sleep that gets longer.

## Risks & Assumptions

- **One extra packet per device per apply**, for effect modes only. Sub-
  millisecond against a running server, and the same packet the write already
  sends.
- **Read-back cannot confirm this.** Every field hotaru can read said the
  write had worked while the keyboard disagreed, so this rests on looking at
  the hardware -- which is what AC3 is.
- **Rollback** is a revert. Nothing is stored differently.
