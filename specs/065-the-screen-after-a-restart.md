# Spec 065: the screen after a restart

**Issue**: [#184](https://github.com/ushineko/hotaru/issues/184)

## Status: COMPLETE

## Executive Summary

After `hotaru serve` restarted, the lights came back and the cooler's panel
did not. A scene that had put a picture or the readout on the panel left the
dashboard drawing. The window's System card showed an empty Scene and Screen
until something changed them. hotaru now records what the panel was asked to
show, in the state file beside the lights, and puts it back when the cooler
attaches. The scene label is kept the same way (spec 064 R5).

Look first at `Service.Draw` in `internal/service/service.go`. Every change of
the panel is recorded there, once it has worked. Then look at
`Store.RecordScreen` in `internal/state/state.go`, for when a picture's bytes
are copied.

## Context

The System card reads `Status.Scene` and `Status.Showing`. Both lived in
`Service` memory. The panel's state was recorded nowhere, so a restart had
nothing to put back, and the cooler attached with the dashboard drawing.

The label was also incomplete. `screen()` set it for a scene's picture and
readout, and `UseDashboard` set it for a dashboard. `ShowImage` and
`POST /v1/screen` changed the panel without setting it, so Screen could name
something the panel was no longer showing.

## Requirements

- **R1** The state file records what the panel was last asked to show: the
  label, and either the readout or a picture file. No record means the
  dashboard.
  - **R1.1** A picture with a file of its own (a stored picture, a scene's
    screen) is recorded by its path, and its bytes are not copied.
  - **R1.2** A picture sent as bytes (`POST /v1/screen`) is written next to
    the state file as `screen.gif`, and recorded by that path.
  - **R1.3** The record is written only once the panel has accepted the
    change.
- **R2** Every route that changes the panel updates the record and the
  label: a scene's screen, `UseDashboard`, `ShowImage`, `POST /v1/screen`
  with a picture, the readout or the dashboard. Brightness and orientation
  are the device's own settings, and the device keeps them.
- **R3** A service that starts reads the label back before any cooler
  attaches.
- **R4** When a cooler attaches, hotaru puts back what was recorded. A
  picture whose file has gone is reported in the journal, and the dashboard
  stays.
- **R5** A fresh install records nothing and puts nothing on the panel.
- **R6** The state file still loads when it holds a scene or a screen and no
  devices.
- **R7** `hotaru status` prints the scene and the screen, as the window's
  System card does. It printed neither, which broke CLI and window parity.

## Acceptance Criteria

- [x] A scene's picture is on the panel after a restart, and Scene and
      Screen are read back (`TestAScenesPictureIsOnTheScreenAfterARestart`).
- [x] A picture sent as bytes is put back (`TestAPictureSentAsBytesIsPutBack`).
- [x] The readout is put back (`TestTheReadoutIsPutBack`).
- [x] A dashboard chosen after a picture stays the dashboard
      (`TestTheDashboardAfterAPictureStaysTheDashboard`,
      `TestASceneThatAsksForTheDashboardReplacesThePicture`).
- [x] Showing a stored picture sets Screen, and it is put back
      (`TestShowingAStoredPictureSaysWhatIsShowing`).
- [x] A fresh install puts nothing on the panel
      (`TestAFreshInstallPutsNothingOnTheScreen`).
- [x] `hotaru status` names the scene and the screen
      (`TestStatusSaysTheSceneAndTheScreen`).
- [x] `make test`, the GUI tests and `make lint` pass.
- [x] Live, with someone watching: apply a scene with a picture, run
      `systemctl --user restart hotaru`, and the picture is back on the
      panel. System shows that scene and `picture: <name>`.

## Risks & Assumptions

- **One dashboard frame before a restored picture.** The dashboard loop draws
  its first frame before it reads the hold. A restored picture replaces it
  about a frame later. The alternative is starting the loop held, which
  would need a second way to start it.
- **`screen.gif` sits in the state directory.** The packaged unit can write
  there under `ProtectHome=read-only`, because `~/.local/state/hotaru` is in
  its `ReadWritePaths`. One file, replaced each time; only a picture sent as
  bytes writes it.
- **The label is still a label.** Something that drew on the panel without
  hotaru (another program, the firmware after a power cycle with hotaru
  stopped) leaves it stale until hotaru draws again.
- **Restoring the readout or a picture writes to the cooler at attach.**
  That is one write, the same cost as restoring the lights. A recorded
  dashboard costs nothing more than today.
- **Rollback**: revert the merge. An older build ignores the `screen` key in
  the state file.

## Alternatives Considered

- Derive the screen from the applied scene at start. Rejected: a dashboard
  chosen or a picture shown after the scene would be lost.
- Copy every picture into the state directory. Rejected: a hotkey that
  switches scenes would write megabytes each time.

## Verification

Tests: the criteria above, each named. Each fix was checked against a
mutation that undoes it:

- Not reading the label back in `SetRecorder` fails the picture and readout
  tests.
- Skipping the picture branch of `RestoreScreen` fails all three picture
  tests.
- Not keeping the bytes of a picture with no file fails
  `TestAPictureSentAsBytesIsPutBack`.
- Not recording the dashboard in `Draw` fails
  `TestASceneThatAsksForTheDashboardReplacesThePicture`.

`make test`, the GUI tests and `make lint` pass.

Live, with the development build as the user's service, `nebula` (a scene
with a picture) was applied. The state file then held:

```yaml
  scene: nebula
  screen:
    picture: /home/<user>/.local/share/hotaru/images/nebula.gif
    showing: 'picture: nebula'
```

After `systemctl --user restart hotaru`, the journal showed the cooler
attaching and no "could not put the screen back". `/v1/status` read
`showing= picture: nebula`, and `hotaru status` printed:

```
  scene     nebula, the last applied
  screen    picture: nebula
```

The maintainer confirmed the panel showed the picture after the restart, not
the dashboard.
