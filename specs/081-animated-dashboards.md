# Spec 081: animated dashboards

**Issue**: [#228](https://github.com/ushineko/hotaru/issues/228)

## Status: COMPLETE

## Executive Summary

Dashboards on the Kraken Elite now move. A dashboard's `motion` melts, morphs
or rolls the numbers that changed and sweeps its rings, at up to 20 frames a
second; the trail drifts between samples on every dashboard, at a 1 s
interval by default; an animated picture behind a dashboard plays. Every
frame on the panel is Q565. Drift costs what spec 080's still dashboard did
(about 5% of a core); motion, which is opt-in, costs 13 to 16%. Review
`internal/dashboard/animate.go` (the loop) and `internal/motion` (the
effects) first.

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
    about 2 frames a second.
  - R4.4 **Every frame on a panel that takes motion frames is a motion frame
    (`Move`)**, at rest and drifting as much as moving: tens of kilobytes
    where a full-colour frame is 1.2 MB, and 16-bit colour is not told apart
    from 24-bit on the Kraken Elite's panel (the maintainer, 2026-11-18). A
    drift frame does not advance the tick. *(Revised: the first draft kept
    drift frames as full-colour stills.)*
  - R4.5 **The trail's interval is the dashboard's**: `trail.every`, 1, 2 or
    5 seconds (default 1; revised from 5 after the live run), over the same
    5-minute span. The history
    keeps 1-second buckets for the span (300 a source), and a dashboard's
    trail averages them into its interval: 300, 150 or 60 points. The trace
    is drawn with as many points as its interval gives, so a shorter
    interval is finer detail and a sooner newest sample, and the drift's
    speed (about 2 px a second) and frame cost do not change. Set like
    `motion` (R1.2); any other value is refused.
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
  - R8.0 Drift, on every dashboard with a trail, costs no more CPU than spec
    080's still dashboard did. *(Added after the first cost run found 8.5%
    of a core against about 5.5%.)*
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

- [x] `make test`, `make lint`, `make lint-windows` and the GUI tests pass.
- [x] Tests, with an injected clock and a fake Mover:
  - [x] layers composed give the same pixels as `Render` for every shipped
        dashboard, still (R6);
  - [x] changed regions: a reading that changes one number yields one region
        around it, and labels are outside every region;
  - [x] each of melt, morph and roll: at the start the old text, at the end
        the new, unchanged text identical throughout;
  - [x] a benchmark of preparing a change, recorded (R2.6);
  - [x] a change with `numbers: melt` sends motion frames at 50 ms apart for
        700 ms, the last drawing the new reading; with `motion` empty, one
        frame as before;
  - [x] a change during a transition starts from the frame on the panel;
  - [x] drift: the trace moves one step per bucket, keeps its length (the
        leaving sample drawn at the left), sends a frame only when it has
        moved a pixel, and does not advance the tick;
  - [x] the history keeps 300 one-second buckets; a trail at `every` 1, 2
        and 5 has 300, 150 and 60 points, each the mean of its buckets, over
        the same span, so a 5 s trail spans the band a 1 s one does; the
        default is 1 s; every second has a reading (no gaps in a 1 s trail);
  - [x] an animated background sends a frame per picture frame at its delay
        clamped to 50 ms, loops, and decodes the picture once;
  - [x] held, nothing is sent; released, the dashboard draws at once;
  - [x] a panel without `Mover` gets spec 080's frames and a first-frame
        background;
  - [x] `motion` round-trips through the API and the store, an unknown value
        is refused, and the shipped dashboards leave it empty.
- [x] Live, the maintainer watching:
  - [x] ice1 with `numbers: melt`: changed numbers melt, labels stay, the
        trails drift and keep their length, no flash;
  - [x] load with `numbers: melt, ring: comet`: as above, and the rings sweep
        with the comet;
  - [x] `morph` and `roll` each once;
  - [x] ice1 with `trail.every: 1`: a finer trace that still drifts
        smoothly;
  - [x] a dashboard over an animated picture plays it under the numbers;
  - [x] a picture shown and the readout chosen stop the motion; the dashboard
        comes back at once;
  - [x] the ring lighting keeps its effect throughout and through a stop.
- [x] Cost recorded per R8.1; drift within spec 080's (R8.0).
- [x] Docs per R10.

## Risks & Assumptions

- **Cost of a busy dashboard.** The desk's readings change about once a
  second. With `melt`, about 700 ms of each second is motion at 20 frames a
  second: about 14 frames, each drawn (10 to 18 ms in the prototype) and
  sent. R8 measures it; if it is out of line, the transition shortens or its
  pace halves.
