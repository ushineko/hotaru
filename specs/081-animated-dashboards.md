# Spec 081: animated dashboards

**Issue**: [#228](https://github.com/ushineko/hotaru/issues/228)

## Status: INCOMPLETE

## Executive Summary

(Written last.)

## Context

Since spec 080 the dashboard on the Kraken Elite is a full-colour still frame,
rendered every second and sent when what it says changes. Numbers jump from
one reading to the next, the sparklines ("trails", spec 046) jump a step when
a sample joins them, and a dashboard whose background is an animated picture
shows its first frame only (`Service.behind` reads it through
`images.Library.Still`).

sanshoku 0.4.0 adds `screen.Mover`: Q565 frames, 16-bit colour, at the
device's 20 frames a second (sanshoku spec 038).

### What the prototype showed

A throwaway prototype on this branch (`cmd/proto-anim`, with experimental
`RenderLayers`, `RingsLayer`, `TrailsLayer` and a drift in `drawTrail`) drew
dashboards on the panel from made-up readings, with the maintainer watching
(2026-11-18):

| Tried | Verdict |
|---|---|
| Interpolated readings (numbers count, ring sweeps, eased) | "technically works but looks kind of bad" |
| Liquid morph: the old and new text's distance fields blended | liked |
| Gooey melt: changed text blurred, cross-faded, cut back to a hard edge | liked best |
| Odometer: changed digits roll vertically with motion blur | liked, below the melt |
| Comet ring: a spring sweep with a glowing head and fading trail | liked, as an option |
| An animated picture (rotatingearth) playing under ice1's numbers | good |
| Trails drifting left between samples, their range blending (as fynedesygn spec 070 does for hayami) | smooth, once the leaving sample was drawn at the left edge |

Two faults the prototype found, which this spec designs out: a still frame
sent at the end of a transition without the trail flashed it away; and a
drifting trail that dropped its oldest sample at once shrank and grew a step
on every sample.

Measured on the prototype: drawing a frame took 10 to 18 ms; a Q565 frame took
37 ms to send over the starfield and 62 ms over a photograph; preparing a
melt or morph took 40 to 80 ms per change, because it measured the whole
640x640 frame.

## Requirements

- **R1 Settings, per dashboard.** `Dashboard` gains `Motion`:

  ```yaml
  motion:
    numbers: melt     # "" (jump, as now), melt, morph, roll
    ring: comet       # "" (jump, as now), sweep, comet
  ```

  - R1.1 Empty is today's behaviour, and the default. The shipped dashboards
    leave both empty.
  - R1.2 Set through the existing routes (`POST /v1/dashboards/{name}`,
    `hotaru dashboard save`), shown by `hotaru dashboard show`, and chosen in
    the window's dashboard editor. An unknown value is refused with the list
    of known ones.
- **R2 Number transitions.** On a panel that takes motion frames, when what a
  dashboard's text says changes and `numbers` is set:
  - R2.1 Only the regions of text that changed move; text that did not change
    stays as it is (the prototype's melt hid every label until it did this).
  - R2.2 `melt`: within each changed region, the old and new text are
    blurred, cross-faded and cut back to a hard edge; the blur is widest
    halfway and scaled to the region's height. Over 700 ms.
  - R2.3 `morph`: the old and new text's signed distance fields are blended
    and cut at zero. Over 700 ms.
  - R2.4 `roll`: each changed region's old text rolls up and out as the new
    rolls in from below, with motion blur along the roll. Over 500 ms.
  - R2.5 Colour follows the text: each pixel takes the colour of the nearest
    old and new glyph pixel, blended along the transition.
  - R2.6 Preparing a change (finding the regions, measuring the shapes)
    works on the changed regions only, and takes at most 10 ms for the
    shipped dashboards, measured by a benchmark.
  - R2.7 A change during a transition starts a new one from what is on the
    panel.
- **R3 Ring transitions.** When `ring` is set:
  - R3.1 `sweep`: the arcs move to their new values, eased, over the number
    transition's time (500 ms when `numbers` is empty).
  - R3.2 `comet`: a spring sweep that overshoots and settles, with the arc's
    path over the last moment glowing towards white and fading.
- **R4 Drifting trails.** On a panel that takes motion frames, every trail
  drifts, whatever `motion` says:
  - R4.1 Between samples the trace moves left at a constant speed, one step
    per `readings.Bucket`; the newest sample enters at the right edge and the
    sample leaving is drawn at the left edge until it has moved out, so the
    trace keeps its length.
  - R4.2 The trace's range blends at the same rate from the old samples'
    range to the new.
  - R4.3 A frame is sent only when the trace has moved at least a pixel:
    about 2 frames a second with 5 s buckets.
  - R4.4 A frame that only drifts is a full-colour still (`Stream`), so a
    dashboard at rest keeps 24-bit colour. Motion frames (`Move`) are for
    number and ring transitions and animated backgrounds. A drift frame does
    not advance the tick.
- **R5 Animated backgrounds.** On a panel that takes motion frames, a
  dashboard whose background picture has more than one frame plays it: each
  frame the picture's next with the dashboard over it, sent with `Move`, at
  the picture's delays clamped to 50 ms, looping for as long as the dashboard
  shows and is not held. Frames are decoded and darkened once per picture and
  darkness. On a panel without `Mover` the first frame is the background, as
  now.
- **R6 Composition.** Every frame is the background, then the rings, then the
  trails, then the text, each drawn on its own layer. Rendering a still frame
  this way gives the same picture as spec 080's `Render`; the golden test
  holds.
- **R7 Holding and stopping.** A picture or the readout held over the
  dashboard stops every transition, drift and loop at once, and nothing is
  sent until the dashboard is released. Stopping the service stops it before
  the cooler closes.
- **R8 Cost, measured.**
  - R8.1 The spec records the service's CPU time over 10 minutes, with the
    frames sent, for: ice1 as spec 080 left it (drift only); ice1 with
    `numbers: melt`; load with `numbers: melt, ring: comet`; and a dashboard
    over an animated picture.
  - R8.2 `docs/architecture.md` states the background work for each.
- **R9 The prototype goes.** `cmd/proto-anim` is removed; the layer functions
  stay only as the implementation uses them.
- **R10 Docs.** `docs/api.md` (the field and its values), `docs/architecture.md`
  (R8.2), `docs/hardware.md` (motion frames on the Kraken Elite), the README
  changelog under `### Unreleased` with this spec and #228, and the move to
  sanshoku 0.4.0.

## Acceptance Criteria

- [ ] `make test`, `make lint`, `make lint-windows` and the GUI tests pass.
- [ ] Tests, with an injected clock and a fake Mover:
  - [ ] layers composed give the same pixels as `Render` for every shipped
        dashboard, still (R6);
  - [ ] changed regions: a reading that changes one number yields one region
        around it, and labels are outside every region;
  - [ ] each of melt, morph and roll: at the start the old text, at the end
        the new, unchanged text identical throughout;
  - [ ] a benchmark of preparing a change, recorded (R2.6);
  - [ ] a change with `numbers: melt` sends motion frames at 50 ms apart for
        700 ms, then a still; with `motion` empty, one still as before;
  - [ ] a change during a transition starts from the frame on the panel;
  - [ ] drift: the trace moves one step per bucket, keeps its length (the
        leaving sample drawn at the left), sends a still only when it has
        moved a pixel, and does not advance the tick;
  - [ ] an animated background sends a frame per picture frame at its delay
        clamped to 50 ms, loops, and decodes the picture once;
  - [ ] held, nothing is sent; released, the dashboard draws at once;
  - [ ] a panel without `Mover` gets spec 080's frames and a first-frame
        background;
  - [ ] `motion` round-trips through the API and the store, an unknown value
        is refused, and the shipped dashboards leave it empty.
- [ ] Live, the maintainer watching:
  - [ ] ice1 with `numbers: melt`: changed numbers melt, labels stay, the
        trails drift and keep their length, no flash;
  - [ ] load with `numbers: melt, ring: comet`: as above, and the rings sweep
        with the comet;
  - [ ] `morph` and `roll` each once;
  - [ ] a dashboard over an animated picture plays it under the numbers;
  - [ ] a picture shown and the readout chosen stop the motion; the dashboard
        comes back at once;
  - [ ] the ring lighting keeps its effect throughout and through a stop.
- [ ] Cost recorded per R8.1.
- [ ] Docs per R10.

## Risks & Assumptions

- **Cost of a busy dashboard.** The desk's readings change about once a
  second. With `melt`, about 700 ms of each second is motion at 20 frames a
  second: about 14 frames, each drawn (10 to 18 ms in the prototype) and
  sent. R8 measures it; if it is out of line, the transition shortens or its
  pace halves.
- **Drift costs every dashboard with a trail.** About 2 full-colour frames a
  second (1.2 MB each) where spec 080 sent about one. This was the
  maintainer's choice ("always on").
- **16-bit colour while moving.** Transitions and animated backgrounds are
  Q565; a dashboard at rest is full colour (R4.4).
- **Readability mid-melt.** A melt is unreadable for part of its 700 ms by
  design. A reading that changes every second spends most of its time
  settled.
- **One firmware.** As specs 037, 038 and 080.
- **Rollback.** Revert the branch: `motion` disappears from the API (clients
  ignore it); a stored `motion` is ignored by older builds, as YAML fields
  they do not know are.

## Alternatives Considered

- Considered interpolating the readings themselves (numbers counting);
  rejected after the prototype: the maintainer found it looked bad.
- Considered a constant 20 frames a second; rejected for its cost on a
  dashboard that is mostly still.
- Considered drift as part of the per-dashboard setting; the maintainer chose
  always on, for its low cost.
- Considered sending drift frames as Q565; rejected because a dashboard with
  a trail would then never be in full colour.

## Verification

(Results pasted here.)
