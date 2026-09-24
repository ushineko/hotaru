# Spec 046: a band that says where it has been

**Issue**: [#124](https://github.com/ushineko/hotaru/issues/124)

## Status: INCOMPLETE

## Context

The stacked arrangement has a hole in it.

Measured on a rendered frame rather than read off the constants: the
headline's ink ends at y=201 and the first table row's ink starts at y=296.
Ninety-four pixels of panel, the full width, drawing nothing. It is the one
arrangement with room to say something more, because it is the one with no
ring.

What belongs there is the headline's own past. The number at the top says 12%
and the four rows say what everything else is doing, and none of them says
whether 12% is where this machine has been sitting or where it has just
arrived. A trace under the number answers that without another figure to read.

### What the push gate actually costs, measured

The first version of this spec was built around the push gate, and the
measurement says the concern was mostly wrong. It is recorded here because the
design that came out of it is better than the one that went in.

The pusher renders every cycle, hashes what the frame *says*, and writes only
when that hash moves. The worry was that a trace advancing every cycle would
change the hash every cycle, turning a panel that writes when something
changes into one that writes every three seconds for as long as the machine is
on.

Ten readings, three seconds apart, on an idle desk:

| Source | What it did |
|---|---|
| `cpu_c` | 52, 59, 51, 69, 82, 60, 72, 64, 51, 62 -- moved every sample |
| `pump_rpm` | 2676, 2681, 2686, 2676, 2688, 2693 -- moved every sample |
| `cpu_pct` | 11, 9, 10, 15, 14, 10, 9, 10, 11, 8 -- moved nine of ten |
| `fan_rpm` | 1357, 1363, 1428, 1357, 1363 -- moved most samples |
| `coolant` | 39.1 throughout |
| `gpu_c` | 39 throughout |

Any dashboard carrying CPU temperature, CPU share, pump or fan already moves
its own hash on nearly every cycle, and the shipped screens all carry at least
one of them. The gate earns its keep on a screen built purely from slow
readings, which is not the common case. **The trail costs nothing there that
the numbers above it are not costing already.**

Two things from that measurement do shape the design.

**A point is a mean, not a sample.** A processor that reads 51, then 82, then
60 within six seconds cannot be drawn from one instantaneous reading per
point: the trace would be a coin toss at full height. Each point is the mean
of whatever readings arrived inside its bucket.

**The gate still holds where it works.** A flat trace shifted one column along
a line of equal points is the same picture, so a screen of slow readings
writes no more often than it did. That is asserted rather than assumed.

### Five minutes across sixty points

The window is a readability choice rather than a cost one. **Five minutes, in
five-second buckets.** Long enough to show a build start and finish, short
enough that somebody who has just done something sees it happen. The dashboard
loop reads every one to three seconds, so each bucket holds two or three
readings to average.

### What it plots, and what it is drawn against

The headline's source, with no new field on the dashboard and no new control
in the editor. The band belongs to the number above it.

The vertical scale comes from the reading rather than from a table. A source
measured in per cent is drawn against 0 to 100, because the eye already knows
that scale and a CPU sitting at 3% should look low. Everything else is drawn
against the window's own lowest and highest value, with a minimum span of a
tenth of the highest, so that a coolant temperature that moved by 0.2 degrees
does not fill the band with what is really a flat line.

A sample the machine could not take is a gap rather than a zero, for the
reason `known` exists in the API: a pump drawn at 0 RPM is the most alarming
number this panel can show, and it would rest on no evidence.

### The history has to live somewhere

The service keeps a ring buffer per source, and **whoever takes a reading
fills it**. The dashboard loop reads every cycle and the API reads whenever
somebody asks, so the history needs no sampler of its own and costs no sensor
traffic. A trail asked for while the panel is held takes a reading first, so
that a screen showing somebody's photograph does not leave a hole in the
history behind it.

Every source rather than the active dashboard's headline: the buffer is sixty
floats per source, somebody can change the active dashboard at any moment, and
a trail that started when the dashboard changed would be a trail that says
nothing for half an hour.

The history is in memory. A service that restarts starts the trail again, and
the band fills from the right as the samples arrive.

## Requirements

**R1. The stacked arrangement draws a trail** of its headline's reading in the
band between the headline and the rows.

**R2. The trail is five minutes** of history in five-second buckets, and each
point is the mean of the readings that arrived in its bucket.

**R3. A flat reading does not move the frame's content**, so the push gate
still holds an idle machine quiet.

**R4. The trail is drawn as a line with the area under it filled**, blended
with what is behind it so that it reads over a photograph.

**R5. Per cent is drawn against 0 to 100**; everything else against the
window's own range, with a minimum span.

**R6. A missing sample is a gap**, not a zero.

**R7. A trail with fewer than two points draws nothing**, and a partial trail
draws what it has, against the right-hand edge.

**R8. The other arrangements are unchanged.**

## Acceptance Criteria

- [x] AC1. A stacked dashboard whose headline has a trail draws ink in the
      band between the headline and the rows, and no ink outside it that was
      not there before.
- [x] AC2. Ring, Grid and Big render byte-identical frames with and without a
      trail.
- [x] AC3. A trail of identical values, advanced by one sample, renders the
      same `Content` hash: the gate still stops the push.
- [x] AC4. A trail whose values change renders a different `Content` hash.
- [x] AC5. A per cent source is drawn against 0 to 100: 50% lands within a
      pixel or two of the band's middle whatever else is in the trail.
- [x] AC6. A source with a tiny range is not drawn as a full-height wiggle,
      and a source with a wide range fills the band.
- [x] AC7. A gap in the trail draws as a gap, with the line resuming after it.
- [x] AC8. Fewer than two points draws nothing at all.
- [x] AC9. The history keeps the mean of each bucket per source, drops the
      oldest past sixty, hands back the trail oldest first, and does not walk
      a million buckets after a machine sleeps for a week.
- [x] AC10. The history fills from the readings the service already takes,
      and the editor's preview is drawn with the same trail the panel has.
- [ ] AC11. Verified on the development machine, on the panel.

## Alternatives Considered

- **A trail per row.** Rejected: the rows are 66 pixels tall and hold a label
  and two numbers already.
- **A source chosen per dashboard.** Rejected for now: it is a field, a
  chooser in a form that has already lost a card off the bottom of a window,
  and a decision to make on every dashboard. The headline's own history needs
  none of that.
- **A faster window, chosen per dashboard.** Rejected: one setting would
  quietly make the panel write ten times as often, and the editor would have
  to explain that.
- **Bars rather than a filled line.** Rejected: sixty bars in 400 pixels is
  six pixels each, and the shape of the trend is what is being read.

## Risks & Assumptions

- **Every saved stacked dashboard changes appearance.** There is no field to
  turn this off, which is what "no new configuration" costs. If somebody wants
  the band empty, that is a field and a spec of its own.

- **The trail is empty for the first bucket after a restart**, and partial for
  the first five minutes. It fills from the right. A window opened in that
  time shows a short trace rather than a fault.

- **Five minutes is a judgement**, like the unit's size in spec 044 and the
  gap in spec 045. The numbers that constrain it are measured; where to sit
  between "live" and "trend" is not, and it wants looking at on the panel.

- **A screen of slow readings now writes more often than it did.** A dashboard
  of coolant and GPU temperature alone held its frame for minutes, and a
  moving trace under the headline will move it every five seconds. That is the
  one case where this costs a push that was being saved.

- **Rollback** is a revert. Nothing is written to disk: the history is in
  memory and no file format changes.
