# Spec 081: animated dashboards

**Issue**: [#228](https://github.com/ushineko/hotaru/issues/228)

## Status: INCOMPLETE

## Executive Summary

(Written last.)

## Context

Since spec 080 the dashboard on the Kraken Elite is a full-colour still frame,
rendered every second and sent when what it says changes. Numbers jump from
one reading to the next. A dashboard whose background is an animated picture
shows its first frame only: `Service.behind` reads the picture through
`images.Library.Still`, which for a GIF is the first frame.

sanshoku 0.4.0 adds `screen.Mover`: Q565 frames, 16-bit colour, at the
device's 20 frames a second (sanshoku spec 038). On the bench a frame took
about 45 ms to send. Encoding a 640x640 frame took 1 to 6 ms, and hotaru renders
a dashboard in about 3 ms (spec 080).

The maintainer chose, for this spec:

- **Tween on change.** When a reading changes, the dashboard moves to the new
  value over about half a second at 20 frames a second, then is still again.
- **Per dashboard.** Smooth motion is a dashboard's own setting, off by
  default, so existing dashboards keep their cost.

## Requirements

- **R1 A dashboard setting.**
  - R1.1 `Dashboard` gains `Smooth bool` (`smooth` in YAML and the API), off
    by default. The shipped dashboards leave it off.
  - R1.2 It is set through the existing routes (`POST /v1/dashboards/{name}`,
    `hotaru dashboard save`), shown by `hotaru dashboard show`, and set in
    the window's dashboard editor.
- **R2 Tweening.** On a panel that takes motion frames (`screen.Mover`), for a
  dashboard with `Smooth`:
  - R2.1 When the rendered content changes, the pusher draws the change over
    `tween` (500 ms) at the motion pace (50 ms, 20 frames a second): each
    frame renders the reading interpolated between the one shown and the new
    one, and is sent with `Move`.
  - R2.2 Interpolated: every number a reading carries, so a ring sweeps, a
    bar slides, a number counts and a grade colour follows. A source present
    in only one of the two readings is not interpolated; it shows as the new
    reading has it.
  - R2.3 When the tween ends, the final frame (the new reading itself) is sent
    as a full-colour still with `Stream`, as spec 080 sends every frame, and
    the 1 s cycle and the 25 s resend resume.
  - R2.4 A reading that changes during a tween starts a new tween from the
    value on screen, not from the old reading.
  - R2.5 The tick advances once per change, not per tween frame.
- **R3 An animated background.** On a panel that takes motion frames, a
  dashboard whose background picture has more than one frame:
  - R3.1 plays the picture: each frame is the picture's next frame with the
    dashboard drawn over it, sent with `Move`, at the picture's own delays,
    no faster than the motion pace;
  - R3.2 loops for as long as the dashboard is on the panel and not held;
  - R3.3 tweens its readings as R2 when `Smooth` is set, and otherwise draws
    each new reading from the next frame;
  - R3.4 decodes and darkens the picture's frames once per picture and
    darkness, not per frame, and drops them when the dashboard changes.
  - R3.5 On a panel that takes only stills (no `Mover`), the first frame is
    the background, as now.
- **R4 Holding and stopping.** A picture or the readout held over the
  dashboard stops tweening and looping at once, and nothing is sent until the
  dashboard is released (spec 080 R3.3). Stopping the service stops the loop
  before the cooler closes.
- **R5 Cost, measured.**
  - R5.1 The spec records the service's CPU time over 10 minutes for: a
    dashboard with `Smooth` on the desk's readings; a dashboard over an
    animated picture; and, for comparison, the same dashboard still. Each
    with the frames sent.
  - R5.2 `docs/architecture.md` states the background work: none extra for a
    still dashboard, a 500 ms burst per change for a smooth one, a
    continuous loop at the picture's pace for an animated background.
- **R6 Docs.** `docs/api.md` (the field), `docs/architecture.md` (R5.2),
  `docs/hardware.md` (motion frames on the Kraken Elite), the README
  changelog under `### Unreleased` with this spec and #228, and the move to
  sanshoku 0.4.0.

## Acceptance Criteria

- [ ] `make test`, `make lint`, `make lint-windows` and the GUI tests pass.
- [ ] Tests, with an injected clock and a fake Mover:
  - [ ] a reading interpolated halfway is halfway for every shared source;
        a source in only one reading takes the new value;
  - [ ] a smooth dashboard's change sends motion frames over 500 ms at 50 ms
        apart, then one still frame; a dashboard without `Smooth` sends one
        still frame as before;
  - [ ] a change during a tween starts from the value on screen;
  - [ ] the tick advances once per change;
  - [ ] an animated background sends a frame per picture frame at its delay,
        clamped to 50 ms, loops, and decodes the picture once;
  - [ ] held, nothing is sent, tween or loop; released, the dashboard draws
        at once;
  - [ ] a panel without `Mover` gets still frames and a first-frame
        background, as in spec 080;
  - [ ] `smooth` round-trips through the API and the store, and the shipped
        dashboards do not set it.
- [ ] Live, the maintainer watching:
  - [ ] a smooth dashboard: the ring sweeps and the numbers count on each
        change, then hold still; no blink, no readout;
  - [ ] a dashboard over an animated picture: the picture plays at its own
        pace under the numbers;
  - [ ] a picture shown and the readout chosen stop the motion, and the
        dashboard comes back at once;
  - [ ] the ring keeps its effect through all of it, and through a stop.
- [ ] Cost recorded per R5.1.
- [ ] Docs per R6.

## Risks & Assumptions

- **Cost of a busy dashboard.** The desk's dashboards change about once a
  second, so a smooth one spends about half of each second tweening: about 10
  motion frames a second, each a render (3 ms) and an encode (1 to 6 ms). R5
  measures it; if it is out of line, the tween shortens or the pace halves.
- **Cost of an animated background.** Continuous for as long as it shows, at
  the picture's pace. Chosen by choosing such a picture, and measured.
- **16-bit colour while moving.** Motion frames are Q565. The still frame at
  the end of a tween is full colour, so a dashboard at rest looks as it does
  today. An animated background is always 16-bit; its picture was already a
  256-colour GIF.
- **Interpolation of steps.** A reading that jumps (a fan from 0 to 1200 RPM)
  counts through the values between. That is the intent of "smooth".
- **One firmware.** As specs 037, 038 and 080.
- **Rollback.** Revert the branch: `smooth` disappears from the API (clients
  ignore it) and from new saves; a stored `smooth: true` is ignored by older
  builds, as YAML fields they do not know are.

## Alternatives Considered

- Considered a constant 20 frames a second; rejected by the maintainer for
  its cost on a dashboard that is mostly still.
- Considered smooth motion on by default; rejected by the maintainer, so
  existing dashboards keep spec 080's cost.
- Considered sending tween frames as full-colour stills; rejected because a
  raw frame is 1.2 MB and the device takes about 10 a second at best.

## Verification

(Results pasted here.)
