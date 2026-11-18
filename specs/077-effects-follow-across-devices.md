# Spec 077: effects follow across devices, and the canvas gaps

**Issue**: [#218](https://github.com/ushineko/hotaru/issues/218)

## Status: INCOMPLETE

## Executive Summary

Written last.

## Context

Since spec 075 every device is a sanshoku canvas, and since spec 076 (#215)
nothing else lights them. Moving the tests onto canvases found five gaps on
that path, and the maintainer set a requirement on 2026-11-17 that none of
them ship:

1. A zone's shape is guessed: every zone of more than one light is a line, so
   `light map` asks the K4 HE how many things are chained on it.
2. A `reassert` rule never reaches a canvas: `ReassertRules` leaves canvases
   out, and the animator does not resend an unchanged frame.
3. hotaru ignores sanshoku's catalogue (spec 025): every firmware effect is
   listed with one colour and no speed. A scene's speed ("Solid Reactive
   Multinexus at 55") is dropped without a word; the Kraken's Tai Chi gets one
   colour of two and its Fading one of three; the RTX 4090's Breathing one of
   two; Cycle All is listed as taking a colour. The Kraken's 14 effects, as
   the CAM captures characterised them, are reachable only by name.
4. `light map` asks about dimming after every device, since every canvas mode
   dims.
5. The write queue's latest-wins was covered only through a slow OpenRGB
   fake.

**The requirement: every supported effect can follow on every device that
can show one.** On this desk that is the Kraken (ring and fans), the ASUS
Aura's headers (side fans, rear fan, mirror accent), the RTX 4090, the Corsair
MM700, the G502 X PLUS, the K4 HE, and the Apex and the F75 where they are
attached. An effect the leader runs shows on each follower, as close as each
can. Which devices follow, and which are set on their own, is hotaru's
configuration, device by device.

Decided on 2026-11-17:

- **The Aura draws hotaru's renderers** of the Kraken's effects, matched by
  eye on the bench. The Aura's own firmware effects have nothing like Tai Chi,
  Water Cooler, Loading, Candle or Starry Night. Every other device hotaru
  draws (the MM700, the G502, the K4 HE, the Apex, the F75) draws the same
  renderers.
- **The Kraken runs its own firmware effect.** It costs nothing while it runs.
  The renderers start in step with it, and restart in step on every apply.
- **The RTX 4090 takes its closest firmware effect**: one light at four
  frames a second at most is too coarse to draw on.

What sanshoku has and lacks (sanshoku #89):

- `lighting.Catalogue` lists each effect's number, speeds (slowest first, in
  the device's units) and colour count. The Kraken's: Fading 3 colours, Tai
  Chi and Water Cooler 2, Covering Marquee and Alternating 2, the rainbows
  none, the rest 1; speeds as a period, smaller faster (sanshoku
  `docs/kraken-lighting.md`).
- `lighting.Onboard` sets an effect and **one** colour. Nothing sets a speed
  or a second colour.
- `lighting.Zone` has no shape.

## Requirements

### R1. sanshoku (sanshoku #89, its own spec)

- R1.1 A firmware effect can be set with its colours, up to the catalogue's
  count, and a speed from the catalogue's range, in one call; the Kraken, the
  MSI card and the K4 HE implement it. The K4 HE's catalogue gains its speed
  (`07 03 03`, one value for every effect).
- R1.2 A zone says its shape where the driver knows it: a ring (the Kraken's
  fans and ring), a grid (a keyboard), a single light (the GPU, the MM700's
  three), a line. An addressable header's is unknown: the board cannot tell a
  fan from a strip.

### R2. The catalogue in hotaru (gap 3)

- R2.1 A device's firmware effects are listed from its catalogue: how many
  colours each takes (none, one, or a range) and its speeds. The API, the CLI
  and the window show them; a colourless effect takes no colour.
- R2.2 An effect applied with several colours and a speed, from `light set`,
  a scene or the window, reaches the device with all of them, through R1.1. A
  speed outside the catalogue's range is the nearest it has, and the result
  says so.
- R2.3 The colours and speed are recorded, and a restore puts them back.
- R2.4 A record or scene made through OpenRGB keeps its speed where the units
  agree (the K4 HE's 0 to 255); otherwise the nearest the catalogue lists.

### R3. Effects follow (the requirement)

- R3.1 **One set of names.** Each of the Kraken's 14 effects is offered by
  every device that can show an effect, under the Kraken's name: on the
  Kraken, its firmware; on every drawn device (the Aura, the MM700, the G502,
  the K4 HE, the Apex, the F75), a hotaru renderer; on the RTX 4090, its best
  fit (R3.6). A device lists which it draws and which it maps. A device's own
  firmware effects stay offered beside them.
- R3.2 **Following is configured, device by device.** A rule's
  `follow: <device>` makes a device take the leader's effect, colours and
  speed whenever the leader is given one: an apply, a scene, a restore. A
  device with no `follow` is set on its own, as today. A scene that names an
  effect for a follower overrides it for that device, for that scene. Any
  device may lead; a follower of a follower takes the first leader's, and a
  loop of follows takes nothing, since none of it is given an effect first. A
  device with one light takes its own best fit (R3.6) over a drawn copy.
  An effect a scene gives with no colours of its own takes the scene's
  colours on the device, each once, in the order its lights show them, as
  many as the effect takes; a follower takes the same colours. The rule is
  the default and the scene the override, as the maintainer put it on
  2026-11-17: an existing scene, given an effect, runs it in its colours. The window and `light map` set
  and clear `follow`, as they set every other rule. The maintainer's rules
  start with the Aura and the RTX 4090 following the Kraken; the rest are the
  maintainer's to choose.
- R3.3 **Renderers.** Each of the 14 effects is a hotaru renderer taking the
  Kraken's colours and speed in the Kraken's units, the period mapped to time
  as E1 measures it. Each is matched to the Kraken by eye on the bench (E1):
  the colours' order, the direction, the pace, how the ring and the fans
  differ.
- R3.4 **Drawn per thing.** A renderer draws each fan of a zone as the Kraken
  draws each of its fans, and a strip along its length. A zone's things are
  the rules' segments that split it (as `light map` writes them); its shape is
  the driver's (R1.2), else a rule's `shapes` (`ring` or `line`), which
  `light map` asks for a header of unknown shape.
- R3.5 **Starting together.** Applying an effect sends the Kraken its effect,
  and the renderers start their phase when the Kraken acknowledges it. Every
  apply and restore restarts them together. The drift over 15 minutes is
  measured (E2); if it is visible, the spec states how the renderers re-sync
  without writing the Kraken more often.
- R3.6 **The RTX 4090's best fit**: a table from each Kraken effect to one of
  the card's, with the colours and speed it can take, tuned by eye (E4). A
  starting point: Breathing, Pulse, Candle and Starry Night to Breathing;
  Fading to Color Cycle; Spectrum Wave and the three rainbows to Rainbow Wave;
  Alternating, Tai Chi and Water Cooler to Fade In with both colours; Covering
  Marquee and Loading to Flowing. The speed is the nearest of the card's
  three.
- R3.7 **Cost.** A renderer draws at the pace E1 finds smooth for the effect,
  never under the device's floor, and stops when a still scene replaces it.
  The CPU, USB and radio cost of every device following the Kraken is measured
  and stated, against the same desk with a still scene.
- R3.9 **A pace trim per device, set by matching** (the maintainer, 2026-11-17,
  during E1's tuning: a pace matched by eye depends on the device's LEDs and
  diffusers and on who looks, and a description of one is hard to give). A
  rule's `pace` is a multiplier on a device's copy of each named effect
  (`pace: {Breathing: 2.2}`); E1's measured paces are the defaults. It applies
  wherever the copy runs: following, or set on the device. `hotaru light
  match <device> <effect>` previews the effect on the device and its leader
  together; `f` and `s` make it faster or slower, `r` restarts both in step,
  and Enter writes the trim to the rules. The window has the same: a speed
  slider beside a live preview, and Save. One API route serves both.
- R3.8 **Each device's limits, stated and kept.** Where a device cannot show
  part of an effect, it shows the nearest and the listing says so:
  - the K4 HE takes each key's hue, and one brightness for the board, so an
    effect that dims keys apart (Candle, Starry Night) dims the board;
  - the MM700's logo has no blue channel (spec 009);
  - the F75 through its receiver shows a merged palette, and a moving effect
    is radio traffic for as long as it runs;
  - the G502 is wireless: a moving effect costs battery, and E5 measures how
    much an hour of one costs against a still colour;
  - a single light (the MM700's, the RTX 4090's) shows an effect's colour over
    time, not across lights.

### R4. Shapes and the map (gaps 1, 4)

- R4.1 A zone's shape is the driver's (R1.2), or the rules' (R3.4). `light map`
  asks how many things are chained only on a line or a ring.
- R4.2 `light map` asks about dimming once a run, and per device only for the
  devices a person names.
- R4.3 `light map` finds a header's length, which the board cannot report:
  it lights the header one LED further at a time (halving the range, as the
  maintainer's bench did) and asks where the light stops, then writes the
  `lengths` rule. A header with nothing on it is written as 0.

### R5. Re-assert on a canvas (gap 2)

- R5.1 A `reassert` rule on a drawn device resends its frame at the interval,
  where the frame is still; a firmware effect is not re-written (spec 071
  R6.3). Whether the G502 needs one is measured (E3), and the example rules
  say what was found.

### R6. The queue (gap 5)

- R6.1 `canvastest` gains a device whose writes take a set time, and the
  latest-wins and superseded-is-not-failure tests run on it.

### R7. Documentation

- R7.1 `docs/hardware.md`: what follows and how on each fan device, the 4090's
  table, `follow` and `shapes`. `docs/api.md`: the catalogue's fields. README:
  the changelog.

## Experiments, on the desk (someone watching)

- **E1. The Kraken's effects.** For each of the 14, at CAM's speed and the
  slowest and fastest listed, with distinct colours: the period, the
  direction, the colours' order, and how the ring and the fans differ. Then
  the renderer on the rear fan beside the Kraken, tuned until the two look
  alike.
- **E2. Drift.** Tai Chi and Spectrum Wave on the Kraken and the Aura,
  started together: in step after 1, 5 and 15 minutes?
- **E3. The G502 asleep.** A still frame, the mouse left to sleep, then woken:
  the frame, or its own lighting?
- **E4. The RTX 4090.** Each mapping in R3.6 beside the Kraken: the closest of
  the card's effects, and its speed.
- **E5. The other drawn devices.** Tai Chi, Spectrum Wave, Breathing and
  Candle following the Kraken on the MM700, the G502, the K4 HE (and the Apex
  and F75 where attached): does each read as the same effect? The G502's
  battery over an hour of a moving effect against a still colour.

## Acceptance Criteria

- [ ] sanshoku #89's spec is merged, and hotaru builds on it.
- [ ] `make test`, `make lint` and the GUI tests pass.
- [ ] Tests:
  - the catalogue is listed: colour counts, speeds, colourless effects;
  - an effect's colours and speed reach a `canvastest.Firmware`, are recorded,
    and are restored;
  - a follower takes the leader's effect, colours and speed, and a scene's own
    effect for it wins; a device with no `follow` is untouched; a chain
    resolves to the first leader; a loop takes nothing;
  - each renderer draws per thing on a ring and a line, at its speed;
  - renderers restart in phase on apply;
  - the 4090 table maps each of the 14;
  - shapes come from the driver or the rule;
  - `light map` asks chaining only on a line or ring, and dimming once;
  - `light map` finds a header's length from a person's answers and writes it;
  - a `pace` trim lengthens or shortens a copy's period, following or not;
    `light match` writes the trim it was left at;
  - a re-assert resends a still canvas frame;
  - latest-wins holds on a slow canvas.
- [ ] Live, with someone watching:
  - each of the 14 effects follows on the Kraken, the Aura's three headers
    and the RTX 4090;
  - E5's four effects follow on the MM700, the G502 and the K4 HE (and the
    Apex and F75 where attached);
  - a device with no `follow` keeps its own effect while the Kraken's changes,
    and one given `follow` in the window starts following;
  - Tai Chi in two colours and Fading in three, on all of them;
  - a speed change on all of them;
  - E2's drift recorded;
  - a scene with "Solid Reactive Multinexus at 55" keeps its speed through a
    restart;
  - `light map`'s live checks moved to spec 078 (#220): run on the desk on
    2026-11-17, the wizard proved too hard to use, and 078 replaces it. Its
    chaining and header-length behaviour here is held by tests;
  - `light match` and the window's slider match the rear fan's Breathing to
    the Kraken's, and the trim survives a restart.
- [ ] The cost of R3.7 measured and recorded.
- [ ] Docs per R7.

## Risks & Assumptions

- **Matching by eye.** The cooler reports no frame, so a renderer is matched
  to what a person sees. Close, not identical, and stated as such.
- **Drift.** The Kraken's clock is not hotaru's. E2 says whether it matters.
- **Cost.** Following streams frames to the Aura for as long as an effect
  moves, which a still scene does not. Measured (R3.7), and the cost rule
  holds: the effect's pace, not the device's floor.
- **`follow` is a rule key**, set device by device (the maintainer, on
  2026-11-17): following is configuration, and a device can as well be set on
  its own. A scene could name the effect for each device instead; the rule
  keeps every scene short.
- **Battery and radio.** Following puts moving effects on wireless devices
  that showed still colours. E5 measures it; the default for a new device is
  not to follow.
- **sanshoku's API grows.** A new optional interface beside `Onboard`, so
  existing consumers are untouched.
- **Also before the release:** sanshoku #90 widens each lighting driver's
  allow-list to every product sharing its protocol, as expected.
- **Rollback:** revert. A rules file with `follow` or `shapes` loads on an
  older hotaru with a problem reported for each unknown key.

## Verification

### E1: the Kraken's effects (2026-11-17, the maintainer watching)

`sanshoku-bench onboard --yes` on the Kraken, hotaru stopped, each effect in
chosen colours at CAM's speed, and the slowest and fastest where it has more
than one. Every speed was linear: the period grows with the number.

- **Fading** (01), RGB, speed 40: ~6 s per full cycle of 3 colours; smooth crossfade throughout; ring and fans in step, every LED the same colour.
  - speed 80: ~12 s per cycle, smooth.
  - speed 10: ~1.5 s per cycle, smooth. Linear: ~0.05 s per speed unit per colour.
- **Spectrum Wave** (02), no colours, speed 250: full spectrum around each ring at once, rotating clockwise (facing the pump), ~5 s per revolution; each fan its own full rainbow, clockwise, in step with the ring.
  - speed 350: ~7 s per revolution. Linear: ~0.02 s per speed unit per revolution.
  - speed 80: ~1.5 s per revolution.
- **Covering Marquee** (04), red/blue, fixed speed 150: one colour sweeps clockwise and covers the other, then the other sweeps over it, alternating; ~2-3 s to cover the ring. The fan channel is one chain: the sweep runs fan 1, then 2, then 3, taking as long as the ring takes once.
- **Alternating** (05), red/blue, speed 500: blocks of ~4 LEDs alternating red/blue (6 blocks on the 24-LED ring) stepping clockwise by ~2 LEDs (half a block), 2-3 steps a second; fans the same, in step with the ring.
  - speed 1600: ~1 step a second, clockwise. ~0.7 ms per speed unit per step. (500 is the fastest listed.)
- **Pulse** (06), red, speed 15: brightens fast, fades out smoothly, dark very briefly, ~1 pulse a second; ring and fans together.
  - speed 25: ~1.5-2 s per pulse, same shape. ~0.07 s per speed unit per pulse.
- **Breathing** (07), red, speed 20: ~2 s per breath, smooth both ways, fully dark at the bottom. ~0.1 s per speed unit (40 slow and 4 fast seen in sanshoku spec 032).
- **Candle** (08), amber, fixed speed 50: each LED flickers separately, slight dips, fast flutter; fans the same.
- **Starry Night** (09), blue, speed 15: overall blue; LEDs twinkle at random between dim and brighter blue (no dark background); about Candle's pace, a little slower.
- **Rainbow Flow** (0B), no colours, speed 250: part of the spectrum spread round the ring, moving clockwise ~3-4 s per revolution, the hue band shifting so later turns show more of the spectrum; the fan channel shows one rainbow stretched along the chain of three fans, in step with the ring.
- **Super Rainbow** (0C), no colours, speed 150: the whole spectrum as a smooth gradient round the ring, clockwise ~2 s per revolution; fans one gradient stretched along the chain; smoother than Rainbow Flow.
- **Rainbow Pulse** (0D), no colours, speed 250: two blobs of colour circling the ring clockwise ~3 s per revolution, grading to black between them; their colours cycle through the rainbow. Each fan has one blob (its own colour, differing fan to fan), sweeping clockwise at the ring's pace.
- **Tai Chi** (0E), red/blue, fixed speed 30: on the ring, two halves chasing clockwise ~3 s per revolution, blended edges. On each fan (offset fan to fan), one colour advances over the other until the fan is full, then the other takes it back.
- **Water Cooler** (0F), red/blue, fixed speed 30: a still Tai Chi base (red half, blue half, not chasing) with a brighter section travelling round it clockwise, ~6 s per revolution, a dim-to-bright gradient. Fans do the same with half the effect each, staggered fan to fan.
- **Loading** (10), red, fixed speed 10: like Water Cooler, in the one colour on a base that fades to black: a red section runs clockwise from the top, ~6 s per revolution; when its leading edge reaches the top it waits for the tail to catch up until the ring is dark, then starts again. Fans the same, staggered fan to fan.

### E1: the copies tuned beside the Kraken (2026-11-17)

Each effect on the Kraken (its firmware) and the Aura (hotaru's copy, the
rear fan given `shapes: {Addressable RGB Header 3: ring}`), together, the
maintainer comparing:

- Spectrum Wave: full rainbow, clockwise, about the same pace. As written.
- Fading: slow red and blue "very close"; the copy a little fast, so its step
  went from 50 to 52 ms a unit.
- Covering Marquee, Starry Night, Super Rainbow, Rainbow Pulse, Tai Chi, Water
  Cooler: "close enough". As written.
- Alternating: "choppier"; the copy now moves smoothly, each light blended by
  how much of it each block covers: "much better".
- Pulse: the copy's fade was too quick; it now fades over 87% of the pulse
  with a 3% dark moment, and runs at 74 ms a unit, not 70: "close enough".
- Breathing: the copy matched only near 4.5 s a breath where E1's count was
  2 s. Pace by eye depends on the device and the person, so R3.9 (a pace
  trim, set by matching) was added rather than tuning each pace here.
- Candle: dips deepened from 30% to 50%: "closer".
- Rainbow Flow: "chunky" from the seam where its band's ends met; the band
  now runs out and back: "smoother".
- Loading: "choppy" on the side fans' 16-LED strip; the head and tail now
  glide between lights: "smoother, close enough".

Not yet run: E2 to E5, and the live criteria.

### E2 and E3: drift and the mouse's sleep (2026-11-17, the maintainer watching)

`follow: kraken` on the Maximus and GeForce rules; a scene giving only the
Kraken Tai Chi in red and blue, applied at 13:41:47. All three devices lit:
the Kraken on its own Tai Chi, the Aura on hotaru's copy, the RTX 4090 on its
best fit, Fade In, fading between red and blue.

- **E2:** the Aura and the Kraken in step at 1, 5 and 15 minutes: no drift.
- **E3:** the G502 left untouched past its sleep, then moved: it showed the
  scene's colours.

### E4: the RTX 4090's best fits (2026-11-17, the maintainer watching)

Each of the Kraken's effects given to the Kraken alone, the 4090 following.
Three faults found and fixed on the way, each with a test:

- Pulse: the card's Breathing takes two colours and filled the second with
  its own green. One colour now fills every slot of a firmware effect.
- Breathing: the card's own Breathing was given the Kraken's speed as it
  stood, out of the card's range, and held Static. The speed now goes across
  as a share of each range.
- Rainbow Flow: the scene's two colours made the Kraken's driver refuse an
  effect that takes none, and the cooler held red. A firmware effect is now
  given only the colours it takes.

| Kraken | 4090 | Seen |
|---|---|---|
| Fading | Color Cycle | yes, close enough |
| Spectrum Wave | Rainbow Wave | yes, close enough |
| Covering Marquee | Flowing | yes, close enough |
| Alternating | Fade In | yes, close enough |
| Pulse | Breathing | was red and green (card filled slot 2); fixed, red only, fair |
| Breathing | Breathing (own) | was Static (Kraken speed out of card range); fixed, red, fair |
| Candle | Breathing | close enough |
| Starry Night | Breathing | close enough |
| Rainbow Flow | Rainbow Wave | Kraken held red (colours on a colourless effect); fixed, fair |
| Super Rainbow | Rainbow Wave | close enough |
| Rainbow Pulse | Rainbow Wave | close enough |
| Tai Chi | Fade In | fair (E2 run, red/blue) |
| Water Cooler | Fade In | close enough |
