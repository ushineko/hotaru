# Spec 073: the kraken drawn through sanshoku

**Issue**: [#208](https://github.com/ushineko/hotaru/issues/208)

## Status: INCOMPLETE

## Executive Summary

hotaru draws the Kraken's ring and fans through sanshoku's canvas on the
cooler's own handle, and OpenRGB's detector for the cooler goes off. A
`formerly` rule keeps scenes, segments and recorded colours written against
OpenRGB's listing working without rewriting any file, and spec 070's retouch
is removed because sanshoku stores the frame before the screen's readout.
Review first: `lightCooler` in `internal/daemon/serve.go`, `Borrow` in
`internal/canvas/canvas.go`, and `formerly` and `adopted` in
`internal/service`.

## Context

hotaru reads the NZXT Kraken Elite's status and draws on its screen through
sanshoku (spec 059), and lights its pump ring and fans through OpenRGB. Two
programs on one hidraw node cost specs 068 and 070, and #206: OpenRGB 1.1 can
come up with both Hue 2 channels at 0 LEDs, and no client can repair it.

sanshoku spec 024 (PR ushineko/sanshoku#73) makes the cooler's device a
`lighting.Canvas` on the handle it already holds. Measured there on the
desk:

- Frames are `26 14` live writes, acknowledged in about 2 ms, even from 100 ms
  down to 16 ms; the floor is 16 ms. A still frame holds with nothing sent.
- The screen's switch to its firmware readout re-applies the cooler's stored
  lighting 150 to 300 ms later. sanshoku commits the held frame before every
  switch it makes, and on `Release`, so the ring keeps its colours.
- Spec 070's green LED is OpenRGB's split 64-byte write leaving ring LED 22
  unset in that stored buffer. The 12 s retouch covered it; the commit removes
  the cause.
- Keys are `ring 1` to `ring 24` (channel 1) then `fan 1` and on (channel 2),
  read from the cooler's accessory list.

Three things in hotaru stand in the way of drawing it as a canvas (spec 060):

1. **The handle.** Canvas discovery scans its own drivers and opens what it
   finds. Adding `nzxt` to it would open the cooler's node a second time, and
   a canvas that is lost or released closes its device, which here is the
   cooler's.
2. **Release.** A canvas's release assumes the device reboots, as the Apex
   does: it closes the handle and re-attaches. The Kraken does not reboot.
3. **Names.** The scenes, the `kraken` segment rule and state.yml name the
   cooler as OpenRGB lists it: `NZXT Kraken 2024 ELITE Series RGB`, zones
   `Hue 2 Channel 1` and `Hue 2 Channel 2`. With OpenRGB's detector off,
   nothing lists that name. The maintainer's scenes file has 8,486 lines and
   addresses the ring light by light.

## Requirements

### R1. One handle

- R1.1 When the cooler is attached and its device is a `lighting.Canvas`, the
  daemon attaches it as a canvas too, from the same `sanshoku.Device`.
  `internal/cooler` gives the device out for this. Canvas discovery does not
  scan the `nzxt` driver.
- R1.2 The canvas made this way is *borrowed*: losing it, closing it and
  releasing it never close the device. The cooler owns the handle and closes
  it; the canvas is detached before that, when the cooler goes or the service
  stops.
- R1.3 A cooler whose accessory list does not answer gives a canvas with no
  keys. It is not attached, and is asked again on a backoff while the cooler
  is attached.

### R2. Release keeps the device

- R2.1 Releasing a borrowed canvas stops its animator, commits the frame
  (sanshoku `Release`) and keeps it attached. Nothing re-attaches. The
  recorded state is forgotten, as for any canvas, and so is the record under
  a former name (R4.4), or the next full reconcile would adopt it and undo the
  release. The release's message says the cooler keeps its colours and stays
  attached.
- R2.2 The Apex's release is unchanged.

### R3. Zones from the key names

- R3.1 A canvas whose every key name is a word and a number (`ring 3`) lists
  one zone per word, in key order, named with the word capitalised (`Ring`,
  `Fan`). Any other canvas keeps its one `Keys` zone. The Apex and the F75
  are unchanged.

### R4. The names a cooler had in OpenRGB

- R4.1 A device rule gains `formerly`: the name the hardware had in OpenRGB,
  and a map from its old zone names to the canvas's. For this desk:

  ```yaml
  - match: "kraken"
    formerly:
      name: "NZXT Kraken 2024 ELITE Series RGB"
      zones: {"Hue 2 Channel 1": "Ring", "Hue 2 Channel 2": "Fan"}
  ```

- R4.2 When OpenRGB does not list the former name, hotaru treats it as the
  canvas's twin (spec 060): a listing with the former name, the canvas's LEDs
  and names, and its zones under their former names. Scenes are carried by
  the existing `carry`. An assignment to the former device, a former zone, a
  former zone's index range, or a segment of the rule all land on the same
  LEDs of the canvas.
- R4.3 A segment whose zone is a former name resolves on the canvas through
  the map, so a scene written against the canvas's own name can use the
  rule's segments.
- R4.4 Restoring: a canvas with nothing recorded under its own name takes the
  record under the former name, when it has the canvas's LED count. The next
  write records it under the canvas's name. The former record is left as it
  is, for a rollback, until a release forgets it (R2.1).
- R4.5 No user file is rewritten.

### R5. The retouch goes

- R5.1 Spec 070's retouch (the daemon's `retouch`, the service's `Retouch`,
  `Handbacks` and `onNode`) is removed. sanshoku commits the held frame
  before the switch to the readout.

### R6. sanshoku

- R6.1 hotaru depends on the sanshoku release that carries spec 024. Until
  it is tagged, the branch's pseudo-version; the PR does not merge on it.
- R6.2 `screen.Panel.Floor(bytes)` is `ImageFloor(bytes)`:
  `internal/cooler` and `coolertest` follow. hotaru's own `Floor(int)`
  interfaces are left as they are.

### R7. Cost

- R7.1 A still scene sends one frame per changed channel and nothing after
  (sanshoku's canvas is not `Fading`). A switch to the readout costs one
  commit, about 0.7 s of exchanges, only when the frame changed since the
  last.
- R7.2 An animated scene runs at the rule's `frame_interval`, default 56 ms.
  Its CPU cost is measured with `pidstat` over 60 s and recorded here.

### R8. Documentation

- R8.1 `docs/hardware.md`: the Kraken's lighting is drawn by hotaru through
  sanshoku; OpenRGB's detector for it is switched off (the exact key in
  `OpenRGB.json`), and why.
- R8.2 `docs/architecture.md`: the cooler's device is also a canvas; diagram
  re-rendered if it changes.
- R8.3 `docs/configuration` or wherever rules are documented: `formerly`.
- R8.4 README changelog under `### Unreleased`, with this spec and #208.

## Acceptance Criteria

- [x] `make test` and `make lint` pass; `go test -tags migrated_fynedo
      ./internal/gui/...` passes.
- [x] A test: the cooler's device is attached as a canvas once, and canvas
      discovery never opens the `nzxt` driver.
- [x] A test: losing, closing or releasing a borrowed canvas does not close
      the device; the cooler's close does, after the canvas is detached.
- [x] A test: zones from key names (`ring n`, `fan n`), and the Apex's keys
      still one zone.
- [x] Tests: with `formerly`, a scene addressing `NZXT Kraken 2024 ELITE
      Series RGB/Hue 2 Channel 1[3]`, `[1:2]`, the whole device, and a rule
      segment on `Hue 2 Channel 2` lights the same canvas LEDs; and a record
      under the former name restores the canvas.
- [x] The retouch is gone and no test refers to it.
- [x] Live, with OpenRGB's detector for the cooler off, someone watching:
      hotaru restores the ring and fans from the existing record after a
      restart; a scene from scenes.yml applies to both; switching the screen
      to the readout and back leaves the ring as it was; `hotaru light release`
      leaves the frame and the cooler attached; telemetry and the dashboard
      keep working throughout.
- [x] R7.2 measured and recorded.
- [x] Docs per R8.
- [ ] `go.mod` requires a tagged sanshoku release that carries spec 024, not
      the branch's pseudo-version (R6.1).

## Risks & Assumptions

- **sanshoku is unreleased.** Spec 024 is incomplete only for its Windows
  run. hotaru builds on the branch until a release exists; merging waits.
- **OpenRGB's detector must be off.** With it on, OpenRGB still reads the
  accessory list at start and holds a handle; it writes nothing on its own
  (sanshoku spec 024, Verification), but a person using OpenRGB's window
  would write the same buffer. hardware.md says so.
- **The firmware effects** (sanshoku #72) are not here. With OpenRGB's
  detector off they are not reachable until #72 lands.
- **Rollback:** revert, and switch OpenRGB's detector back on. The former
  record is never removed, so OpenRGB's path finds its colours again.

## Alternatives Considered

- Keeping OpenRGB's names on the canvas: no config, but hotaru would keep
  OpenRGB's naming for a device OpenRGB no longer drives.
- Rewriting scenes.yml, hotaru.yml and state.yml once: the cleanest end
  state, but an edit of the maintainer's 8,486-line scenes file.
- Adding `nzxt` to canvas discovery: a second handle on the node, which is
  what this spec removes.

## Verification

### Tests

`make test`, `make lint` (0 issues) and `go test -tags migrated_fynedo
./internal/gui/...` pass, on sanshoku
`v0.1.20-0.20261117005959-808800e784d9` (the spec 024 branch).

- `internal/canvas/borrow_test.go`, `drivers_internal_test.go`: a borrowed
  canvas never closes the device and is not lost on release; no keys is not a
  canvas yet; the Apex's release is unchanged; discovery never scans `nzxt`.
- `internal/daemon/lights_test.go`: the cooler is opened once, its device is
  attached as a canvas, and closing the cooler leaves no canvas behind.
- `internal/service/formerly_test.go`: zones Ring and Fan from the key names;
  the former name listed as the canvas's twin; a former zone's light, a range,
  a segment by either name and the whole device land on the same canvas
  lights; a former record restores the canvas and is kept; a record of
  another size is not adopted; a release keeps the cooler attached and
  forgets the former record.

### Live (2026-11-16, the maintainer watching)

A development build under the user service's drop-in, OpenRGB 1.1 with
`"NZXT Kraken 2024 ELITE Series RGB": false`, and the `formerly` rule from
R4.1 added to the maintainer's `kraken` rule.

- Start: `drawing on NZXT Kraken Elite V2 at /dev/hidraw1`, the cooler's own
  node. The first restore ran before the canvas was attached and named the
  former device as missing; the next restored all six devices. The ring and
  fans showed the recorded colours, adopted from the former record.
- `hotaru light list`: the canvas `Static, holding`, and the former name
  listed as handed to it.
- `hotaru scene apply cherryblossoms-1`, whose 40 Kraken targets are
  `NZXT Kraken 2024 ELITE Series RGB/Hue 2 Channel 1[…]`: the ring and fans
  changed to the scene's blues and pinks.
- `hotaru screen readout`, then `hotaru screen dashboard` 5 s later: the ring
  and fans kept their colours through both, with no green LED.
- `hotaru light release "NZXT Kraken Elite V2"`: the colours stayed, the
  canvas stayed attached, and `hotaru cooling` read 44.3 °C, 2522 rpm pump,
  961 rpm fan.
- **Found here, and fixed:** the release forgot only the record under the
  canvas's name. The next restart adopted the former record and put the old
  colours back. Release now forgets the former record too (R2.1). After the
  fix: apply, release, restart; the restart restored the other five devices,
  sent the cooler nothing, and its colours stayed. The release's message
  read as the Apex's ("re-enumerates"); it now says the cooler keeps its
  colours and stays attached.

### Cost (R7.2)

The service's CPU time from `/proc/<pid>/stat` over 60 s (pidstat is not
installed), with the dashboard running:

| The cooler's lights | CPU, one core |
|---|---|
| A still frame, holding | 3.48% |
| Rainbow Wave at 17.5 fps (the 56 ms default), both channels | 3.83% |

A moving effect on the cooler costs about 0.35% of a core.

### Not covered

- The preview used for the cost run ended with the cooler still running
  Rainbow Wave, because the cooler had no record after the release, so
  nothing replaced the preview's drawing. That is spec 060's behaviour for
  any canvas with nothing recorded, and is left as it is here.
- The packaged hotaru does not draw the Kraken. Until a release carries this
  spec, OpenRGB's detector must stay on for a packaged build.
