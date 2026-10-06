# Spec 064: colours from the scenes editor

**Issue**: [#183](https://github.com/ushineko/hotaru/issues/183)

## Status: COMPLETE

## Executive Summary

Colouring a Corsair MM700 from the Scenes editor showed three faults, and none
is specific to the mat. A whole-device colour was hidden by the per-light lines
already in the scene, because a draft wrote its lines out sorted. An edit took
a save and then an apply, because the service forgot the applied scene when it
restarted. And Make a scene gave the mat's three one-light zones the same
average colour.

The draft now keeps its lines in the order they were set. A whole device or a
whole zone replaces the lines inside it. The editor draws each light in the
colour that covers it. The state file keeps the applied scene's name. Make a
scene treats a device's one-light zones as one strip.

Look first at `Draft.Set` and `inside` in `internal/gui/draft.go`, for what
counts as "inside" a target. Then look at `paintDevice` in
`internal/service/scenefromimage.go`.

## Context

Reported on the window: on the MM700, "the whole device" did not seem to work.
Each light had to be picked instead. And a saved edit showed only after the
scene was applied again from Scenes.

The MM700 is three zones of one light each: Left, Right and Logo. Every scene
made from a picture gives it one line per light:

```yaml
- colour: '#925841'
  target: Corsair MM700/Left[0]
- colour: '#925841'
  target: Corsair MM700/Right[0]
- colour: '#925841'
  target: Corsair MM700/Logo[0]
```

1. **The whole device was applied first.** `Draft` held its lines in a map
   and `Draft.Scene` wrote them sorted. `Corsair MM700` sorts ahead of
   `Corsair MM700/Left[0]`. The service applies lines in order, so the
   per-light lines painted over the whole-device colour. Any overlap resolved
   by the alphabet, not by the newest edit.
2. **The editor did not show it either.** A light block looked up the draft
   by its own exact target, `Zone[a:b]`. A whole-device colour, a whole-zone
   colour, and a light written `Zone[5]` (the form Make a scene writes) were
   not found. The block kept the hardware's colour.
3. **Editing a `Zone[5]` line coloured the whole zone.** `spotOf` read only
   `[a:b]`, and fell back to the zone.
4. **The applied scene was forgotten on restart.** Saving the applied scene
   relights it (#134), but `Service.applied` lived in memory. After a
   restart, which happens at every boot, a save only wrote the file.
5. **Make a scene gave one-light zones the whole picture's mean.**
   `paintZone` scans one slice per light, and a zone of one light is one
   slice: the whole width. The MM700's three zones came out the same muted
   colour. The comment said "the middle of the picture", which it was not.

## Requirements

- **R1** A draft keeps its lines in the order they were set, and writes them
  in that order. A scene opened for editing keeps its own order. Setting a
  target again moves it to the end with its new colour.
- **R2** Setting a whole device or a whole zone removes the draft's lines
  inside it, as the window writes targets. A device covers its zones,
  lights and named parts, and a zone covers its lights.
  - **R2.1** The whole scene's colour is not a line and keeps every line.
  - **R2.2** Cancelling the colour wheel restores the draft as it was,
    removed lines included.
- **R3** Each light block is drawn in the newest line that covers it, else
  in the whole scene's colour, else as before. Named parts are not resolved
  in the window.
- **R4** `spotOf` reads a single light `Zone[5]` as that light.
- **R5** The state file records the scene last applied. A service that
  starts reads it back, so saving that scene relights it after a restart. A
  rename of the applied scene updates the record.
- **R6** Make a scene treats a device's one-light zones as one strip in the
  device's order, when there are two or more. A device with one such zone
  keeps the mean. Zones of several lights are unchanged.

## Acceptance Criteria

- [x] Overlapping lines resolve by edit order, and a scene's own order is
      kept (`TestTheNewestColourShowsWhereTwoOverlap`,
      `TestAScenesOwnOrderIsKept`).
- [x] A whole device replaces its lines; a device with a longer name sharing
      the prefix is untouched (`TestTheWholeDeviceReplacesWhatWasSetInsideIt`).
- [x] A whole zone replaces only its own lights
      (`TestAWholeZoneReplacesItsLightsOnly`).
- [x] The whole scene's colour keeps its exceptions
      (`TestTheWholeScenesColourKeepsItsExceptions`).
- [x] Restoring a clone brings back replaced lines
      (`TestRestoringAClonePutsBackWhatWasReplaced`).
- [x] Each light reports the newest line that covers it
      (`TestEachLightIsInTheNewestLineThatCoversIt`).
- [x] Editing a `Zone[5]` line colours that light only
      (`TestALineForOneLightSelectsThatLight`).
- [x] After a restart, saving the applied scene lights it
      (`TestSavingTheSceneOnTheMachineLightsItAfterARestart`).
- [x] A mat's three one-light zones take the left and right of a picture
      (`TestOneLightZonesAreSpreadAcrossThePicture`).
- [x] `make test`, the GUI tests and `make lint` pass.
- [x] Live, with someone watching: in the window, on the MM700, "the whole
      device" in a scene made from a picture shows the picked colour on all
      three lights while the wheel is open, and after Save.
- [x] Live: after `systemctl --user restart hotaru`, `hotaru status` names
      the scene applied before the restart.
- [x] Live: Make a scene from a picture with distinct left and right gives
      the MM700 different colours on Left and Logo.

## Risks & Assumptions

- **Scenes saved by the old editor are sorted.** Opening one keeps that
  order. A scene that relied on sorting by accident shows as before until it
  is edited.
- **"Inside" matches the window's spelling.** A target typed with a shorter
  device name (`kraken/ring[3]`) is not pruned by a whole-device colour. It
  is still overridden, because the newer line comes after it.
- **The applied scene is a label.** After a restart it names the last scene
  applied, as before a restart. Lights changed by another route leave it
  stale, as they already did.
- **The state file gains a `scene` key.** An older hotaru reading the file
  ignores it. A newer one reading an older file finds no name, which is
  today's behaviour.
- **Rollback**: revert the merge. The state file's extra key is ignored by
  the older build.

## Alternatives Considered

- Show a warning that per-light lines override the whole-device colour.
  Rejected by the maintainer in favour of "newest wins".
- Give a one-light zone the picture's most prominent colour. Rejected: all
  three of the mat's lights would still match.

## Verification

Tests: the ten criteria above, each named. Each fix was checked against a
mutation that undoes it:

- Sorting the draft's lines again, or not pruning lines inside a whole
  device, fails the draft tests.
- Reading only `[a:b]` in `spotOf` fails `TestALineForOneLightSelectsThatLight`.
- Matching only a light's own target fails
  `TestEachLightIsInTheNewestLineThatCoversIt`.
- Not reading the scene back in `SetRecorder` fails the restart test.
- Raising the one-light threshold fails `TestOneLightZonesAreSpreadAcrossThePicture`.

`make test`, `go test -tags migrated_fynedo ./internal/gui/...` and
`make lint` pass. `govulncheck ./...`: no vulnerabilities.

Live, with the development build as the user's service:

- Make a scene from a square picture in red, green and blue thirds gave:

  ```
  Corsair MM700/Left[0]  #f70000
  Corsair MM700/Right[0] #00f700
  Corsair MM700/Logo[0]  #0000f7
  ASUS ROG MAXIMUS Z790 HERO/Aura Mainboard[0] #8c8c8c
  ```

  The board's one light keeps the mean, as R6 says. A first try with a wide
  picture gave every light green: the library keeps the square middle of a
  picture for the panel, which was the green third.
- `nebula` was applied, and the service restarted. `/v1/status` then read
  `scene= nebula`, and `hotaru status` printed it.
- The maintainer checked the window: "the whole device" on the MM700 in a
  scene made from a picture lit all three lights, in the editor and on the
  hardware, and the colour stayed after Save.
