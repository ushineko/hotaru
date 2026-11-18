# Spec 080: dashboards in full colour, as frames

**Issue**: [#226](https://github.com/ushineko/hotaru/issues/226)

## Status: INCOMPLETE

## Executive Summary

(Written last.)

## Context

The dashboard reaches the Kraken's screen as a GIF uploaded to a bucket
(`dashboard.Pusher`, spec 013). Two costs follow from the format:

- **256 colours.** `palette` builds every frame from the theme's colours, a
  24-step ramp for the starfield and at most 236 colours of a photograph.
  Gradients band, and a photograph is posterised.
- **A settling floor.** A pushed GIF only appears if the next push waits 1 to
  3 s, by file size (`ImageFloor`). The palette is kept short partly to keep
  files small enough for a short floor.

sanshoku spec 037 (ushineko/sanshoku#102) adds a second path on the Kraken
Elite, `screen.Framer`. A raw RGB frame shows at once, in full colour, with no
floor; about 95 ms per frame on the bench. The firmware takes the panel back
for its readout 30 s after the last frame, unasked, so a still picture is sent
again within `FrameHold` (25 s). A resend shows no visible change (bench,
2026-11-18).

The maintainer chose full colour now, rather than a transport-only change, and
a check every second.

## Requirements

- **R1 Full-colour painting.**
  - R1.1 The dashboard is painted on an RGBA canvas. The starfield, the plain
    background and a picture background are drawn without quantising.
  - R1.2 The starfield is drawn once per theme and cached, as now, as RGBA.
  - R1.3 A picture background is darkened as now, and kept in full colour.
  - R1.4 `Content` (what the frame says, without the tick) is a hash of the
    RGBA pixels, so the push gate works as before.
- **R2 The GIF, made from the canvas.**
  - R2.1 A rendered frame offers its GIF on demand, quantised from the RGBA
    canvas through the same palette as today (theme colours, chosen text
    colours, the ramp, the picture's most common colours, at most
    `MaxColours`), to the nearest colour, without dithering.
  - R2.2 Quantising is fast enough to run every cycle on the GIF path: at most
    30 ms per frame on the development machine, measured by a benchmark.
  - R2.3 A panel that does not take frames gets the GIF and its floor exactly
    as now. The GIF of a dashboard looks as it did before this spec, within
    the nearest-colour rounding of anti-aliased text edges.
- **R3 Frames on a panel that takes them.**
  - R3.1 `cooler.Cooler` reports whether its panel takes frames, draws one
    (`Frame`), and gives `FrameHold`.
  - R3.2 The pusher, on such a panel, renders every second, sends the RGBA
    frame when its content changes, and sends the last frame again when
    `FrameHold` has passed since the last send. The tick advances on a
    changed frame only, not on a resend.
  - R3.3 While the dashboard is held (a picture, the readout), the pusher
    sends nothing, resends included. On release it sends at once, as now.
  - R3.4 A frame that fails is reported and retried on the next cycle, as a
    failed GIF push is now. `ErrNoScreen` stops the loop, as now.
  - R3.5 `hotaru screen show` and a scene's picture still go to a bucket as a
    GIF: the cooler plays a GIF by itself, at no cost to the host.
- **R4 The preview shows what the panel will show.**
  - R4.1 `POST /v1/dashboards/{name}/preview` returns a PNG of the RGBA frame
    when the cooler's panel takes frames, and the GIF otherwise. The
    response gains `format` (`"png"` or `"gif"`). `floor_seconds` is 0 for
    frames.
  - R4.2 The window's preview, the scene-from-dashboard colours and
    `hotaru dashboard preview --to` read either format.
- **R5 Cost, measured.**
  - R5.1 The spec records, on the desk: the service's CPU time over
    10 minutes with the dashboard on frames, against the same on 2.0.0 with
    GIFs; and the frames sent in those 10 minutes.
  - R5.2 `docs/architecture.md` states the background work: a render every
    second, a frame on change, a resend every 25 s.
- **R7 Pictures in full colour.** Added during the live run: the image
  library stores only a 640x640 GIF of each picture, so a photograph behind a
  dashboard was at 256 colours before the dashboard drew it.
  - R7.1 Adding a still picture also stores a full-colour PNG, cropped and
    scaled as the GIF is. An animation or a slideshow has no copy.
  - R7.2 Replacing, renaming and removing a picture do the same to its copy.
  - R7.3 A dashboard's picture background reads the copy where there is one,
    else the GIF. A picture added before this spec is in full colour once it
    is added again from its original.
- **R6 Docs.** `docs/api.md` (the `format` field), `docs/hardware.md` (the
  Kraken Elite's screen takes frames; the 30 s return), `docs/architecture.md`
  (R5.2), README changelog under `### Unreleased` with this spec and #226.

## Acceptance Criteria

- [ ] `make test`, `make lint` and the GUI tests
      (`go test -tags migrated_fynedo ./internal/gui/...`) pass.
- [ ] Tests:
  - [ ] a dashboard rendered twice with the same reading has the same
        `Content`; a changed reading changes it; the tick does not;
  - [ ] a gradient (the starfield) renders with more than 256 distinct
        colours in the RGBA frame, and the GIF of the same frame has at most
        256 and decodes at 640x640;
  - [ ] a quantising benchmark, run and recorded in Verification (R2.2);
  - [ ] the pusher on a framing fake: sends on a change, does not send an
        unchanged frame within `FrameHold`, resends it after `FrameHold`
        (injected clock), sends nothing while held, sends at once on
        release, and keeps the tick on a resend;
  - [ ] the pusher on a GIF-only fake behaves as before (existing tests);
  - [ ] the preview route returns `format: "png"` and a decodable PNG with a
        framing cooler, and `"gif"` without one.
- [ ] Live, the maintainer watching:
  - [ ] the dashboard on the panel in full colour (starfield and a picture
        background), compared with 2.0.0's GIF;
  - [ ] a reading that changes shows within about a second;
  - [ ] idle for at least 2 minutes: no return to the readout, no blink at
        the resends;
  - [ ] `hotaru screen show` a GIF, then back to the dashboard: the picture
        stays (no resend over it), and the dashboard returns at once;
  - [ ] the readout chosen, then the dashboard: the same;
  - [ ] hotaru stopped: the readout returns, and the ring is unchanged;
  - [ ] with the ring on an animated effect, the dashboard running for
        2 minutes: the ring does not change at the frames, the resends or
        the stop (moved here from sanshoku spec 037);
  - [ ] the window's preview of a dashboard matches the panel.
- [ ] Tests for R7: a still is kept and read in full colour; an animation has
      no copy, and replacing a still with one removes the old copy; the copy
      follows a rename and a remove; a picture without a copy reads its GIF.
- [ ] Live: a picture added again from its original shows in full colour
      behind a dashboard.
- [ ] Cost recorded per R5.1.
- [ ] Docs per R6.

## Risks & Assumptions

- **The 30 s return.** Measured on firmware 1.2.12. A shorter return on a
  later firmware would flash the readout between resends; the sanshoku
  bench's `--no-resend` run checks it after an update.
- **Cost.** A frame is about 1.2 MB against a GIF of 7 to 20 KB. Sends happen
  only on a change, at most one a second, plus one every 25 s when idle. A
  coolant reading moves in tenths, so a busy machine may send close to one a
  second. R5 measures it rather than assumes it.
- **The GIF path re-quantises every cycle.** Today the starfield is quantised
  once and copied (7.8 ms against 128 ms, `background`'s comment). R2.2 sets
  a budget, with a colour cache in the quantiser to meet it; the GIF path
  runs only on a panel that does not take frames, and for previews.
- **GIF output changes slightly.** Text anti-aliasing was blended into the
  palette as it was drawn; now it is blended in RGBA and rounded to the
  nearest colour afterwards. The difference is at glyph edges.
- **A frame and a picture.** The pusher must not resend while a picture is
  up, or it would replace the picture within 25 s. R3.3 and its live check
  cover it.
- **The ring.** The firmware re-applies the lighting channels' modes when it
  switches to the readout (sanshoku spec 024). hotaru only reaches the
  readout by asking (`liquid` commits the ring first) or by stopping; the
  live ring criterion checks both.
- **Depends on sanshoku #102.** Built on its commit as a pseudo-version until
  a sanshoku release carries it.
- **Disk.** A full-colour copy is a PNG of 0.3 to 1 MB per still picture,
  beside a GIF of 50 to 200 KB.
- **Rollback.** Revert the branch: the pusher goes back to GIFs, and the API
  field disappears (clients ignore it). Nothing is stored in a new format.

## Alternatives Considered

- Considered sending frames only (no full colour) first; rejected by the
  maintainer: the colour is the visible gain.
- Considered keeping the GIF for the preview; rejected because a preview in
  256 colours would not show what a panel in full colour shows.
- Considered dithering the GIF; rejected for the reason `palette` gives:
  dithering a starfield grew a 21 KB frame to 105 KB, and the floor with it.

## Verification

(Results pasted here.)
