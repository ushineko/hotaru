# Spec 080: dashboards in full colour, as frames

**Issue**: [#226](https://github.com/ushineko/hotaru/issues/226)

## Status: COMPLETE

## Executive Summary

The dashboard is painted in full colour and, on the Kraken Elite, sent as
streamed frames: checked every second, sent on a change, sent again every
25 s before the firmware's 30 s return. The image library keeps a full-colour
copy of still pictures for dashboard backgrounds. The live run also found two
sanshoku faults (blue-first byte order, a method name that hid the ring's
canvas), fixed in ushineko/sanshoku#102. Reviewers: `Pusher.frame` in
`internal/dashboard/push.go`, then `Render`/`quantise` in `render.go`.

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

- [x] `make test`, `make lint` and the GUI tests
      (`go test -tags migrated_fynedo ./internal/gui/...`) pass.
- [x] Tests:
  - [x] a dashboard rendered twice with the same reading has the same
        `Content`; a changed reading changes it; the tick does not;
  - [x] a gradient (the starfield) renders with more than 256 distinct
        colours in the RGBA frame, and the GIF of the same frame has at most
        256 and decodes at 640x640;
  - [x] a quantising benchmark, run and recorded in Verification (R2.2);
  - [x] the pusher on a framing fake: sends on a change, does not send an
        unchanged frame within `FrameHold`, resends it after `FrameHold`
        (injected clock), sends nothing while held, sends at once on
        release, and keeps the tick on a resend;
  - [x] the pusher on a GIF-only fake behaves as before (existing tests);
  - [x] the preview route returns `format: "png"` and a decodable PNG with a
        framing cooler, and `"gif"` without one.
- [x] Live, the maintainer watching:
  - [x] the dashboard on the panel in full colour (starfield and a picture
        background), compared with 2.0.0's GIF;
  - [x] a reading that changes shows within about a second;
  - [x] idle for at least 2 minutes: no return to the readout, no blink at
        the resends;
  - [x] `hotaru screen show` a GIF, then back to the dashboard: the picture
        stays (no resend over it), and the dashboard returns at once;
  - [x] the readout chosen, then the dashboard: the same;
  - [x] hotaru stopped: the readout returns, and the ring is unchanged;
  - [x] with the ring on an animated effect, the dashboard running for
        2 minutes: the ring does not change at the frames, the resends or
        the stop (moved here from sanshoku spec 037);
  - [x] the window's preview of a dashboard matches the panel.
- [x] Tests for R7: a still is kept and read in full colour; an animation has
      no copy, and replacing a still with one removes the old copy; the copy
      follows a rename and a remove; a picture without a copy reads its GIF.
- [x] Live: a picture added again from its original shows in full colour
      behind a dashboard.
- [x] Cost recorded per R5.1.
- [x] Docs per R6.

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
- **A refused first frame.** On the first dev build the first frame after a
  start was answered `0x10` at `36 02`; the next frame went through. It did
  not recur on later starts (three restarts). It is reported and retried
  like any failed frame.
- **Depends on sanshoku 0.3.0**, which carries #102 (`screen.Streamer`).
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

Desk, 2026-11-18: Kraken Elite V2 on firmware 1.2.12, the dev build run
under the user service through a drop-in, the maintainer watching.

**Tests.** `make test`, `make lint` (0 issues), `make lint-windows`
(0 issues), `go test -tags migrated_fynedo ./internal/gui/...`, and
`go test -race ./internal/dashboard` three times all pass. `govulncheck`
finds nothing.

**Benchmarks** (`go test ./internal/dashboard -run - -bench 'Quantise|Render'`):

```
BenchmarkQuantiseTheShippedDashboard-30   2729346 ns/op   500510 B/op   3698 allocs/op
BenchmarkRenderTheShippedDashboard-30     3262353 ns/op  1648189 B/op    982 allocs/op
```

Quantising is 2.7 ms against R2.2's 30 ms. The shipped dashboard's GIF moved
at 522 of 409600 pixels, at the edges of text; its full-colour frame has 3679
colours.

**Live run.**

1. First dev build: `ice1` (light text on dark blue) showed yellow text on
   brown, and the cooler's ring was not attached. A diagnostic on the desk:
   `*nzxt.framePanel is not lighting.Canvas: missing method Frame`. Both were
   sanshoku's, fixed in 71e0f45 (blue first; `screen.Streamer`).
2. Fixed build: colours right, the ring attached ("drawing on NZXT Kraken
   Elite V2").
3. R7: a photograph added in the dev window as `tree-full` stored
   `tree-full.png` (485 KB) beside its GIF. On the panel it looked better than
   the GIF version; the window's preview matched the panel.
4. Numbers updated within about a second; no blink, no readout.
5. `hotaru image show cat1`, 40 s; `hotaru screen readout`, 40 s;
   `hotaru screen dashboard`. Each held; the dashboard returned at once. The
   frame counter: 2 frames in those 82 s, one before and one after.
6. The ring on a moving effect through steps 4 and 5, then a stop for 10 s
   and a start: the readout while stopped, the ring unchanged, everything back
   after the start.
7. A dashboard showing only the pump duty on the starfield, 09:12:35 to
   09:15:05: no blink, no readout. The counter, 09:13:00 to 09:14:16: 0
   changed frames, 3 resends. The starfield looked smoother than on 2.0.0.

**Cost (R5.1).** The service's `CPUUsageNSec` over 10 minutes, all lighting
included, a picture dashboard on the panel each time:

| build | CPU in 600 s | share of one core | frames |
|---|---|---|---|
| 2.0.0, GIFs (`ice1`) | 28.9 s | 4.8% | not counted |
| dev, frames (`tree-full`) | 33.9 s | 5.7% | 527 changed, 0 resends |

A picture dashboard with CPU and GPU readings changes nearly every second, so
frames ran at about 0.9 a second, about 1 MB/s over USB. Frames were counted by
a line in the dev build that logged once a minute and was not committed.
