# Spec 050: an effect is not a preference

**Issue**: [#148](https://github.com/ushineko/hotaru/issues/148)

## Status: COMPLETE

## Context

A scene carries a colour per light and a mode per device: spec 015 put the
mode in the model, spec 038 put it in the window. A scene that names an effect
for a device and also gives that device a colour per LED lit the colours and
dropped the effect, and said nothing about having done so.

```
$ hotaru scene apply custom1
custom1: 6 of 6 device(s) lit.
$ hotaru light list | grep Keychron
Keychron K4 HE   100   Direct   yes   Direct, Solid Color, ...
```

`custom1` names `Solid Reactive Multinexus` over about a hundred per-key
colours. The keyboard stayed in Direct, the scene kept the effect, and the
editor kept showing it -- so the one place the truth could be seen was the
keyboard itself.

### The frame was read first

Resolution asks what the frame needs before it looks at what was asked for. A
frame of more than one colour needs a mode that takes a colour per LED, and
`SolidCandidates` drops every mode that does not; `writeFrame` then dropped a
preferred mode for the same reason, in the branch that carries on "rather than
showing one colour and reporting success".

That rule is right for `Mode`. `--mode` and the wizard set a preference about
*how to show these colours*, and a mode that cannot show them is no use: spec
009's whole point is that a device showing one colour where three were asked
for is a worse answer than a mode nobody named.

It is wrong for an effect. An effect is a decision about the device -- the
keyboard ripples under typing -- and the colours are what it ripples in. There
is nothing to fall back to, because the thing being asked for is the mode.

### The frame is what gives way

The effect is written, and the frame reduces to **the colour most of it is**.

A mode that keeps its colour in the mode has to be given one or it shows what
its vendor left behind, which is spec 009 again: a Kraken put into Static
displayed NZXT's red while every read-back held purple. Until now the colour
was only sent for a frame that had a single colour to give. A frame of a
hundred has an answer too, and it is the one most of the frame is: a keyboard
lit blue with a handful of amber keys is a blue keyboard.

Per-key detail is lost on that device, and it was never showable -- the mode
takes one colour. What is gained is the effect the person picked, in a colour
they picked, instead of Direct and silence.

**The fall-through stays.** The effect is tried first; a device that accepts
it and does not honour it still lands on Direct with its colours, which is the
behaviour every other write has.

### Reconciling had the same hole

Re-assertion restores the mode as well as the colours, and resolved it the
same way -- so an effect that did survive an apply would come back from a
reconcile sitting in Direct. The mode desired state holds is one the device
took and hotaru recorded; a frame it cannot show every colour of is not
grounds to resolve past it.

## Requirements

**R1. An effect a scene names is written**, whether or not the scene also
gives that device a colour per LED.

**R2. An effect that takes one colour is given the frame's dominant colour**,
so the effect looks like the scene rather than like the vendor's leftovers.

**R3. Re-assertion puts the effect back**, under the same rule.

**R4. `Mode` is unchanged**: a mode that cannot carry the frame is still
resolved past, because it is a preference about the colours rather than a
decision about the device.

**R5. An effect the device does not have still costs the effect and not the
scene**, as spec 038 has it.

## Acceptance Criteria

- [x] AC1. A scene with a colour per LED and an effect on the same device
      applies with the device in that effect's mode
      (`TestAnEffectSurvivesAFrameItCannotShow`).
- [x] AC2. The mode is set with the frame's most common colour
      (`TestAnEffectIsLitInTheColourTheSceneMostlyIs`).
- [x] AC3. `Frame.Dominant` answers the most common colour, ties to the first,
      and says no for an empty frame (three tests in `internal/devices`).
- [x] AC4. A reconcile over such a device puts the effect back, not Direct
      (`TestAnEffectIsPutBackOverAFrameItCannotShow`).
- [x] AC5. `Request.Mode` over a frame it cannot show still resolves past it
      (`TestAModeIsStillNotForcedOverAFrameItCannotShow`).
- [x] AC6. Verified on the development machine: `hotaru scene apply custom1`
      leaves the Keychron in `Solid Reactive Multinexus`, a reconcile leaves
      it there, and `custom2` -- which names no keyboard effect -- returns it
      to Direct with its per-key colours.

## Alternatives Considered

- **Report it and keep the colours.** Honest and does not do what the scene
  says. The effect is in the file, in the list and in the editor; the one
  thing missing was it happening.
- **Send no colour and let the effect keep its own.** Simplest, and the
  scene's colours would then have no bearing on that device at all -- a
  keyboard lit in whatever the vendor left is not a scene.
- **Refuse to save a scene that asks for both.** The combination is not a
  mistake: per-key colours are what the device falls back to, and a person
  switching the effect off should find their colours still there.

## Risks & Assumptions

- **A device lit per-LED loses that detail when a scene names a one-colour
  effect.** Deliberate, and the alternative is the effect not happening.
  Confined to devices a scene names an effect for; no other device changes.
- **The dominant colour is a count, not a perceptual judgement.** A frame that
  is half one colour and half another picks the first of the two. The tie rule
  is fixed so the reduction does not depend on map order.
- **The frame is still written after a one-colour mode is set**, as it always
  was: such a mode does not read the buffer, and the write is what commits the
  frame on an NZXT (spec 010). Verified not to knock the Keychron out of its
  effect.
- **Rollback** is a revert. No file format changes, and scenes written before
  or after this are the same scenes.
