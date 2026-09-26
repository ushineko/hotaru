# Spec 055: the buffer agrees with the mode

**Issue**: [#159](https://github.com/ushineko/hotaru/issues/159), superseded in reasoning by [#160](https://github.com/ushineko/hotaru/issues/160)

## Status: COMPLETE

## Context

> **Note**: this spec was first written as "the buffer is what it shows", on
> the reading that a keyboard displays its buffer rather than its mode's
> colour. The hardware disproved that: the device later read back with buffer
> and mode colour both correct while still showing the wrong thing. The change
> below survived; the reasoning for it did not. Spec 056 is what was actually
> wrong. This is kept rather than deleted because the record of a wrong
> reading is worth as much as the fix.

A mode that shows one colour of its own is sent a frame of that colour, rather
than the per-key colours the scene carries for everything else.

Two things recommend it, neither of which is "the buffer is what it shows":

- **The two cannot disagree.** The mode holds `#67798c` and so does every LED,
  so nothing downstream -- a read-back, a reassert, another program looking at
  the device -- can see a device that is half one colour and half another.
- **It was part of the sequence confirmed on the hardware.** The probe that
  settled spec 056 wrote a uniform frame of the effect's colour before the two
  mode packets. Keeping the frame's content as measured is better than
  changing one more variable after the fact.

**The scene's own colours are not lost.** They are what goes into desired
state, so they are there for the moment the effect comes off. What changes is
only what is put in the device's buffer while a mode that cannot display them
is running. A per-LED mode is untouched: the buffer is what it shows, and the
scene's colours are what go in it.

## Requirements

**R1. A mode that shows one colour of its own is sent a frame of that colour.**

**R2. A per-LED mode is sent the scene's colours**, unchanged.

**R3. What is remembered is the scene's colours**, not what the buffer was
given, so the lights go back to what somebody asked for when the effect comes
off.

**R4. The read-back check is against what was sent**, which is now a different
frame for these modes.

## Acceptance Criteria

- [x] AC1. A scene whose effect names a colour sends that colour to every LED
      (`TestAModeThatShowsOneColourIsSentABufferOfThatColour`).
- [x] AC2. A per-LED mode still gets the scene's own colours
      (`TestAPerLEDModeStillGetsTheScenesOwnColours`).
- [x] AC3. Desired state keeps the scene's colours and the effect's colour
      separately (`TestTheSceneKeepsItsColoursWhileTheEffectShowsItsOwn`).
- [x] AC4. Measured on the development machine: the device reads back with
      mode colour and buffer both `#67798c`, where the buffer held the
      previous scene's colours before.

## Alternatives Considered

- **Leaving the scene's colours in the buffer**, which is what hotaru did
  before. It leaves the device holding two different answers about what it is
  showing, and it was not what the confirmed sequence wrote.
- **Not writing a frame at all for such a mode.** Spec 052 considered and
  rejected this; spec 056 found the frame's *position* was the problem rather
  than its presence, so there is no longer a reason to drop it.

## Risks & Assumptions

- **The device's buffer no longer holds the scene's colours while an effect is
  running.** Anything reading the buffer back to find out what a scene was
  showing would see the effect's colour. Nothing does: desired state is what
  hotaru reads, and it keeps both.
- **An effect with no colour of its own is unchanged.** There is nothing to
  agree with, so spec 050's fallback still decides and the frame is still the
  scene's.
- **This did not fix the reported symptom on its own**, and was installed and
  tested while the symptom persisted. See spec 056.
- **Rollback** is a revert. Nothing is stored differently.

## Executive Summary

A mode that shows one colour of its own is now sent a frame of that colour, so
the device's buffer and its mode colour cannot disagree. First written on a
reading of the hardware that later proved wrong; kept because the change is
sound on its own terms and was part of the sequence spec 056 confirmed.
