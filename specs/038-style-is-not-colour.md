# Spec 038: style is not colour

**Issue**: [#94](https://github.com/ushineko/hotaru/issues/94)

## Status: COMPLETE

## Context

A scene has carried two things since spec 015: a colour per light, and a mode
per device -- what that device does with the colours once it has them. The
model said so, the CLI said so (`hotaru scene write --effect`), and the window
said nothing. The only place a mode was ever asked about was the mapping
wizard, which asks once about a machine rather than every time about a scene.

So a keyboard that should ripple under typing was a scene somebody had to
write at a terminal, and the parity rule this program holds everywhere else
was backwards: the window could do less than the CLI.

### What the device is doing now

"Leave it alone" is not an answer until you know what it leaves. The chooser
says what the scene sets; the machine is where the mode actually lives, and a
keyboard sitting in a reactive mode looks identical in the editor to one
sitting in Direct. The difference is what the scene looks like when it is
applied, so each row says `now: <mode>`.

### A style spreads

Somebody who decides their keyboard should be reactive has decided that about
the keyboard, not about one scene -- and a bank of nine scenes meant opening
nine. `hotaru scene style <from> <to>...` gives other scenes one scene's
effects, and the window offers the same from the editor.

**It replaces rather than merges.** A target keeping an effect for a device
the source says nothing about would make "these scenes now look like that one"
true of some devices and not others. Copying a scene with no effects is
therefore how a bank goes back to plain colours.

**And it is all or nothing**: every target is read before any is written, so a
name that is not there costs nothing rather than leaving half a bank restyled.

### A picture says nothing about modes

`hotaru image scene` and the window's "Make a scene" build a scene from a
picture's colours. A picture answers "what colour is each light" and nothing
about what a device should be doing with it, so the effect is the caller's:
`--effect`, and the same chooser in the dialog. Nothing is the default, which
is every device showing the colours it was given.

Recolouring keeps whatever effects the scene has. The separation slider is
about colour, and dragging it must not quietly undo a mode.

### It did not fit

The first version put the chooser in the editor's fixed controls -- the ones
that deliberately do not scroll away. Five devices with a mode and a line each
is three hundred pixels of a pane whose job is to stay out of the way: the
editor's minimum height went to 730 against a 760-pixel default window, and
the card fell off the bottom. It was built, it was correct, and it could not
be seen.

It is a card that says what is set and a dialog that sets it, which is the
shape of the screen chooser beside it. The editor's minimum is 370 now, and a
test holds it under what a section actually gets.

### And the rows are a table

Six label-and-control pairs are six left edges. The chooser column starts in
the same place on every row now, with the device on the left and what it is
doing on the right, under a heading.

## Requirements

**R1. The editor sets a mode per device**, for the devices that have more than
one and are in scope.

**R2. Each row says what that device is doing now.**

**R3. A scene's effects can be given to other scenes**, from the window and
from the CLI, replacing rather than merging.

**R4. A scene built from a picture takes effects too**, with nothing as the
default.

**R5. Recolouring keeps them.**

**R6. The editor still fits a window.**

## Acceptance Criteria

- [x] AC1. The chooser offers every mode a device advertises, defaults to
      leaving it alone, and reaches the draft.
- [x] AC2. A device with one mode, or out of scope, is not offered one.
- [x] AC3. Each row says the device's current mode, or that it does not say.
- [x] AC4. `hotaru scene style` and the window's copy give the named scenes
      exactly the source's effects and leave their colours alone.
- [x] AC5. A wrong name changes nothing at all.
- [x] AC6. `hotaru image scene --effect` and the dialog's chooser reach the
      saved scene; without them it has none.
- [x] AC7. Recolouring a scene keeps its effects.
- [x] AC8. The editor's minimum height fits a default window.
- [x] AC9. The chooser's columns line up, asserted against the shape it
      replaces.
- [x] AC10. Verified on the development machine, on a Keychron with 23 modes.

## Risks & Assumptions

- **A mode is the device's own spelling**, passed through to OpenRGB as the
  scene already did. A mode a device stops advertising is a scene that no
  longer applies cleanly, which is the same risk `--effect` has always had.
- **Copying replaces**, which is destructive to a target's own effects. The
  dialog says so; the CLI's help says so.
- **The offer needs a saved scene**, because the service copies what a scene
  holds. A draft that was never saved is told to save first rather than
  saved silently.
- **Rollback** is a revert. A scene with no effects is what every scene had
  before this.
