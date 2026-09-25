# Spec 051: an effect has a colour of its own

**Issue**: [#149](https://github.com/ushineko/hotaru/issues/149)

## Status: COMPLETE

## Context

Spec 050 stopped an effect being dropped by a scene that also gives its device
a colour per LED, and lit the effect in the colour most of the frame is. That
is a reduction, and a reduction is not a choice. The keyboard's reactive modes
say exactly what they can take:

```
Solid Reactive Multinexus   flags=0x141  colours min=1 max=1  speed 0-255 (127)
```

One colour, and a speed. Neither is sayable.

### The window promises what the mode cannot do

The editor draws every light of every device as something to aim at, which is
spec 025's whole point and is right for Direct. It is wrong for a mode that
shows one colour: the picture offers a hundred targets and the device will
show one of them, so a person paints a keyboard and looks at a keyboard that
ignored them. A control that does nothing is the fault the editor already
refuses to commit for a device out of scope.

### What a scene says

```yaml
effects:
  Keychron K4 HE:
    mode: Solid Reactive Multinexus
    colour: '#0000ff'
    speed: 127
```

**The short form still means what it meant.** `Keychron K4 HE: Solid Reactive
Multinexus` is a mode and nothing else, and nothing else is what most scenes
want: spec 050's dominant colour remains the answer when no colour is named.
Every scene on disk is read unchanged, and one that names no colour is written
back in the short form -- a file somebody can still read.

### The per-key colours stay

A device under a one-colour effect keeps its assignments. They are what the
device falls back to the moment the effect is turned off, and losing a hundred
keys to a mode change somebody is trying out is not a trade the editor gets to
make on their behalf. The editor stops *showing* them while the effect is on;
the file keeps them.

### Which colour the picture picks

`hotaru image scene` and "Make a scene" answer "what colour is each light"
from a picture. A device in a one-colour effect cannot use that answer, so it
gets one representative colour instead -- the dominant of what the picture
gave that device, which is the same reduction spec 050 applies at the write
and keeps the two paths saying the same thing.

### A snapshot that dropped what it did not know about

Found on the way: desired state's `copyOf` built each device by listing its
fields, so the mode's colour and speed were remembered, copied out without
them, and re-asserted from the frame. A field added above a field silently
dropped in every snapshot is the kind of fault that survives review, so the
copy is the struct with its slice replaced now, and a test holds it.

### The chooser typed it first

The first version of the effects dialog took the colour as a hex field, on the
grounds that a modal over a modal was a stack nobody asked for. That is a
reason to look at the stack, not a reason to make somebody spell `#0000ff`:
the argument for a window at all is that a colour is *looked at*, and a control
that is a text box in the dialog people reach first and a wheel in the editor
is two ways to do one thing with the worse one in front. The wheel opens over
the chooser, which is what overlays are for.

It does not preview on the hardware, and neither does anything else in that
dialog: choosing a mode there does not light it either. "Show it on the
hardware" is the one control that does, for the scene as a whole, when somebody
asks -- and saving applies it. The editor's own card keeps its live preview
because that is what the card has always done.

## Requirements

**R1. A scene can name a colour and a speed for a device's effect**, in the
file, the CLI and the API.

**R2. The short form is unchanged**: a scene naming only a mode reads, writes
and applies as it does today, with the dominant colour as the fallback.

**R3. A named colour wins over the dominant one.**

**R4. A speed is written where the mode advertises one**, and left alone where
it does not.

**R5. The editor offers one colour for a device under a one-colour effect**,
in place of its zones and lights, and a speed where the mode has one. The
effects chooser offers the same colour, picked the same way.

**R6. The scene keeps that device's assignments** while the effect is on, and
shows them again when it is turned off.

**R7. "Make a scene" gives such a device one representative colour.**

**R8. The API says which modes take one colour and which take a speed**, so a
client does not have to know about hardware to ask.

## Acceptance Criteria

- [x] AC1. A scene file written before this reads unchanged, and a scene that
      names only a mode is written back in the short form.
- [x] AC2. A scene naming an effect colour lights the effect in it, asserted
      on the mode write rather than on the frame.
- [x] AC3. A scene naming no effect colour still uses the dominant one.
- [x] AC4. A speed is written for a mode that advertises one and omitted for a
      mode that does not.
- [x] AC5. `hotaru scene write` can set an effect's colour and speed, and
      `hotaru scene list` says them.
- [x] AC6. The API's device listing names the modes that take one colour and
      the modes that take a speed.
- [x] AC7. The editor draws one colour control for a device under a one-colour
      effect and its zones for every other device, and the effects chooser
      opens the wheel for it rather than a field to type a value into.
- [x] AC8. Turning the effect off puts that device's per-LED picture back with
      its colours intact.
- [x] AC9. A scene built from a picture gives a device in a one-colour effect
      one representative colour.
- [x] AC10. Verified on the development machine: the keyboard's reactive
      effect lit in a colour the scene names, at a speed it names.

## Alternatives Considered

- **A second map beside `Effects`.** Two maps keyed by device that must be
  kept in step, and a scene file where a device's effect is in three places.
- **Repainting the device's assignments to the chosen colour.** It makes the
  scene say one thing, and it destroys the per-key work the moment somebody
  tries a mode out.
- **A hex field in the effects chooser.** Built, and wrong for the reason
  above: the window exists so a colour can be seen.
- **Leaving the editor alone.** The controls would keep promising colours the
  device cannot show, which is the half of this issue that is actually
  visible.

## Risks & Assumptions

- **The scene file grows a nested form.** Read compatibility is the risk that
  matters and is covered by AC1; the short form stays the written form
  wherever it is sufficient.
- **`Effects` changes type across the API.** A breaking change by the letter
  of the package doc. The clients are hotaru's own CLI and window, both in
  this repository and both updated here.
- **Speed is written blind.** Nothing reads back what a speed looks like, so
  the read-back that guards mode and colour does not cover it. It is stored,
  sent where advertised, and looked at by a person.
- **Rollback** is a revert, plus rewriting any scene that has been saved with
  a nested effect. Files written before this are untouched by it.
