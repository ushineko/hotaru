# Spec 076: remove the openrgb backend

**Issue**: [#215](https://github.com/ushineko/hotaru/issues/215)

## Status: INCOMPLETE

## Executive Summary

Written last.

## Context

Since spec 075, hotaru draws every device it supports through sanshoku, and
OpenRGB is optional. The code that exists only for OpenRGB remains, and the
tests are built on its fake:

- the `openrgb` package: client, connection, reconnection and fake (about
  2,450 lines);
- `stale` (spec 058) and the server bounce behind `light rescan`;
- `light probe` and the mode fall-through;
- the twin and handed-over listings, and zone regrow (spec 069);
- the health states that describe a server;
- `openrgb.Fake` under 41 test files (216 constructions).

The maintainer decided on 2026-11-17 (#212):

- **remove OpenRGB entirely**, so hotaru supports what sanshoku drives;
- **move the tests onto canvases** built from `canvastest`, not a renamed
  listing fake;
- **one spec in two PRs**: the tests first, with no change in behaviour, then
  the deletion.

## Requirements

### R1. Tests on canvases (PR 1)

- R1.1 `canvastest.Board` is a canvas with named zones (`lighting.Zoned`),
  some resizable; `canvastest.Firmware` is a Board with effects of its own
  (`lighting.Onboard`, `lighting.Catalogue`) whose frames hold. They replace
  spec 075's `canvastest.Zoned`.
- R1.2 `servicetest` attaches canvases to a service as the daemon does, and
  builds a service with no OpenRGB server, for the tests of every package.
- R1.3 Every test of behaviour hotaru keeps without OpenRGB runs on canvases.
  A test of OpenRGB-only behaviour stays on `openrgb.Fake` until R2 deletes
  it. Verification lists each file's tests by verdict.
- R1.4 No production code changes.

### R2. The deletion (PR 2)

- R2.1 The `openrgb`, `stale` and server-bounce code is deleted, with what
  only it reaches: the client and its reconnection, `rescan`, `probe`, the
  mode fall-through, regrow, twins and the handed-over listing, and the
  server's health states.
- R2.2 The service's devices are its canvases. Its device interface is
  hotaru's own, not OpenRGB's `Client`.
- R2.3 `light rescan` and `light probe`, and their API routes, are removed,
  with their window counterparts; the parity tests list what remains.
- R2.4 Records and scenes made through OpenRGB keep working: `formerly`, the
  Static fallback and the zone fit (spec 075) stay.
- R2.5 Rules that only meant something to OpenRGB are named in the docs as
  ignored, not rejected, so a rules file with them still loads.
- R2.6 Packaging: no `openrgb` optdepend, no `openrgb-enumeration.conf`
  example; the Windows installer docs drop OpenRGB.

### R3. Documentation

- R3.1 `docs/architecture.md`, `docs/hardware.md`, `docs/api.md`,
  `docs/packaging.md` and the README describe hotaru without OpenRGB; the
  README says supported hardware is what sanshoku drives.

## Acceptance Criteria

- [x] PR 1: `make test`, `make lint` and the GUI tests pass; no production
      file changed; Verification lists every test file's verdicts.
- [x] PR 2: no package imports `internal/openrgb`; `make test`, `make lint`
      and the GUI tests pass.
- [x] PR 2: the parity and guard tests pass with `rescan` and `probe` gone.
- [ ] Live, with someone watching, after PR 2: the desk restores on a
      restart, a scene lights every device, and `hotaru light health`
      reports the canvases.
- [x] Docs per R3; packaging per R2.6.

## Risks & Assumptions

- **Hardware support narrows.** Hardware sanshoku does not drive is no
  longer lit. On Windows that includes the RTX 4090, whose driver is Linux
  only. Accepted (#212).
- **Test coverage could drop with the fake.** R1 moves every surviving test
  first, so PR 2's deletions remove only tests of deleted code.
- **Rollback:** revert PR 2; PR 1 is test-only.

## Verification

### PR 1: the tests on canvases (2026-11-17)

`make test` passed three runs in a row; `make lint` reports 0 issues; the GUI
tests pass. `TestAPartialSceneOnACanvasBuildsOnWhatHotaruWrote`, seen failing
once in a run while files were being edited, passed 20 runs and 5 under
`-race`. Outside `_test.go` files only `canvastest` (`board.go`,
`firmware.go`, `Zoned` removed), `servicetest` and this spec changed (R1.4).
Mutation checks: a converted assertion was broken on purpose in each group
(brightness rule, profile, scene colour, held preview, late device, reconcile,
segment name, redraft effect) and its test failed.

Verdicts. "Canvas" is rebuilt on canvases; "left" stays on `openrgb.Fake` for
PR 2 to delete, with the reason.

| File | Canvas | Left (why) |
|---|---|---|
| service/service_test | no-config, never_blank, scope, naming, list, brightness rule, typo, no wait | fall-through ×2, own Off mode, one-colour strip, stopped server, server failure, mode colours ×6, per-LED order, brightness packet |
| service/canvas_test | 8 | twins and handed-over ×6 |
| service/stored_test, formerly_test, screen_test, images_test, dashboards_test, holding_test, restorescreen_test, rename_test, redraft_test, scenefromimage_test, keys_test | all | |
| service/reconcile_test | 5, and a split of the partial scene | device misreporting, re-assert of OpenRGB devices, no server, slow-server supersede ×2, firmware put back on reconcile ×2 |
| service/zones_test | 2 | regrow ×2, misfit health |
| service/colours_test | 5 | two-colour firmware modes ×5 |
| service/correct_test | 3 | a change made elsewhere, read back |
| service/scene_test | 31 | Direct and packet order ×5, effect speed |
| service/health_test | 1 | server states ×3 |
| service/probe_test, rescan_test, writeorder_test | | probe, rescan and bounce, packet order |
| service/sanshoku_test | 4 | twin by name |
| api/api_test, cooling_test, scene_test | all but 2 | fall-through, unreachable server |
| api/guard_test | 2 (one split) | the probe route |
| daemon/reconciler_test | 3 | re-assert rule |
| daemon/reconnect_test | | redial ×2 |
| daemon/canvas_test, lights_test, lengths_test, cooler_test | all | |
| gui/preview_test | 2 | |
| cli/cli_test | 19 | fall-through, unreachable health, probe |
| cli/colours_test, parity_test | all | |
| cli/canvas_test | 2 (one split) | twin left alone |
| cli/wizard_test | 31 (one split) | Static against Direct, brightness by mode ×3, chaining (see below) |

### Found on the way

Each is canvas-path behaviour that matters now that every device is a canvas.
None is changed here (R1.4).

1. **A keyboard is asked about chaining.** `drawn.zones` gives every zone of
   more than one light the line shape, so `light map` asks how many things
   are chained on the K4 HE. `TestOnlyALineOfLightsIsAskedAboutChaining`
   stays on the fake until a zone's shape comes from the driver.
2. **A `reassert` rule never reaches a canvas.** The animator does not resend
   an unchanged frame, and canvases are not re-asserted.
3. **Firmware effects are listed without the catalogue.** Every effect but
   Off and Self-define is one colour, with no speed: a scene's speed ("at
   55") is dropped, Cycle All shows a colour, and a two-colour effect (the
   GPU's Breathing) takes one.
4. **`light map` asks about dimming on every device**, since every canvas
   mode is dimmable.
5. **The write queue's supersede** had its coverage only through a slow
   OpenRGB fake.

### PR 2: the deletion (2026-11-17)

`make test`, `make lint` (0 issues), the GUI tests and govulncheck pass. No
file imports `internal/openrgb`; `go mod tidy` drops
`github.com/csutorasa/go-openrgb-sdk`. The parity and guard tests pass with
`rescan` and `probe` gone (R2.3). 111 files, 8,587 lines removed.

- **Deleted:** `internal/openrgb`, `internal/stale`, `internal/services`; the
  service's `absent`, `probe` and `rescan`; regrow, twin matching, the settle
  and second mode packet; `Health`'s server states, address and protocol;
  `Status`'s connection; `Result.Resized`, `Attempt.Settled`; the daemon's
  dial, redial and environment; `light probe`, `light rescan`, their routes
  and the window's rescan card; the CLI and window's stale check.
- **The service's devices are its canvases** (R2.2): `machine` is four
  methods (`Devices`, `Device`, `SetMode`, `SetFrame`), served by `lights`;
  `Style` moves to `devices`; `service.New(cfg)`. A write to a device that is
  not drawn is `NoDevice`, a 404 on release; a name a rule says a canvas was
  formerly called is listed handed to it, and refused with that reason (R2.4).
- **Health** is no-devices, none-in-scope or healthy, with remedies that name
  hotaru's udev rules.
- **Rules:** `twin` loads and is ignored (R2.5); its test stays.
- **Tests:** every test PR 1 left on `openrgb.Fake` is deleted with its code;
  four were rewritten for the new contract (health ×3, nothing found).
- **Docs and packaging:** architecture (both diagrams re-rendered), hardware,
  api, packaging, credits, gallery, README, CONTRIBUTING and the example
  rules; no `openrgb` optdepend or enumeration example; the unit and
  installer comments and keywords.

### Live

Not yet run: it needs the dev build from this branch on the desk and someone
watching.
