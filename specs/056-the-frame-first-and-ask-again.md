# Spec 056: the frame first, and ask again

**Issue**: [#160](https://github.com/ushineko/hotaru/issues/160)

## Status: COMPLETE

## Context

Specs 052 and 055 both tried to make an effect's colour survive arriving from
a per-LED scene, and both failed on the hardware while passing every test
written for them. This is what was actually wrong, and how it was finally
measured.

### Taking hotaru out of the loop

Every earlier attempt reasoned from read-backs, and every read-back said the
write had worked. The thing that broke the deadlock was driving the keyboard
from OpenRGB's own CLI, with none of hotaru's code in the way:

| | result |
|---|---|
| arriving from Direct: `--mode "Solid Reactive Multiwide" --color 00FF00` | red |
| the identical command again, already in the mode | green |

So spec 052 was right that the packet has to go twice, and right that the
second one is the already-in-the-mode case. It did not work because of what
hotaru did *between* them.

### The frame between the packets

```
hotaru:       SetMode(colour) -> SetFrame -> SetMode(colour)    red
OpenRGB CLI:  SetMode(colour) -> SetMode(colour)                green
```

A frame landing between the two puts the device back in the arriving state,
so hotaru's second packet was never the already-in-the-mode case the fix
depended on. Both packets were correct and the first one was undone.

Moving the frame ahead of both was confirmed on the keyboard before any of it
was written: frame, then the mode twice, arriving from Direct, shows the
colour.

### And then it reported the wrong mode

The reorder alone made things worse, which is how it was reported: applying
the scene left the keyboard in Direct showing the scene's own colours, with
the effect abandoned entirely.

```
attempt: mode="Solid Reactive Multiwide"  accepted=true  active="Direct"
attempt: mode="Direct"                    accepted=true  active="Direct"
```

The device reports its old mode if it is asked straight away. Measured three
ways:

| sequence | mode read back |
|---|---|
| frame, mode, mode -- no delay | `Direct` |
| frame, mode, mode -- 300ms between packets | `Solid Reactive Multiwide` |
| frame, mode, mode -- delay before the **read** only | `Solid Reactive Multiwide` |

The third line is the useful one: the writes land either way, and only the
reporting is slow. hotaru read back immediately, saw the old mode, took it as
evidence the mode had not taken, and fell through to the next candidate --
ending in the mode it had decided against.

### Asked again, not waited for

The first version of this slept before every read-back and a test caught it:
the ordinary write path does not sleep, and a delay every device pays for one
device's slowness is the wrong trade. So the second read happens only when the
first one disagrees, and only for a mode that shows one colour of its own.

### What the earlier specs got wrong

Spec 052 read the asymmetry correctly and missed the frame sitting between its
two packets. Spec 055 concluded the device shows its buffer rather than its
mode colour; the keyboard later read back with buffer and mode colour both
correct while still showing the wrong thing, which disproved it. What survives
from 055 is the change itself, not its reasoning: the buffer is written in the
mode's colour, it was part of the sequence confirmed on the hardware, and
having the two agree costs nothing. See spec 055 for what it now claims.

## Requirements

**R1. A mode that shows one colour of its own takes the frame first**, then
the mode, then the mode again, with nothing between the two mode packets.

**R2. Every other mode is unchanged**: the mode packet first and every time,
because on an NZXT it is what commits the frame (spec 010).

**R3. A device that reports the old mode is asked once more**, after a pause,
before the mode is treated as not having taken.

**R4. The ordinary write path still does not sleep.** Only a disagreement
costs anything, and only for these modes.

## Acceptance Criteria

- [x] AC1. A one-colour mode is written frame, mode, mode
      (`TestAModeThatShowsOneColourTakesTheFrameBeforeTheMode`).
- [x] AC2. A per-LED mode is written mode, frame
      (`TestAPerLEDModeTakesTheModeBeforeTheFrame`).
- [x] AC3. A device that reports the old mode on the first read still ends up
      in the mode it was given, rather than falling through
      (`TestASlowModeIsAskedAgainRatherThanAbandoned`).
- [x] AC4. The ordinary write path does not sleep (the existing test that
      caught the first version of R3).
- [x] AC5. Measured on the development machine, through the service:
      `attackiq` then `custom3` reports
      `{"mode":"Solid Reactive Multiwide","accepted":true,"active":"Solid
      Reactive Multiwide"}` on the first attempt, and the device reads back
      with mode colour and buffer both `#67798c`.
- [x] AC6. Verified by looking at the keyboard: the reported transition leaves
      it in the reactive mode showing the scene's colour, where it showed the
      previous scene's colours before.

## Alternatives Considered

- **Sleeping before every read-back.** Written first, and a test rejected it:
  120ms on every device on every apply to cover one device being slow to
  answer.
- **Sleeping between the two mode packets**, which also worked on the
  hardware. The measurement showed the pause is only needed before the read,
  so this would be a delay in the write path for nothing.
- **Trusting the accepted write and not reading back.** Read-back is how
  hotaru catches a device that takes a mode without honouring it, which is a
  real case on this hardware (spec 001). Giving it up to fix a slow reporter
  trades a rare wrong answer for a common one.

## Risks & Assumptions

- **One extra read, and one pause, per one-colour mode that answers slowly.**
  Nothing pays it unless it disagrees.
- **A device that genuinely refuses a mode now costs a pause before it falls
  through.** It fell through immediately before. The mode is still rejected,
  one `settleDelay` later.
- **The measurements are from one keyboard.** The behaviour is a firmware's,
  and another device may report instantly or take longer than one pause. The
  shape of the fix -- ask again rather than assume -- is what generalises.
- **Rollback** is a revert. Nothing is stored differently.

## Executive Summary

An effect's colour was lost arriving from an all-lit scene, through two failed
fixes. Driving the keyboard from OpenRGB's own CLI showed the mode packet must
arrive twice with nothing between: hotaru wrote the frame between its two
packets, which put the device back in the arriving state. Moving the frame
ahead of both fixed the colour and exposed a second fault -- the device
reports its old mode if asked immediately, so hotaru concluded the mode had
not taken and fell through to Direct. It now asks once more, and only when the
first answer disagrees.

Reviewers should look at `writeFrame` in `internal/service/service.go`: the
ordering and the re-read are both there, and the tests pin the packet order
for both kinds of mode.
