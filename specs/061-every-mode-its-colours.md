# Spec 061: every mode its colours

**Issue**: [#177](https://github.com/ushineko/hotaru/issues/177)

## Status: INCOMPLETE

Every criterion is met except the live run on cachyos, which needs someone
watching the hardware.

## Executive Summary

An effect carries a list of colours, and every mode says how many it takes:
firmware modes as OpenRGB reports `colors_min`/`colors_max`, hotaru's
renderers as they draw (Breathing 0-4, Spectrum and Rainbow Wave 0-8). Several
colours fill a firmware mode's slots in order and are put back by reconcile;
the editor and Make a scene show a picker per slot; Make a scene picks the
picture's most prominent colours and marks them so a recolour picks again.
One-colour scenes are written exactly as 0.1.21 wrote them.

Look first at `openrgb.Slots` (how slots are filled, shared with the fake),
`modeColoursFor` in `internal/service/service.go` (why a renderer that needs
no colours is not handed the frame's), and `images.Palette`. Then the Gaps
found, which record what this spec did not say.

## Context

A scene's effect carries one colour at most. `scenes.Effect` has `Mode`,
`Colour` and `Speed`, and the editor offers the colour only for a mode that
"shows one colour" (`oneColour`, `effectColour` in `internal/gui/scenes.go`).
Two kinds of device want more than that:

- **hotaru's own renderers** (spec 060). Breathing scales one colour; it could
  fade between two. Spectrum cycles every hue; it could cycle through the
  colours somebody picked. Rainbow Wave could be a gradient of picked colours.
  These are hotaru's code, so the number of colours is hotaru's choice.
- **Firmware modes** that take several colours. OpenRGB describes each mode's
  colours with `colors_min`, `colors_max` and a colour mode (none, per-LED,
  mode-specific, random). hotaru reads only the first mode colour and writes
  one (`modeColours` in `internal/openrgb/conn.go`). A device whose Breathing
  alternates two colours in firmware cannot be given the second today.

"Make a scene" builds a scene from a picture or a dashboard
(`SceneFromImage`, `SceneFromDashboard`). Its effects are the caller's choice
(scenefromimage.go:34), so an effect chosen there runs in its default colour
rather than the picture's. The picture already knows its colours; the scene
should use them.

## Requirements

### R1. Colours on an effect

- R1.1 `scenes.Effect` gains `Colours []string`. `Colour` stays as the
  one-colour form and is read as `Colours: [Colour]`. A scene written by
  0.1.21 loads unchanged, and a scene with one colour is written back in the
  short form.
- R1.2 The API's effect carries `colours`. The CLI's `--effect-colour` takes a
  comma-separated list (`device="#ff0000,#0000ff"`). `scene show` prints them.
- R1.3 Each mode says how many colours it takes, as `ColoursMin` and
  `ColoursMax` on `devices.Mode` (0 and 0 is a mode that takes none). Firmware
  modes read them from OpenRGB; hotaru's renderers declare their own (R2).

### R2. hotaru's renderers

| Renderer | Colours | With none | With one | With several |
|---|---|---|---|---|
| Static | 0 | the frame's colours, as now | | |
| Breathing | 0-4 | the frame's colours, as now | that colour fades in and out, as now | fades from each to the next in turn |
| Spectrum | 0-8 | every hue, as now | | cycles through them in order |
| Rainbow Wave | 0-8 | the full rainbow, as now | | a moving gradient through them, wrapping |
| Off | 0 | | | |

Pure functions in `internal/render`, as the rest. Speed means the same as it
does now.

### R3. Firmware modes

- R3.1 `SetMode` fills the mode's colour slots from the effect's colours, up
  to `colors_max`, for a mode whose colour mode is mode-specific. Fewer
  colours than `colors_min` repeats the last one, as `modeColours` already
  does for one.
- R3.2 Reconcile and re-assert put back every colour, not only the first.
  Desired state records them.

### R4. The editor

- R4.1 Every mode that takes colours shows that many colour pickers in the
  scene editor and in Make a scene's effect fields: one per slot, the first
  `ColoursMin` required, up to `ColoursMax`, with a control to add or remove a
  slot between the two.
- R4.2 Choosing colours never depends on the mode showing one colour. A mode
  that takes a colour per light (Static, Direct) still shows the picture of
  the device, as now.

### R5. Make a scene picks the colours

- R5.1 When Make a scene sets an effect that takes colours, and the caller
  did not give them, hotaru picks them from the picture: the most prominent
  distinct colours, as many as the mode's `ColoursMax` up to four, ordered by
  how much of the picture each covers. The `distance` setting the dialog
  already has decides what counts as distinct.
- R5.2 Recolour (`scene recolour`) picks them again from the new picture or
  dashboard, unless the scene's colours were chosen by hand. A scene records
  which it was (`colours_from: picture` or absent).
- R5.3 The dialog shows the picked colours before the scene is made, and
  they can be changed there.

## Acceptance Criteria

- [x] `make test`, `make lint` and the canaries pass; a 0.1.21 `scenes.yml`
      loads and round-trips with no change.
- [x] Render tests per R2's table: each renderer with none, one and several
      colours, as pure functions.
- [x] `openrgb.Fake` gains `colors_min`/`colors_max` per mode; a test shows a
      two-colour firmware Breathing written with both colours, and a
      reconcile writing both back.
- [x] A palette test: a picture made of known blocks of colour gives those
      colours, in coverage order, and `distance` merges near ones.
- [x] CLI, API and window parity for the new colours (`parity_test.go`).
- [ ] **Live, on cachyos with someone watching:** a Breathing between two
      colours on the Apex; a firmware mode with two colours on a device that
      offers one (the GPU or a board header, chosen during the run); Make a
      scene from a picture with Breathing on the Apex runs in the picture's
      colours.
- [x] Docs: `docs/api.md` (effect colours), `docs/hardware.md` if a device's
      behaviour is documented there, the README changelog.

## Risks & Assumptions

- **What OpenRGB reports.** Assumes the SDK's `ModeColorsMin`,
  `ModeColorsMax` and `ModeColorMode` are filled in by every controller that
  has mode colours. A device that reports 0 and 0 gets no pickers and is
  written as now. Measured per device in the live run.
- **Scene format.** Additive: `colours` and `colours_from` are new keys. An
  older hotaru reading a newer scene ignores them and uses `colour`, which is
  still written as the first of the colours.
- **Picking colours.** A picture with one dominant colour gives fewer
  distinct colours than the mode takes; the rest repeat the last, and the
  dialog shows that.
- **Rollback**: revert. A scene saved with several colours keeps working on
  an older build with its first colour.

## Alternatives Considered

Considered a fixed second colour field (`colour2`) instead of a list;
rejected because firmware modes take up to OpenRGB's `colors_max` (eight and
more on some controllers) and the canvas renderers do not have a natural
limit of two.

## Verification

### Mode colours as reported

Read from OpenRGB on 2026-10-01 with a read-only probe (controller data
only: no mode, colour or frame was written). Flags in hex; colour mode 1 is
per-LED, 2 mode-specific, 3 random. Only modes with `colors_max` of 2 or more
are listed, plus the shapes that matter for the edge cases.

The development machine:

| Device | Mode | Flags | Colour mode | Min | Max | Held |
|---|---|---|---|---|---|---|
| MSI RTX 4090 Suprim Liquid X | Color Cycle | 0x151 | 2 | 1 | 3 | 3 |
| MSI RTX 4090 Suprim Liquid X | Wave | 0x153 | 2 | 1 | 3 | 3 |
| MSI RTX 4090 Suprim Liquid X | Fade In | 0x151 | 2 | 1 | 2 | 2 |
| MSI RTX 4090 Suprim Liquid X | Breathing | 0x151 | 2 | 1 | 2 | 2 |
| NZXT Kraken 2024 Elite | Fading, Cover Marquee, Pulsing, Breathing, Candle | 0x041-0x061 | 2 | 1 | 8 | 1 |
| NZXT Kraken 2024 Elite | Alternating | 0x043 | 2 | 1 | 2 | 1 |

The ASUS Z790 Hero, G502 X Plus and Keychron K4 HE report one and one for
every mode-specific mode; the MM700 has Direct only.

cachyos:

| Device | Mode | Flags | Colour mode | Min | Max | Held |
|---|---|---|---|---|---|---|
| Corsair Dominator DDR5 (x4) | Color Shift, Color Pulse | 0x2d1 | 2 | 2 | 2 | 2 |
| Corsair Dominator DDR5 (x4) | Color Wave, Visor, Rain | 0x2d5-0x2d9 | 2 | 2 | 2 | 2 |
| EVGA RTX 3080 FTW3 Ultra LHR | Breathing | 0x151 | 2 | 1 | 2 | 1 |
| EVGA RTX 3080 FTW3 Ultra LHR | Color Cycle, Color Stack | 0x151/0x153 | 2 | 2 | 7 | 2 |
| EVGA RTX 3080 FTW3 Ultra LHR | Direct | 0x130 | 1 | 1 | 1 | 0 |
| Razer Mouse Dock Pro | Breathing | 0x0d0 | 2 | 1 | 2 | 1 |

Both Z790 Aorus Master X controllers report one and one; the Apex lists
Direct and Onboard with none.

**No device reports zero and zero while having mode colours.** Every mode
with the mode-specific flag reports a minimum of at least one. Two readings
change the design: the Kraken's modes hold one colour while allowing eight,
so the slot count comes from `colors_max` and not from the colours held; and
the EVGA's Direct reports one and one beside a per-LED colour mode, so the
counts are read only for a mode with the mode-specific flag.

### Checks

| Command | Result |
|---|---|
| `go vet ./...` and with `-tags migrated_fynedo` on `./internal/gui/...` | clean |
| `make test` | every package ok |
| `go test -tags migrated_fynedo ./internal/gui/...` | ok |
| `go test -race ./internal/canvas/... ./internal/service/... ./internal/daemon/...` | ok |
| `GOLANGCI_LINT_CACHE=$PWD/.cache/golangci-lint make lint` | 0 issues |
| `make build` | builds without cgo |

Tests added: `scenes` (0.1.21 file round-trips byte for byte; several
colours written as a list with the first as `colour`; `colours_from`),
`openrgb` (`Slots` with the measured shapes; `convert` reads min, max and
every held colour; per-LED Direct gets no slots), `render` (R2's table, a
renderer at a time), `canvas` (the same effect in other colours is a new
show), `state` (several colours survive a restart; an old file reads as one),
`images` (blocks in coverage order; distance merges red and orange; colour
outweighs a dark background), `service` (two-colour firmware Breathing
written with both and read back; reconcile writes both; one colour fills a
two-slot mode; a canvas Spectrum is not handed the frame's colour; Make a
scene picks in coverage order; recolour picks picked colours again and keeps
chosen ones), `api` (the listing's `coloured`), `cli` (comma list; `image
colours`; parity covers both new routes), `gui` (a second slot offered,
removed and cleared; Make a scene shows the picked colours and sends them
marked).

### Live run

Not yet run. The commands for it, on cachyos:

```sh
# A Breathing between two colours on the Apex (hotaru draws it).
hotaru scene write duo "Apex Pro TKL Wireless Gen 3=#202020" \
    --effect "Apex Pro TKL Wireless Gen 3=Breathing" \
    --effect-colour "Apex Pro TKL Wireless Gen 3=#ff0000,#0000ff"
hotaru scene apply duo

# A firmware mode with two colours: the EVGA card's Breathing (1 to 2), or a
# DDR5 stick's Color Shift (exactly 2).
hotaru scene write gpu-duo "EVGA=#202020" --effect "EVGA=Breathing" \
    --effect-colour "EVGA=#ff0000,#00ff00"
hotaru scene apply gpu-duo

# Make a scene from a picture with Breathing on the Apex.
hotaru image colours <picture> --count 4
hotaru image scene <picture> pictured --effect "Apex Pro TKL Wireless Gen 3=Breathing"
hotaru scene show pictured
hotaru scene apply pictured
```

### Gaps found

- **Several colours are written as `colours` and their first as `colour`
  too.** R1.1 says one colour goes in the short form; the Risks section says
  an older build reads `colour`. Both hold only if the first colour is
  written twice for a scene with several. On read, `colours` wins.
- **`--effect-colour` was a string slice, which splits on commas.** R1.2's
  `device="#ff0000,#0000ff"` could not reach the scene through it. It is a
  string array now, and the value is split on commas by hotaru.
- **Make a scene's colours needed a route of their own.** R5.3 shows the
  colours before the scene exists, which no route offered. `POST
  /v1/images/{name}/colours` and `/v1/dashboards/{name}/colours` return them,
  reachable as `hotaru image colours` and `hotaru dashboard colours`.
- **The mode's colour mode is set to mode-specific when colours are
  written.** A mode that also offers random colours and was left in them
  ignores every slot. R3.1 assumed the mode was already mode-specific.
- **The frame's colour is the fallback only for a mode that shows one of its
  own.** Before, every non-per-LED mode was handed the frame's colour, and
  firmware ignored it. Spectrum and Rainbow Wave now draw from the colours
  they are handed, so the fallback would have held them still in one colour.
- **R2's "with one" column is blank for Spectrum and Rainbow Wave.** One
  colour there holds that colour on every light, which is a loop of one
  colour; the editor still offers it, as Most allows.
- **Several colours on Breathing breathe in turn, through dark.** R2 says
  "fades from each to the next in turn". A crossfade with no dark between
  colours is Spectrum through the same colours, so Breathing keeps the dark,
  as the two-colour firmware Breathings do.
- **Make a scene picks for every mode that takes colours, not only modes
  with several.** A one-colour mode used to get the dominant colour of that
  device's own frame; it now gets the picture's most prominent colour, the
  same way. The two agree for most pictures.
- **Picked colours are not pushed apart by the distance.** The distance
  decides which colours count as distinct, at 64 RGB units per step of
  distance from 1. Separating the picked colours as well would move them off
  the picture's own colours for no gain a test could show.
- **A per-light mode is left out of `coloured` even when it carries the
  mode-specific flag.** Its colour is the frame (R4.2), and the fake already
  treated its own slot as spare.
