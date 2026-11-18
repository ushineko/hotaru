# Spec 074: sanshoku devices without openrgb

**Issue**: [#210](https://github.com/ushineko/hotaru/issues/210)

## Status: COMPLETE

## Executive Summary

hotaru's own devices (canvases) are listed, drawn and restored with no
OpenRGB server, by standing them beside a server with no devices; health,
probe and rescan still need the real one. The live run also found that a
solid colour on the Kraken went to its Fading effect, and a canvas that keeps
its frame now draws a solid colour itself. Review `absent.go`, `current()`,
and `storedSolid` in `internal/devices/mode.go`.

## Context

hotaru draws some devices itself through sanshoku (spec 060): the Apex, the
F75 and, with spec 073, the Kraken's lights. It reaches them through the same
calls it makes for OpenRGB's devices, by wrapping the OpenRGB client
(`lights`, `internal/service/canvas.go`). With no OpenRGB server there is no
client to wrap: `Service.current()` returns nil, and listing, applying,
scenes and restoring all answer "no OpenRGB server" (`unreachable`). A canvas
already attached waits for a server it does not use
(`internal/daemon/canvas.go`: "waits for the OpenRGB server").

This is the first step of retiring OpenRGB (#212). Each sanshoku driver that
follows can then be used on its own.

## Requirements

### R1. A missing server is an empty one, where hotaru draws something itself

- R1.1 With no OpenRGB client and at least one canvas attached, `current()`
  gives the canvases beside an *absent server*: a client that lists no
  devices, and refuses a write to a device it does not have with the same
  `unreachable` error as today.
- R1.2 Listing, applying a colour or a scene, scenes from images and
  reconciling then reach the canvases as they do with a server. A recorded
  OpenRGB device is missing, as when a server does not list it.
- R1.3 With no client and no canvas, nothing changes: every call is
  `unreachable`, as before.

### R2. What stays about OpenRGB

- R2.1 Health still reports the server unreachable, with its remedies, and
  lists the attached canvases (it does today). `probe` and `rescan` still
  need the server: they are OpenRGB's.
- R2.2 When a server connects, its devices join the canvases, as today.

### R3. The daemon

- R3.1 A canvas attached with no server is restored from what is recorded,
  at once. The "waits for the OpenRGB server" path remains only for a restore
  that fails.

### R4. Documentation

- R4.1 `docs/architecture.md`: hotaru's own devices work with no OpenRGB
  server.
- R4.2 README changelog under `### Unreleased`.

### R5. A solid colour on a canvas that keeps its frame is drawn

Found in the live run below. Spec 071 sends a solid colour on a device with
stored effects to the running effect, or else to the firmware's first effect
that takes a colour: on the AULA F75 that is Fixed on. On the Kraken it is
Fading, and `hotaru light set` made the cooler fade.

- R5.1 The firmware's first coloured effect is used only on a canvas whose
  frames fade (`lighting.Fading`, the F75). A canvas that keeps its frame
  (the Kraken) draws the colour itself. A running effect that takes a colour
  is still given the colour, on both.
- R5.2 The device record says whether a canvas fades (`Canvas.Fades`).

## Acceptance Criteria

- [x] `make test`, `make lint`, and the GUI tests pass.
- [x] Tests with no OpenRGB client and a canvas attached: the canvas is
      listed; a colour and a scene light it; a reconcile restores it from the
      record; a write to an OpenRGB device is `unreachable`; health is
      unreachable and lists the canvas; `probe` is `unreachable`.
- [x] A test with no client and no canvas: listing is `unreachable`.
- [x] A test: a solid colour on a canvas with stored effects that keeps its
      frame is drawn in Static, and the test fails with R5.1 removed.
- [x] Live, with someone watching: OpenRGB's server stopped, hotaru
      restarted; the Kraken's ring and fans are restored, `hotaru light set`
      changes them, and `hotaru light list` lists them. OpenRGB started
      again; its devices join.
- [x] Docs per R4.

## Risks & Assumptions

- **A scene names OpenRGB devices too.** With no server those are left out
  and reported missing, as with a server that does not list them.
- **Rollback:** revert. With a server running, nothing changes.

## Verification

### Tests

`make test`, `make lint` and the GUI tests pass.
`internal/service/absent_test.go`: no server and no canvas is unreachable; a
canvas is listed, lit by a colour and by a target, and restored from the
record with no server, the recorded OpenRGB device reported missing; a write
to an OpenRGB device writes nothing; health is unreachable and lists the
canvas; probe is unreachable. `internal/service/holding_test.go`: a solid
colour on a holding canvas with stored effects is Static; with R5.1 removed
it is "Fixed on".

### Live (2026-11-17, the maintainer watching)

The development build, OpenRGB's server stopped, hotaru restarted:

- The log said "no OpenRGB server … lighting waits for one", then "drawing
  on NZXT Kraken Elite V2". `hotaru light list` listed the cooler, holding a
  Static frame; its tan was restored.
- `hotaru light set "NZXT Kraken Elite V2=#0060ff"` answered "Fading": the
  colour went to the cooler's Fading effect (R5). The maintainer did not see
  that one. After the fix: "Static", and the maintainer saw steady blue.
- Fading chosen on purpose from a scene's `effects` faded between green and
  blue.
- OpenRGB's server started: "connected to the OpenRGB server", "restored 6
  devices", and its five devices listed again beside the cooler.
