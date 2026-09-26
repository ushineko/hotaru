# Spec 054: one door for a scene

**Issue**: [#158](https://github.com/ushineko/hotaru/issues/158)

## Status: INCOMPLETE -- AC5 is a look at the machine, and is the author's

## Context

Spec 052 made an effect's colour survive arriving from a per-LED mode: the
mode packet goes again after the frame, so the second one is the
already-in-the-mode case. It was reported working and then reported broken
again, from the window, moving between scenes in the editor's section.

The report came with a diagnosis: the fix went into one place and the other
places a mode could be set were never audited. Half right, and the half that
is wrong is worth writing down, because it is what made this hard to see.

### There is only one place a mode is written

Every mode packet in the lighting path comes from `writeFrame`, which is the
only caller of `SetMode` outside the hardware prober. The window and the
terminal both reach it the same way:

```
ApplyScene ─┐
            ├─> light() ─> Apply ─> applyOne ─> writeFrame ─> SetMode
Preview   ──┘
```

So spec 052's second packet is sent for every scene applied, from either
shell. Nothing about it is duplicated, and auditing for a second copy of it
finds nothing.

### There were two ways for a scene to become lights

The duplication was a layer above, and of a different thing. An editor showing
a draft on the hardware takes a preview lease once and then says "the draft is
this now" on every change -- re-taking the lease would release it first, and a
release puts the lights back, so every keystroke came with the previous
colours flashing ahead of it.

That second call did not send the scene. It built a request out of the scene's
colours and sent it to `POST /v1/lighting/apply`, and **that request has no
effects field at all**:

| | carries colours | carries effects |
|---|---|---|
| `POST /v1/preview` (a scene) | yes | yes |
| `POST /v1/lighting/apply` (the wizard's route) | yes | **no** |

So a draft went up with its effect, and the first change to it took the
keyboard out of the effect and into Direct -- invisibly, because a keyboard in
Direct lit in the scene's colours looks like a keyboard doing what it was
told. The effect only came back when something applied a saved scene.

**Which is the all-lit-to-preset transition, arrived at from the window.** An
editor that has been open has left the keyboard in a per-LED mode, so the next
preset scene is spec 052's case every time -- and spec 052's own verification
went preset-to-preset from the terminal, where nothing ever took that route.

### The rule this puts back

A scene is a scene. What reaches the hardware cannot depend on whether it
arrived from a list, a keypress or an editor, and the way to keep that true is
for there to be one function that turns a scene into lights and no way to
write one without it. `light()` is that function; the editor's update now goes
through it like everything else.

`POST /v1/lighting/apply` keeps its shape and its job: colours on devices, for
`hotaru light set` and the mapping wizard, which are about colours and not
about scenes. Adding effects to it would be a second way to apply a scene,
which is the thing being removed.

## Requirements

**R1. A change to a draft goes through the same path a saved scene does**, so
a scene's effects reach the hardware however the scene arrived.

**R2. It does not take the lease again.** Releasing and re-taking flashes the
previous colours back, which is what the plain-apply shortcut was avoiding and
is not a reason to have a second path.

**R3. A draft that grows a device takes it into the lease**, because an editor
that adds the keyboard to a scene means the keyboard, and a device outside the
lease is one the reconciler is free to correct under the person editing it.

**R4. A device somebody else is showing is refused**, whether the claim
arrives as a new lease or as a draft that grew into one.

**R5. `POST /v1/lighting/apply` is unchanged.** It is the colours route, and
giving it effects would restore the second door.

## Acceptance Criteria

- [x] AC1. Changing a draft sends the scene, to the route that carries
      effects, naming the lease rather than taking a second one
      (`TestChangingADraftSendsTheSceneAndNotJustItsColours`). Verified to
      fail against the previous code.
- [x] AC2. A changed draft leaves the keyboard in its effect, in the effect's
      own colour (`TestChangingADraftKeepsItsEffect`).
- [x] AC3. An edited draft and a saved scene write the same modes to the same
      device (`TestADraftAndASavedSceneWriteTheSameThing`). The rule itself,
      asserted as the two paths agreeing rather than as a copy of what one of
      them writes today.
- [x] AC4. A draft that grows a device takes it into the lease
      (`TestADraftThatGrowsADeviceTakesItIntoTheLease`), a lapsed token is
      refused (`TestRedraftingWithoutALeaseIsRefused`), and a device held by
      somebody else is refused
      (`TestADraftCannotTakeADeviceSomebodyElseIsShowing`).
- [ ] AC5. Verified on the development machine: open a scene in the editor,
      change something, close it, then apply a preset scene -- the keyboard
      shows the effect's colour. This is the case that was reported, and the
      only one that can settle it, because no read-back distinguishes the two
      states (spec 052).

## Alternatives Considered

- **Adding an effects field to `ApplyRequest`.** The smallest diff and the
  wrong one: it would leave two routes that can apply a scene and make the
  next divergence a matter of remembering to set a field in both.
- **A route of its own for changing a draft.** `POST /v1/preview` with a token
  is the same sentence -- this is what the preview shows -- and a second route
  is a second place for a scene to become lights, which is the thing this spec
  removes.
- **Releasing and re-taking the lease per change**, which is what the code did
  before the shortcut existed. It flashes the previous colours back on every
  keystroke; that was a real fix and the shortcut it took is what this
  replaces.

## Risks & Assumptions

- **A change to a draft now costs what an apply costs**, because it is one:
  the screen state is resolved too, where the draft names one. Applying a
  scene is already what the button beside it does, and the editor writes on a
  debounce rather than per keystroke.
- **AC5 is the only thing that can confirm the reported symptom.** Every field
  hotaru can read said the write had worked while the keyboard disagreed,
  which is spec 052's finding and is unchanged by this.
- **The prober still sets modes of its own**, deliberately: it asks the
  hardware what a mode does and puts it back. It is not a path a scene takes
  and is not consolidated here.
- **Rollback** is a revert. Nothing is stored differently, and a client that
  sends no token gets exactly the behaviour it got before.

## Executive Summary

To be written before the pull request.
