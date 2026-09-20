# Spec 010: the mode packet is the commit

**Issue**: [#23](https://github.com/ushineko/hotaru/issues/23)

## Status: COMPLETE

## Context

hotaru sends a mode packet before every frame, including to a device already in
the mode being asked for. That looks like waste, and `settle()`'s own comment
suggests it is worse than waste:

> A mode change is not instant on every bus. Over SMBus to a stick of DDR5 it
> is slow enough that a frame written immediately afterwards lands while the
> controller is still changing mode, and is discarded.

So the obvious improvement is to skip the packet when it changes nothing: same
result, one less thing for the frame to race against.

It is not an improvement. On the development machine's NZXT cooler **the mode
packet is what commits the frame**. Suppress it and the cooler stops changing
colour at all -- both its channels hold whatever they were showing while every
other device on the machine cycles around them.

Built four ways and tried with somebody watching the case, five colours in
sequence:

| mode packet | frames per write | the cooler |
|---|---|---|
| always | 1 | tracked every colour |
| **skipped when unchanged** | 1 | **never changed at all** |
| **skipped when unchanged** | 2 | **never changed at all** |
| always | 2 | tracked every colour |

The two that froze are the two that skipped the packet. The cooler's fans are
the evidence rather than its ring: the fans respond promptly and consistently,
where the ring lags and has a failure mode of its own (#24).

## How the mistake measured as a success

With the packet suppressed, the rate of `settled` attempts -- devices whose
colours did not match on the first read-back -- fell from four devices in six
to nearly zero. That was reported as the change working.

It was the writes not arriving. `settled` compares a frame hotaru sent against
colours the server reports, and a write that never reaches the hardware
produces perfect agreement between the two. **The metric improved because
hotaru had stopped talking to the devices.**

This is the third time in two days that a number derived from OpenRGB's
read-back has pointed the wrong way. Spec 009 found it being used to excuse a
write nobody could confirm; this spec used it first to gate a retry and then to
validate a regression. The rule that follows:

> **No change to the write path is accepted on hotaru's own telemetry.**
> Somebody looks at the machine.

## Requirements

**R1. The mode packet is sent on every write**, including to a device already
in the mode being asked for. It is not redundant; on NZXT hardware it is the
commit. A test says so, named for the hardware rather than for the code,
because the reason lives in a cooler and not in this repository.

**R2. Brightness continues to be asserted on every write.** Device state nobody
owns: whatever was written last sticks, invisibly, until something asserts it.
Carried from peripheral-battery-monitor, which learned it on this machine.

**R3. Nothing else in the write path changes.** In particular the frame is
written once. An earlier draft required writing every frame twice, justified by
the `settled` rate above -- a number this spec has just discredited -- and
adding packets to hardware that stops responding when written to repeatedly
(#24) is the wrong direction on its own merits.

## Acceptance Criteria

- [x] AC1. A device already in the requested mode still receives a mode packet,
      with a test that says why.
- [x] AC2. Brightness is asserted on every write.
- [x] AC3. The frame is written once per attempt; the 120 ms settle still
      applies where the read-back disagrees.
- [x] AC4. A test fails if a sleep reaches the ordinary write path, so the
      write stays fast enough to be worth having.
- [x] AC5. Verified on the development machine by eye: five colours in
      sequence, the cooler tracking each one, with the suppressed-packet
      builds compared side by side.

## Risks & Assumptions

- **This is one machine's cooler.** The finding is that the packet is sometimes
  load-bearing, not that it always is. Sending it costs one packet and is what
  hotaru has always done; the risk is entirely on the side of removing it.
- **The first draft predicted its own failure.** Its risk section read: "a
  device that needs a redundant mode packet to wake up would regress, and no
  such device is known". The cooler is that device. The risk was written down
  and then overruled by the requirement above it, which is worth remembering
  the next time a risk section says "no such device is known".
- **Rollback** is a revert; the change restores prior behaviour rather than
  introducing any.

## Alternatives Considered

Considered sending the mode packet only for devices known to need it, by rule;
rejected because the packet is cheap, the rule would have to be discovered per
device by somebody watching their case, and the failure when the rule is
missing is total silence from the hardware.
