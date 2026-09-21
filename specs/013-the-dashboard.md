# Spec 013: the dashboard

**Issue**: [#3](https://github.com/ushineko/hotaru/issues/3)

## Status: COMPLETE

## Context

peripheral-battery-monitor draws a dashboard onto the cooler's screen: coolant
temperature as a headline with a severity-coloured ring, CPU, GPU and pump
across the bottom, over a starfield. It is a good design, arrived at by looking
at the panel in a case, and several of its constants are corrections to
mistakes -- the metric row's 140px inset exists because the ring cut through
the word "RPM" at the old margin.

This spec is that design, ported, plus the one thing the Python could not
measure: what the panel will actually accept.

Spec 012 established that hotaru reaches the cooler directly. Rendering follows
from it: Qt drew the Python's frames and Pillow encoded them, and neither
belongs in a service. Go's standard library has `image/gif` but no font
rasteriser, so the port uses `golang.org/x/image` with the Go fonts embedded --
one dependency, pure Go, no cgo, no system fonts.

### What the panel will take

Measured by pushing a frame repeatedly and watching an indicator that advances
on every update:

| frame | 1 s | 1.5 s | 2 s | 3 s |
|---|---|---|---|---|
| 5 KB, seven-segment digits | **lands** | | | |
| 8 KB, flat background | never updates | skips | **lands** | |
| 21 KB, full starfield design | | | **lands** | **lands** |

**This is a settling time that scales with frame size, not a size limit.** A
frame that never appears at one second appears reliably at two. The device
accepts every transfer either way: the HID exchange succeeds, the bulk write
completes, the bucket switch returns success, and the screen does not change.

So the constraint is a floor on the interval, decided by how large the encoded
frame is -- which makes it a property of the *design*, not of the code. Somebody
who adds a gradient later raises the floor without touching a line of the
pushing logic.

Two things follow that were not obvious:

- **Frame size is decided by the palette, not by the drawing.** GIF is LZW over
  palette indices, so a 249-step gradient gives nearly every pixel its own
  value and compresses to nothing, while the same picture through 24 steps is a
  third of the size. Dithering is worse again: Floyd-Steinberg across a
  starfield turned a 21 KB frame into 105 KB.
- **Scattered single pixels are expensive.** The Python's 420 one-pixel stars
  each break a run that would otherwise compress away. Drawn as 2x2 blocks at
  150 of them, the sky looks the same at arm's length and the frame is smaller.

### Something must change every frame

A screen that has stopped being written is indistinguishable from a machine
whose sensors are steady. Coolant sits at 37.5 C, the pump at 2600 rpm and the
fan at 1190 rpm for hours; only the CPU moves, and only a little.

Most of the time spent establishing the table above was spent not knowing
whether a frame had landed. The thing that made it legible was a row of twelve
marks with one lit, advancing on every update: a frozen screen stops the mark
moving, and the difference is visible from across the room.

This is not decoration. **The dashboard carries an element that changes on every
render**, and it is the first thing to look at when somebody reports that the
screen is stuck.

### What it costs

Running the ported design at a 2 s cadence, measured over 40 s:

	cpu: 218ms over 40.016s wall (0.55% of one core)
	     10.914ms of CPU per update
	peak rss: 17248 KB   go heap in use: 4704 KB
	render: avg 7.8ms    push: avg 49.9ms    frame: avg 21 KB

The push is almost entirely blocked on USB rather than working. The Python
spends a Qt paint pass, a Pillow encode and a 105 ms `liquidctl` process on
every frame; the process spawn alone is ten times the whole Go update.

Caching matters more than it looks: re-quantising the static background every
frame cost 128 ms per render, against 7.8 ms once it is quantised once and
copied.

## Requirements

**R1. The Python's design is the design.** Layout, colours, thresholds and
copy are ported constant for constant, including the corrections its comments
record. Changes are a separate decision, made by looking at the panel, and not
made incidentally during a port.

**R2. A push floor, derived from the frame.** The service does not push more
often than the measured floor for the frame size it is producing. It is stated
as a measured table, not a magic number, and it is checked against the encoded
length rather than assumed.

**R3. Nothing is pushed when nothing would look different.** The gate is a
hash of the rendered frame, which answers "would the screen look different"
exactly, where the Python's per-field comparison approximates it. An idle
machine pushes nothing at all.

**R4. Something changes on every render.** Without it a stuck screen is
invisible; with it, it is obvious. The gate in R3 must not suppress it -- the
changing element is part of the frame the hash covers, so the hash alone would
make every frame differ. The element therefore advances only when the *rest* of
the frame changes, so it marks real updates rather than ticking forever.

**R5. Rendering never fails.** A missing metric draws a placeholder. The screen
is decorative and a render fault must not disturb telemetry, reconciliation or
lighting.

**R6. The background is rendered once.** It is static by design, and
re-quantising it per frame was seventeen times the cost of copying it.

**R7. The screen is given back on shutdown.** A machine that stops running
hotaru shows the firmware readout, not a frozen dashboard from whenever the
service died.

## Acceptance Criteria

- [x] AC1. The rendered panel matches the Python's layout: ring, headline,
      unit, and three metric columns at the same coordinates.
- [x] AC2. Coolant colour bands change at 50 C and 60 C, matching the alert
      thresholds so the screen and the notifications never disagree.
- [x] AC3. CPU is never colour-graded; a pump reading zero is always critical,
      whatever else the screen is doing.
- [x] AC4. A missing metric renders a placeholder and the frame still encodes.
- [x] AC5. The frame hash gates pushes, and a run with unchanging inputs
      pushes once and then stops.
- [x] AC6. An element advances with each accepted update, and a test asserts
      two successive frames with identical inputs are byte-identical -- so the
      indicator cannot defeat the gate.
- [x] AC7. The push floor is enforced against the encoded frame size, with the
      measured table in the code as a comment and in the spec as the source.
- [x] AC8. The background is rendered and quantised once per process.
- [x] AC9. Rendering is allocation-light enough to run forever: a benchmark
      asserts the per-frame allocation does not grow across frames.
- [x] AC10. The screen returns to the firmware readout when the service stops.
- [x] AC11. Verified on the development machine: the dashboard updating at the
      floor with the indicator advancing, and an idle machine not pushing.

## Verified on hardware

Development machine, NZXT Kraken Elite V2, with somebody watching the panel.

The dashboard draws, the numbers agree with `hotaru cooling` and `nvidia-smi`,
the indicator advances, and the service costs 0.6% of one core -- the 0.55% the
table above predicted. `hotaru screen readout` takes the panel and the
dashboard stops rather than drawing over it two seconds later; `hotaru screen
dashboard` gives it back and it redraws at once.

### The panel blanked at random, and it was hotaru deleting the picture

The first run on hardware blanked every so often, with no pattern anybody could
see and nothing reported wrong -- every transfer succeeded.

**The slot being displayed is not free, whatever the device says about it.**
Each push took the next free slot of sixteen and nothing ever released the old
ones. Once all sixteen were occupied, placement wrapped around and cleared the
slot that was on screen; the transfer that followed takes about a second, and
the panel has nothing to show for the duration. Whether a given update blanked
depended on which slots happened to refuse to clear, which is why it looked
random rather than periodic.

The fix is double buffering, which on this panel is not an optimisation: write
into a slot that is not displayed, switch to it, and only then delete the
previous one. hotaru tracks what it last asked the device to show, because the
device does not report it.

This is the same shape as every other finding in specs 009 through 012 -- a
write that succeeds at the protocol level and is wrong at the panel -- and it
was found the same way, by somebody looking at the machine.

## Risks & Assumptions

- **One panel, one firmware.** The floor table is from an NZXT Kraken Elite V2
  on firmware 1.2.0. Another cooler will have its own, and hotaru should
  measure rather than assume -- pushing too fast is invisible, which is the
  worst failure mode available.
- **The floor is a discovery, not a specification.** Nothing in the protocol
  announces it. If a future firmware changes it, the symptom will be a screen
  that quietly stops updating while every write reports success.
- **A graphics card temperature may need a process.** The kernel exposes one
  for AMD and nouveau; NVIDIA's own driver registers no hwmon, so hotaru asks
  `nvidia-smi` -- 18 ms wall and about 2 ms of CPU, once per update. The
  objection this project has to spawning processes was `liquidctl` blocking
  every frame on the write path for 105 ms; a sensor read at 0.5 Hz is not
  that, and the alternative is cgo for one integer. A machine with neither
  draws a placeholder.
- **`golang.org/x/image` is a dependency the service takes on** for a font
  rasteriser. It is a Go project module, pure Go, and the alternative is
  hand-drawn glyphs -- which were tried, work, and suit the panel, but are not
  the Python's design.
- **Rollback** is to stop pushing a dashboard; the screen falls back to the
  firmware readout, which is the state R7 already requires on shutdown.

## Alternatives Considered

Considered keeping the seven-segment glyphs drawn from rectangles, which need
no font dependency at all and produce 5 KB frames that push at 1 Hz. Rejected
as the default because it is not the design the Python arrived at by looking at
the panel -- but it is a working fallback if the dependency ever becomes a
problem, and the prototype keeps it.

Considered raising the refresh rate by shrinking the frame. Rejected: the frame
size is the design, and a dashboard that updates every two seconds is not
improved by looking worse.
