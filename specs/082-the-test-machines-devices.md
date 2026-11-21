# Spec 082: the test machines' devices

**Issue**: [#231](https://github.com/ushineko/hotaru/issues/231)

## Status: COMPLETE

## Executive Summary

hotaru draws the devices sanshoku 0.6.0 added drivers for: a Gigabyte
board's RGB Fusion 2 controllers, an EVGA RTX 30 card, Corsair DDR5 memory,
Razer mice and docks, and an MSI board's RGB header. The Linux test machine
is lit by 2.x without OpenRGB. The udev rule grows to match. The Windows
test machine's board and memory need PawnIO, which serves only an
administrator; that is left to sanshoku #110 and a later spec. Review
`drivers()` in `internal/canvas/canvas.go` and `packaging/60-hotaru.rules`.

## Context

hotaru 2.0 dropped OpenRGB (#212) having checked its drivers against the
development machine only. The two test machines in `docs/hardware.md` kept
0.1.29, because 2.x lit one device on each. sanshoku 0.5.0 and 0.6.0 added
the missing drivers (sanshoku #105, #106, #107, #108, #109).

## Requirements

- R1 sanshoku v0.6.0.
- R2 `drivers()` in `internal/canvas` adds `gigabyte.Driver`, `evga.Driver`,
  `corsairdram.Driver`, `razer.Lights` and `msi.Board`. Each device they find
  satisfies `lighting.Canvas`; nothing else in hotaru names a product.
- R3 `packaging/60-hotaru.rules` tags, from sanshoku's rule: Razer's hidraw
  nodes (1532), the Gigabyte ITE controller (048d:5702), and the chipset's
  SMBus adapters (I801, PIIX4). An EVGA card is on the NVIDIA adapters
  already tagged.
- R4 `docs/hardware.md` says what 2.x lights on each test machine.

## Acceptance Criteria

- [x] `make test`, `make lint`, `make lint-windows`, and the GUI tests
      (`-tags migrated_fynedo`).
- [x] Live on the Linux test machine, under the user service with the
      packaged unit's hardening, OpenRGB stopped: every device is found, and
      a colour set on each shows, with the maintainer watching.
- [x] `docs/hardware.md` and the README changelog say what changed.

## Out of scope

- The Windows test machine's board and memory: the drivers are registered,
  and the installed service, as `NT SERVICE\hotaru`, cannot open PawnIO
  (sanshoku #110). A later spec settles the account and installs PawnIO's
  modules.
- A release, and moving the Linux test machine onto it from its drop-in.

## Risks & Assumptions

- **The SMBus rule** lets the logged-in user's programs reach everything on
  the chipset's SMBus, the memory's SPD included. sanshoku's driver writes
  only the lighting controller's registers, and never one that stores a
  setting. OpenRGB's rule gives the same.
- **Recorded state from OpenRGB's days** does not always fit: the Gigabyte
  controller recorded with 61 LEDs is one light now, and is skipped on
  restore with a log line, as a length mismatch always is. A colour set
  afterwards records the new shape.
- **Cost:** the I2C devices are written only when a frame changes, at their
  drivers' floors (Corsair DDR5 and the EVGA card as sanshoku measured).
- **Rollback:** revert. The udev rule's extra lines tag nodes no other part
  of hotaru opens.

## Verification

### Live run (2026-11-20, the Linux test machine)

The dev build ran under the packaged user unit (`ProtectSystem=strict` and
the rest) through a drop-in overriding `ExecStart`. The new udev rule was
installed in `/etc/udev/rules.d` and applied to the hidraw and i2c-dev
nodes; OpenRGB was stopped.

`hotaru light list` found: Corsair Dominator Platinum RGB DDR5 at 0x18,
0x19, 0x1A and 0x1B (12 LEDs each, `/dev/i2c-21`), EVGA GeForce RTX 3080
FTW3 Ultra LHR (4, `/dev/i2c-10`), Razer Mouse Dock Pro (9) and the
Basilisk V3 Pro through it (13, `/dev/hidraw0`), Z790 AORUS MASTER X at
0x57010100 and 0x57020100, and the Apex Pro TKL Gen 3. The restore put back
seven devices from 0.1.29's record.

Previews, with the maintainer watching: the four modules magenta, the card
green, both Gigabyte controllers (strips and board) cyan, the dock blue, the
mouse yellow, the keyboard unchanged. All showed as set.
