# Spec 075: every device through sanshoku

**Issue**: [#211](https://github.com/ushineko/hotaru/issues/211)

## Status: COMPLETE

## Executive Summary

hotaru draws every device on the maintainer's desk through sanshoku and runs
with OpenRGB stopped; OpenRGB becomes an optional dependency. Zones come from
the drivers, named as OpenRGB named them; a header's length is a `lengths`
rule; records made through OpenRGB restore, fitted to the new lengths. The
live run with OpenRGB stopped found two gaps, both fixed here: the keyboard's
own effects (sanshoku spec 031) and the zone fit (R3.4). Review
`internal/service/reconcile.go` (`refit`, the Static fallback) and
`internal/service/health.go` first.

## Context

sanshoku now drives every lit device on the maintainer's desk (#212): the
Aura mainboard (sanshoku spec 027), the Corsair MM700 (026), the Keychron K4
HE (028), the MSI RTX 4090 (029) and the G502 X PLUS (030), beside the Apex,
the F75 and the Kraken it already drove. Spec 074 lets hotaru draw sanshoku
devices with no OpenRGB server. This spec makes hotaru draw all of them, so it
runs with OpenRGB stopped and no longer depends on it.

It is the functional switch. Deleting the code that exists only for OpenRGB
(its client and reconnection, `rescan`, `probe`, the mode fall-through, zone
regrow, `openrgb.Fake` under 35 test files) is the next spec: wide and
mechanical, and easier to review apart from a change in behaviour.

What the drivers need from hotaru, beyond what spec 060 gives a canvas:

- **Zones.** sanshoku's `lighting.Zoned` names each device's zones as OpenRGB
  did ("Addressable RGB Header 2", "Keyboard", "GPU", "Mouse LEDs"), so scenes,
  segments and recorded colours written against OpenRGB find them.
- **Lengths.** An addressable header cannot say how many LEDs its strip has;
  OpenRGB kept the length in its profile, and kept it wrong on this desk (16
  where the strips have 18 and 20). The length is now a rule.
- **Names that are not OpenRGB's.** The G502's canvas is the receiver's node,
  "Logitech USB Receiver"; spec 073's `formerly` maps "G502 X PLUS" onto it.
- **Modes that are not OpenRGB's.** A device recorded in OpenRGB's "Direct"
  or a firmware mode a canvas does not have is restored in Static.

## Requirements

### R1. Every sanshoku canvas

- R1.1 Canvas discovery scans the `asus`, `corsair`, `keychron`, `msi` and
  `logitech` drivers beside `steelseries` and `aula`. `nzxt` stays the
  cooler's (spec 073).
- R1.2 A device that is a `lighting.Canvas` with no keys is not a canvas and is
  closed: every Logitech receiver node is a canvas, empty where nothing behind
  it has per-key lighting.
- R1.3 sanshoku is the release that carries specs 026 to 031 and the MM700's
  zones (sanshoku #85) (a pseudo-version until it is tagged).

### R2. Zones and lengths

- R2.1 A canvas whose device is `lighting.Zoned` lists its zones as the device
  names them, each with its first light and count, and a resizable one with
  its range. Any other canvas keeps spec 073's zones from key names.
- R2.2 A rule's `lengths` gives a resizable zone its LED count:

  ```yaml
  - match: "maximus"
    lengths: {"Addressable RGB Header 1": 18, "Addressable RGB Header 2": 16,
              "Addressable RGB Header 3": 20}
  ```

  Applied when the canvas is attached, before its keys are read. A zone not
  named keeps its length (0 for an Aura header). A length the device refuses
  is reported and skipped.
- R2.3 Spec 069's regrow does not resize a canvas: its lengths are the rule's,
  and a size OpenRGB recorded may be wrong.

### R3. Restoring what OpenRGB recorded

- R3.1 A canvas restoring a record whose mode it does not have (OpenRGB's
  "Direct", a firmware mode of OpenRGB's) restores the colours in Static.
- R3.2 While an OpenRGB server still runs, an OpenRGB device with the same name
  as a canvas is the canvas's twin, as one at the canvas's node is (spec 060):
  the GPU's OpenRGB location is an I2C one, not a node.
- R3.3 With no OpenRGB server and a canvas drawn, health is healthy and says
  there is no server; it is unreachable only when nothing is drawn. This
  replaces spec 074 R2.1, written while OpenRGB was required.
- R3.4 A record laid out for other zone sizes (OpenRGB's 16 a header) is
  fitted to the canvas's zones, zone by zone: each recorded zone's colours are
  stretched or shrunk over the zone's length. Without it, a header's colours
  start where the sizes before it put them, and a fan's gradient is cut.

### R4. Packaging

- R4.1 `packaging/PKGBUILD`: `openrgb` moves from `depends` to `optdepends`.
- R4.2 `packaging/60-hotaru.rules` grants the new devices: hidraw of `0b05`,
  `1b1c`, `3434` and `046d`, and the NVIDIA I2C adapters.

### R5. Documentation

- R5.1 `docs/hardware.md`: each device drawn through sanshoku, the `lengths`
  rule, the G502's `formerly`, and that OpenRGB is optional.
- R5.2 `docs/architecture.md`: sanshoku is the backend; OpenRGB optional.
- R5.3 README: changelog, and the install steps without OpenRGB.

## Acceptance Criteria

- [x] `make test`, `make lint` and the GUI tests pass.
- [x] Tests: discovery scans the new drivers and skips a keyless canvas; a
      zoned canvas lists its zones; a `lengths` rule resizes before keys are
      read, and a refused length is reported; regrow leaves a canvas alone; a
      record in "Direct" restores a canvas in Static; a same-named OpenRGB
      device is the canvas's twin; a record is fitted zone by zone; health
      with canvases and no server is healthy.
- [x] Live, with someone watching, OpenRGB stopped: every device on the desk
      restores its scene; a scene applies to all of them; the Aura's headers
      light to their full lengths.
- [x] Docs per R5; packaging per R4.

## Risks & Assumptions

- **sanshoku is unreleased.** hotaru builds on its main branch's
  pseudo-version until a release.
- **Discovery cost.** Five more drivers on the 30 s canvas scan; a Logitech
  node with no lights is opened, asked and closed on each.
- **Rollback:** revert, and start OpenRGB again: its devices come back through
  it, and the records are untouched.

## Verification

### Tests

`make test`, `make lint` (0 issues), the GUI tests and govulncheck pass, on
sanshoku `v0.1.20-0.20261117161130-afa4e685911f` (main, with #85 and #87
merged).

- `internal/canvas/zoned_test.go`, `drivers_internal_test.go`: a keyless
  canvas is not one; a zoned canvas gives its zones and a header's length
  changes its keys, a refused length changes nothing; discovery scans
  steelseries, aula, asus, corsair, keychron, msi and logitech, and not nzxt.
- `internal/daemon/lengths_test.go`: a rule's lengths are given at attach, and
  a zone the device does not have is reported.
- `internal/service/sanshoku_test.go`: a zoned canvas is listed with its own
  zones; a record in OpenRGB's Direct restores the canvas in Static and the
  record's zone sizes do not resize it; an OpenRGB device named as a canvas is
  its twin; a record of a two-light header is stretched over four lights.
- `internal/service/absent_test.go`: with a canvas and no server, health is
  healthy and names the missing server; with neither, unreachable.

### Discovery on the desk (2026-11-17, read-only, OpenRGB running)

`canvas.Open` from this branch, with the Aura's lengths given as the rule
would (18, 16, 20); nothing drawn:

| Device | Node | Keys | Zones |
|---|---|---|---|
| ASUS ROG MAXIMUS Z790 HERO | `/dev/hidraw5` | 55 | Aura Mainboard 1; Addressable RGB Header 1 18, 2 16, 3 20, 4 0 (resizable) |
| Corsair MM700 | `/dev/hidraw10` | 3 | none, before sanshoku #85 |
| Keychron K4 HE | `/dev/hidraw37` | 100 | Keyboard 100 |
| MSI GeForce RTX 4090 Suprim Liquid X | `/dev/i2c-3` | 1 | GPU 1 |
| Logitech USB Receiver | `/dev/hidraw13` | 8 | Mouse LEDs 8 |

The MM700's missing zones (its scenes use Left, Right and Logo) are sanshoku
#85.

### Live (2026-11-17, OpenRGB stopped, the maintainer watching)

The maintainer's rules gained the Aura's `lengths` (18, 16, 20) and a
`logitech usb receiver` rule with `formerly: {name: G502 X PLUS}`;
`openrgb-server.service` was stopped and the dev build restarted.

1. **Attach.** Every device was drawn through sanshoku: the Kraken, the
   receiver, the Z790, the MM700, the RTX 4090 and the K4 HE. Each restored
   Static, holding; the G502 and the Kraken's OpenRGB names were listed as
   handed over.
2. **The keyboard did not come back in Solid Reactive Multinexus.** Its record
   (`mode: Solid Reactive Multinexus`, `mode_colour: '#9fc95f'`) had no mode on
   the canvas, so R3.1 drew it Static. The driver had no firmware effects:
   sanshoku spec 031 (#86, PR #87) added them. With them, the restore left the
   keyboard alone, because it found the per-key effect the first run had left
   and hotaru writes a device's own effect only when asked (spec 071 R6.3).
   Setting its recorded colour wrote Multinexus in `#9fc95f`: "there it is,
   fixed." It held through every restart after.
3. **The rear fan was missing a LED.** A magenta preview lit all 20 LEDs. The
   record was laid out for OpenRGB's 16 a header (65 colours for 55 lights)
   and was written flat, so each header's colours started late and the rear
   fan's ran into header 4's. R3.4 fits the record zone by zone: "yes, full
   gradient on the rear fan."
4. **A scene on every device.** `custom3`: "6 of 6 device(s) lit", "everything
   looks right, no dark leds", and the keyboard in "white multiwide". The
   scene's 14 targets on the empty header 4 were each reported. The first
   apply's keyboard effect was covered by a preview started by mistake and
   then killed; a second apply showed it.
5. `hotaru light health` with no server: "healthy: 6 devices drawn by hotaru;
   no OpenRGB server" (R3.3); before R3.3 it said unreachable.

The desk was put back after: the record from before the run, the keyboard in
Multinexus, the scene label `landscape-mountains`.

### Not covered

- Scenes address each header by LED index as OpenRGB had it (0 to 15). On the
  18- and 20-LED strips the last LEDs keep what they showed; they looked right
  in `custom3`. A target on the empty header 4 is reported per LED.
- A preview killed while it held a firmware effect's colour left it Static
  until the next apply (step 4); not reproduced.
