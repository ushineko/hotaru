# Spec 011: a zone is what the hardware writes

**Issue**: [#24](https://github.com/ushineko/hotaru/issues/24)

## Status: COMPLETE

## Context

hotaru wrote a device's colours as one array covering every LED it has. The
protocol offers that, and it reads as the obvious choice: a frame is the whole
device, atomically, which is what `Frame`'s own documentation promises.

On an NZXT Kraken it is the wrong shape. The cooler's two Hue 2 channels --
the ring around the pump head, and the chained radiator fans -- are independent
controllers behind one USB endpoint. Handing OpenRGB a single 48-LED array
spanning both meant they were never delivered together, and the cooler behaved
accordingly:

- one channel showed a stale colour while the other moved
- a frame rendered torn, part way along a chain of three fans
- under a run of writes it stopped responding for minutes at a time, ignoring
  everything including black, then recovered on its own

Written one request per zone, it tracks.

| | whole device | per zone |
|---|---|---|
| six colours, 2.5 s apart | missed most, stale channels | tracked every one |
| six colours, 1 s apart | stopped responding for minutes | one missed transition, correct final colour |

### What it cost to find

Most of a day, and most of that was spent on explanations that were wrong:
drift, dropped frames, a colour-specific fault, a rate limit, and a mode packet
that turned out to be innocent of what it was accused of. Every one of them was
built from observations taken immediately after a write, on the channel that
happens to lag by about half a second, and read as evidence.

The question that ended it came from the person looking at the case: *are we
sending commands all at once through OpenRGB, or are they separate API calls?*
Nobody had looked. `SetFrame` had always sent one request, and the fact that a
device's zones might be separate hardware was never considered, because the
abstraction said a device was the unit.

**A zone is not a way of naming part of a device. On some hardware it is a
separate controller.** hotaru's own catalogue already knew this -- it records
each device's location, and the ring and the fans are one USB node -- but the
write path flattened it away.

### What it did not fix

The cooler still misses the occasional update, and once left a single LED at
the 11 o'clock position of the ring showing a colour from two writes earlier
while its twenty-three neighbours were correct. Rewriting the same frame did
not move it; putting the device through another mode and back did.

So this spec claims an improvement, not a fix. That distinction matters because
"root cause found" was said out loud during the investigation and was not true.

## Requirements

**R1. A frame is written one request per zone**, in device LED order, using the
protocol's per-zone update.

**R2. A device reporting no zones is written whole.** There is nothing to cut
it by, and a frame is still the entire device.

**R3. No colour is lost between zones.** Where a device's zone sizes do not add
up to its LED count -- hardware lies about its own sizes, and the catalogue has
seen it -- the remainder travels with the last zone rather than being dropped.
Lighting part of a device and reporting success is the failure this whole area
keeps producing.

**R4. A zone claiming more LEDs than the frame holds does not panic.**

## Acceptance Criteria

- [x] AC1. `SetFrame` sends one `UpdateZoneLeds` per zone.
- [x] AC2. A device with no zones gets one whole-device write.
- [x] AC3. Every colour in a frame reaches some zone, tested against a device
      whose zones under-count its LEDs.
- [x] AC4. A zone over-counting the frame is clamped rather than panicking.
- [x] AC5. Verified on the development machine: six colours in sequence, the
      cooler's ring and fans both tracking, where the same sequence on the
      previous build stalled it.

## Risks & Assumptions

- **Zones are contiguous in device LED order.** The same assumption the
  catalogue already makes when it counts zone offsets; if it is wrong, segments
  address the wrong lights and have always done so.
- **More requests per write.** Two for the cooler where there was one. On
  hardware that dislikes traffic this could in principle be worse, and
  measurably was not.
- **Rollback** is a revert to the single whole-device write.
- **The cooler remains unreliable at the edges.** Tracked in #24, along with
  the one recovery that works: another mode and back.

## Alternatives Considered

Considered writing per zone only for devices known to need it; rejected because
a zone is the hardware's own division and there is no reason to think this
cooler is the only device whose zones are separate controllers -- and the
failure, when the list is missing an entry, is silent.
