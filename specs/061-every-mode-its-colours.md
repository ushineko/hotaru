# Spec 061: every mode its colours

**Issue**: [#177](https://github.com/ushineko/hotaru/issues/177)

## Status: INCOMPLETE

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

- [ ] `make test`, `make lint` and the canaries pass; a 0.1.21 `scenes.yml`
      loads and round-trips with no change.
- [ ] Render tests per R2's table: each renderer with none, one and several
      colours, as pure functions.
- [ ] `openrgb.Fake` gains `colors_min`/`colors_max` per mode; a test shows a
      two-colour firmware Breathing written with both colours, and a
      reconcile writing both back.
- [ ] A palette test: a picture made of known blocks of colour gives those
      colours, in coverage order, and `distance` merges near ones.
- [ ] CLI, API and window parity for the new colours (`parity_test.go`).
- [ ] **Live, on cachyos with someone watching:** a Breathing between two
      colours on the Apex; a firmware mode with two colours on a device that
      offers one (the GPU or a board header, chosen during the run); Make a
      scene from a picture with Breathing on the Apex runs in the picture's
      colours.
- [ ] Docs: `docs/api.md` (effect colours), `docs/hardware.md` if a device's
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

To be filled.
