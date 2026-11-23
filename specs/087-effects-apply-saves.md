# Spec 087: the effects dialog applies and saves

**Issue**: [#242](https://github.com/ushineko/hotaru/issues/242)

## Status: COMPLETE

## Executive Summary

The Effects dialog had a live preview on every pick and two saves after it,
so a change could look applied and be lost. Picks now change the draft only;
Preview shows it until dismissed, and Apply saves the scene and puts it on the
machine. Reviewers should look first at `chooseEffects` and `applyDraft` in
`internal/gui/scenes.go`, and at the test this withdraws from spec 077.

## Context

A scene's effects are set in the editor's Effects dialog. Until now each pick
went to the hardware at once as a preview (spec 077). "Done" kept the picks in
the draft and ended the preview, so the lights went back to the saved scene.
The change was kept only after Save in the editor and Save again in the name
form. A banner then said the scene was saved, and had to be dismissed.

On the cachyos test machine the maintainer set the four Dominator sticks to
Candle, then the other devices. The sticks showed Candle during the dialog.
The stored `landscape1` ended up with the sticks on Breathing and every other
device on Candle, and applying it drew that. A GUI test with that machine's
device listing kept Candle on all four sticks through the same picks. The
likely cause is an edit that never reached Save. The live preview made it look
applied.

The maintainer's decisions (#242):

- One mechanism: applying a change saves it.
- Applying puts the scene on the machine, since somebody who makes a change
  wants to see it at once.
- A new scene asks for its name.
- Picks in the dialog no longer go to the hardware. A Preview button shows the
  draft and puts the lights back when dismissed.
- The saved banner goes.

## Requirements

- **R1** Picking a mode, a colour or a speed in the Effects dialog changes the
  draft only. Nothing is sent to the service.
- **R2** The dialog has three buttons: Cancel, Preview and Apply.
  - **R2.1** Cancel puts the draft's effects back as they were when the dialog
    opened, and sends nothing.
  - **R2.2** Preview shows the whole draft on the hardware, as "Show it on the
    hardware" in the editor does. Dismissing it puts the lights back. Nothing
    is recorded. The Effects dialog stays open behind it.
  - **R2.3** Apply saves the draft and puts it on the machine, then closes the
    dialog. The editor stays open on the saved scene.
- **R3** Apply on a scene opened from a saved one saves it under that name.
  - **R3.1** When it is the scene the machine shows, the save lights it again
    (`Service.relight`), and the window does not apply it a second time.
  - **R3.2** Otherwise the window applies it after the save.
- **R4** Apply on a draft with no name asks for one first, in the form Save
  uses. Cancelling the form saves and applies nothing, and the dialog stays
  open.
- **R5** Apply shows no banner on success. A failure is reported as any
  operation's failure is.
- **R6** The editor's own Save is unchanged.
- **R7** No new route: the window uses `SaveScene` and `ApplyScene`, which the
  CLI already reaches (`hotaru scene write`, `hotaru scene apply`).

## Acceptance Criteria

- [x] Picking a mode in the dialog sends nothing to the service.
- [x] Cancel leaves the draft's effects as they were when the dialog opened.
- [x] Preview sends the draft as a preview, and dismissing it releases the
  preview.
- [x] Apply on a saved scene the machine is not showing saves it under its
  name, then applies it.
- [x] Apply on the scene the machine shows saves it and does not apply it
  again.
- [x] Apply on a new draft asks for a name, then saves and applies under it.
- [x] Apply shows no banner on success.
- [x] `make test`, the GUI tests (`-tags migrated_fynedo`) and `make lint`
  pass.
- [x] Live, on cachyos with the maintainer watching: set the sticks and the
  other devices to Candle, Preview, dismiss, Apply. The lights stay on Candle,
  and `landscape1` in `scenes.yml` has Candle for every device.
- [x] The README changelog says so under `### Unreleased`.

## Risks & Assumptions

- **Spec 077's "shown as it is chosen" is withdrawn.** It answered a live run
  where an effect seemed to do nothing until saved. Preview and Apply answer
  it now, and a pick no longer looks applied when it is not.
- **Apply writes the scene file.** Somebody who wanted to try an effect without
  keeping it uses Preview.
- **Whether the machine shows the scene** is read from the window's last
  snapshot. A stale snapshot can apply the scene once more than needed, which
  starts its effects again. That costs one restart, not a wrong state.
- **The editor's Save keeps its banner.** It closes the editor, and the banner
  is the only sign that the save happened. The banner the maintainer named is
  the one after a change to effects.
- **Rollback**: revert the merge. No stored format changes.

## Verification

- `effectsapply_test.go`: a pick sends nothing; Cancel puts the effects back;
  Preview shows the draft and dismissing it releases the preview; Apply saves
  then applies a saved scene, saves and does not apply the scene the machine
  shows, asks a new scene for its name, and leaves no banner.
- `make test`, `go test -race -tags migrated_fynedo ./internal/gui/`,
  `make lint` (0 issues) and `govulncheck` (none) pass.
- Live, 2026-11-23, on cachyos (service 2.3.3, the window from this branch),
  with the maintainer at the machine. Picks in the dialog left the lights
  alone. Preview showed them, and dismissing it put the lights back. Apply
  switched the lights and they stayed: "it works well." Afterwards
  `landscape1` in `scenes.yml` held Candle for the four sticks, the GPU, the
  dock, the mouse and both boards, and Water Cooler for the keyboard, and
  `hotaru light list` drew the same.
