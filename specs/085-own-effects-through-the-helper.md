# Spec 085: own effects through the helper

**Issue**: [#238](https://github.com/ushineko/hotaru/issues/238)

## Status: INCOMPLETE

## Executive Summary

The PawnIO helper carries a device's own effects across its pipe, so the
service runs the MSI header's and the original Vengeance's hardware effects
(sanshoku #122) as it runs the RTX 4090's. Breathing on those devices is the
device's own, smooth, instead of frames that stepped. Review `own` in
`internal/pawnhelper/handler.go` and `tuned` in `client.go`.

## Context

On the Windows test machine, drawn Breathing stepped on the MSI B450 header
(four bits a channel) and the Vengeance RGB modules (78 ms a colour through
PawnIO), and the maintainer saw it (2.3.1). sanshoku now gives both devices
`lighting.Tuned`. The helper's protocol (spec 083) carried only list, frame
and release, so the service could not reach them.

## Requirements

- R1 Protocol 2. A listed device carries its catalogue where its driver is
  `lighting.Tuned`.
- R2 Three requests: `effect` (a name the device lists, its colours, at most
  16, its speed, restart), `colour` (one colour for the running effect),
  `running` (the effect it runs). A device that runs none, a name it does
  not list, or more colours than allowed is refused before the device is
  asked; per-device lock as for frames.
- R3 The lighting errors a caller tells apart (unknown effect, no effect, no
  colour, too many colours, speed) cross the pipe by name and are the same
  errors on the service's side.
- R4 On the service's side a helper device that lists effects is a
  `lighting.Tuned`; one that lists none is a canvas only.
- R5 sanshoku at the release that carries #122.

## Acceptance Criteria

- [ ] `make test`, `make lint`, `make lint-windows`, GUI tests.
- [ ] Tests: a device's own effects listed, run with colour and speed, read
      back, coloured; a name not listed and too many colours refused as the
      lighting errors; a canvas with no effects stays a canvas
      (`TestADevicesOwnEffectsRunThroughTheHelper`).
- [ ] Live on the Windows test machine, the maintainer at it: the board and
      both modules show their own Breathing, smooth.
- [ ] `docs/architecture.md`, the project guidelines, `docs/hardware.md`
      and the changelog say what changed.

## Risks & Assumptions

- **The helper's surface grows** by effect names the driver lists and their
  settings, which sanshoku checks (`lighting.Fit`); still no bus, register
  or storing command. The Vengeance's fade time is the one register more
  that is written, as OpenRGB writes it.
- **Not phase-locked**: a device's own effect runs on its own clock, so it
  does not breathe in step with the drawn ones.
- **Rollback:** revert; the service and the helper are installed together,
  and a protocol mismatch is refused by name.

## Verification

Pending.