- **Drift costs every dashboard with a trail.** About 2 frames a second
  where spec 080 sent about one. This was the maintainer's choice ("always
  on"), and R8.0 holds it to spec 080's CPU.
- **16-bit colour throughout.** Every frame on the Kraken Elite is Q565
  (R4.4). A panel that takes only full-colour frames still gets them.
- **A 1-second trail is noisier.** The buckets average a reading's samples
  (spec 046) because a processor's temperature is spiky; at one second there
  is little to average. The maintainer chose 1 s as the default; 2 s and 5 s
  remain.
- **Readings shared for half a second.** `Service.Readings` hands a reading
  under 500 ms old to a second asker, as a copy, rather than reading every
  sensor again. The second asker each second was the trail.
- **Memory of an animated background.** Its frames are decoded and kept
  while it shows: 640x640 at one byte a pixel, about 400 KB a frame, so a
  150-frame picture is about 60 MB. The cache holds one picture.
- **History memory.** 300 buckets a source instead of 60: eleven sources,
  a few tens of KB.
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
- Considered drift frames as full-colour stills, so a dashboard at rest kept
  24-bit colour; the first cost run measured them, and the maintainer chose
  Q565 throughout, since the panel does not show the difference.

## Verification

**Checks** (2026-11-19, on the branch): `make test`, `make lint`,
`make lint-windows` and `go test -tags migrated_fynedo ./internal/gui/...`
pass. `govulncheck`: no vulnerabilities found.

**Tests** for the criteria: `internal/dashboard/motion_test.go`
(`TestLayersComposeToTheFrame`, `TestAnUnknownMotionOrIntervalIsRefused`,
`TestTheShippedDashboardsDoNotMove`, `TestAFinerTrailSpansTheSameBand`),
`internal/dashboard/animate_test.go` (melt frames to the end, no motion
jumps, a change mid-transition, drift a pixel at a time, drift keeps its
length, an animated background plays, held does not move, every second has
a reading, a slow panel gets frames back to back, a panel without motion
gets stills), `internal/motion/motion_test.go` (one region per changed
number, each transition old to new, the comet), `internal/readings`
(intervals average their buckets, the default is 1 s),
`internal/service` (a backdrop decoded once, a reading shared for half a
second, a still picture decoded once), `internal/api/motion_test.go`.

**Preparing a change** (`BenchmarkPrepare`, i9-14900K): melt 4.1 ms, morph
5.4 ms, roll 2.8 ms, within R2.6's 10 ms.

**Live**, the maintainer watching the desk's Kraken Elite (2026-11-19):

| Check | Answer |
|---|---|
| ice1 with `numbers: melt`, 1 s trails | "Something off": gaps in the trail. Readings were moved to a fixed point in each second (`readOffset`); then "looks better" |
| load with melt and comet | "All good" |
| morph | worked, "a bit choppy": the loop waited a full 50 ms after a slow send. It now waits the rest of the pace |
| roll, after the pacing fix | "Smooth now" |
| ice1 over the spinning earth | "Plays well" |
| a picture shown over it | "Stays put"; the readout chosen, "Stays" |
| the dashboard released | "Back at once" |
| the service stopped 10 s and started | "All fine": readout while stopped, ring effect unchanged, dashboard and effect back |

The optimisations after these runs change no pixel: the golden frames match
and the composite, arc and darkening code is checked against them.

**Cost** (R8.1), the service's CPU over 600 s per dashboard, desk machine,
with an NVIDIA card (each reading runs `nvidia-smi`):

| Dashboard | First run | After optimising | After sharing readings | Frames a second |
|---|---|---|---|---|
| tree-full (drift only, photograph) | not measured | 50.8 s | 28.9 s (4.8%) | ~2 |
| ice1 (drift only) | 51.0 s | 51.1 s | 31.4 s (5.2%) | ~2 |
| ice1, `numbers: melt` | 114 s | 94.5 s | 76.7 s (12.8%) | ~10 |
| load, melt and comet | 218 s | 111 s | 97.5 s (16.3%) | ~14 |
| ice1 over an animated picture | 174 s | 92.2 s | 79.1 s (13.2%) | ~18 |

Spec 080's still dashboard measured about 5 to 6% of a core. The profile of the
optimised build found half the service's own work outside drawing: the
sensors were read twice a second (once for the reading, once for the trail)
and the picture decoded every second. R8.0 holds.
