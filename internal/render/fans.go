package render

import (
	"math"
	"time"

	"github.com/ushineko/hotaru/internal/colour"
	"github.com/ushineko/sanshoku/lighting"
)

/*
The NZXT Kraken's own effects, drawn by hotaru so that a device without them
shows them too, and a device can follow the Kraken's (spec 077 R3.3).

Each is matched to what a person saw on the Kraken (spec 077 E1, recorded in
the spec's Verification): the colours' order, the direction, the pace, and how
the fans differ from the ring. The cooler reports no frame, so close is what
matched by eye means.

Three layouts, as the Kraken draws them:

  - each thing shows the whole effect, in step: Spectrum Wave, Alternating,
    Pulse, Candle, Starry Night;
  - one effect runs along a zone's things as a chain: Covering Marquee,
    Rainbow Flow, Super Rainbow;
  - each thing shows a part of it, staggered thing to thing: Rainbow Pulse,
    Tai Chi, Water Cooler, Loading.

A thing is one fan, ring or strip; a ring's lights are taken clockwise from
its top. Fading shows one colour on every light.
*/

// Thing is one physical thing a zone holds: a fan, a pump's ring, a strip.
type Thing struct {
	// Lights are the thing's lights by their place in the canvas's keys, in
	// order: clockwise from the top on a ring, end to end on a strip.
	Lights []int
	// Zone is which of the device's zones the thing is in, so the things of
	// one zone can be drawn as a chain.
	Zone int
	// Ring is a thing whose ends meet.
	Ring bool
}

// The Kraken's effects' names, as NZXT CAM and the cooler's firmware call them.
const (
	Fading          = "Fading"
	SpectrumWave    = "Spectrum Wave"
	CoveringMarquee = "Covering Marquee"
	Alternating     = "Alternating"
	Pulse           = "Pulse"
	Candle          = "Candle"
	StarryNight     = "Starry Night"
	RainbowFlow     = "Rainbow Flow"
	SuperRainbow    = "Super Rainbow"
	RainbowPulse    = "Rainbow Pulse"
	TaiChi          = "Tai Chi"
	WaterCooler     = "Water Cooler"
	Loading         = "Loading"
)

/*
krakenPace is each effect's period on the Kraken as E1 measured it: how long
the effect's one repeating step takes, per unit of the cooler's speed, which
is a period, so the step grows with the number. Linear in every effect that
has more than one speed. An effect the Kraken runs at one speed is given it
here as its rate at that speed.
*/
var krakenPace = map[string]time.Duration{
	Fading:          52 * time.Millisecond,  // each colour's crossfade; 40 is 2.1 s a colour (tuned)
	SpectrumWave:    20 * time.Millisecond,  // a turn; 250 is 5 s
	CoveringMarquee: 17 * time.Millisecond,  // one colour covering the chain; 150 is 2.5 s
	Alternating:     700 * time.Microsecond, // a step of half a block; 500 is 0.35 s
	Pulse:           74 * time.Millisecond,  // a pulse; 15 is 1.1 s (tuned)
	Breathing:       100 * time.Millisecond, // a breath; 20 is 2 s
	Candle:          4 * time.Millisecond,   // a flicker; 50 is 0.2 s
	StarryNight:     20 * time.Millisecond,  // a twinkle; 15 is 0.3 s
	RainbowFlow:     14 * time.Millisecond,  // a turn; 250 is 3.5 s
	SuperRainbow:    13 * time.Millisecond,  // a turn; 150 is 2 s
	RainbowPulse:    12 * time.Millisecond,  // a turn; 250 is 3 s
	TaiChi:          100 * time.Millisecond, // a turn; 30 is 3 s
	WaterCooler:     200 * time.Millisecond, // the bright band's turn; 30 is 6 s
	Loading:         600 * time.Millisecond, // a lap and its wait; 10 is 6 s
}

/*
krakenRange is the Kraken's slowest and fastest speed for each effect, in its
own units (sanshoku's catalogue), which set each renderer's period range so
the 0-to-100 speed covers what the Kraken does. One speed is widened to half
and twice it, so a person can still adjust the copy.
*/
var krakenRange = map[string][2]int{
	Fading:          {80, 10},
	SpectrumWave:    {350, 80},
	CoveringMarquee: {300, 75},
	Alternating:     {1600, 500},
	Pulse:           {25, 4},
	Candle:          {100, 25},
	StarryNight:     {25, 4},
	RainbowFlow:     {350, 80},
	SuperRainbow:    {350, 80},
	RainbowPulse:    {350, 80},
	TaiChi:          {60, 15},
	WaterCooler:     {60, 15},
	Loading:         {20, 5},
}

// krakenDefault is NZXT CAM's speed for each effect, which a copy given no
// speed runs at.
var krakenDefault = map[string]int{
	Fading: 40, SpectrumWave: 250, CoveringMarquee: 150, Alternating: 500, Pulse: 15,
	Candle: 50, StarryNight: 15, RainbowFlow: 250, SuperRainbow: 150, RainbowPulse: 250,
	TaiChi: 30, WaterCooler: 30, Loading: 10,
}

// periods is an effect's period at the slowest and the fastest speed.
func periods(name string) (slowest, fastest time.Duration) {
	r := krakenRange[name]
	return time.Duration(r[0]) * krakenPace[name], time.Duration(r[1]) * krakenPace[name]
}

/*
SpeedFor is the speed, 0 to 100, at which a renderer runs at the pace the
Kraken runs its effect of that name at the Kraken's own speed: how a device
follows the Kraken (spec 077 R3.5). False for a name the Kraken does not run.
*/
func SpeedFor(name string, kraken int) (int, bool) {
	per, ok := krakenPace[name]
	if !ok {
		return 0, false
	}
	slowest, fastest := breathingSlowest, breathingFastest
	if name != Breathing {
		slowest, fastest = periods(name)
	}
	want := time.Duration(kraken) * per
	fraction := float64(slowest-want) / float64(slowest-fastest)
	return min(max(int(math.Round(fraction*FastestSpeed)), SlowestSpeed), FastestSpeed), true
}

// krakenRenderers are the Kraken's effects, in the cooler's order.
func krakenRenderers() []Renderer {
	return []Renderer{
		{Name: Fading, Draw: fading, Animated: true, Paced: true, Colours: cycleColours},
		{Name: SpectrumWave, Draw: spectrumWave, Animated: true, Paced: true},
		{Name: CoveringMarquee, Draw: coveringMarquee, Animated: true, Paced: true, Colours: 2},
		{Name: Alternating, Draw: alternating, Animated: true, Paced: true, Colours: 2},
		{Name: Pulse, Draw: pulse, Animated: true, OneColour: true, Paced: true, Colours: cycleColours},
		{Name: Candle, Draw: candle, Animated: true, OneColour: true, Paced: true, Colours: 1},
		{Name: StarryNight, Draw: starryNight, Animated: true, OneColour: true, Paced: true, Colours: 1},
		{Name: RainbowFlow, Draw: rainbowFlow, Animated: true, Paced: true},
		{Name: SuperRainbow, Draw: superRainbow, Animated: true, Paced: true},
		{Name: RainbowPulse, Draw: rainbowPulse, Animated: true, Paced: true},
		{Name: TaiChi, Draw: taiChi, Animated: true, Paced: true, Colours: 2},
		{Name: WaterCooler, Draw: waterCooler, Animated: true, Paced: true, Colours: 2},
		{Name: Loading, Draw: loading, Animated: true, OneColour: true, Paced: true, Colours: 1},
	}
}

// The Kraken's default colours, NZXT CAM's, for an effect given none.
var (
	camPurple = colour.Colour{R: 0x39, B: 0x58}
	camGreen  = colour.Colour{G: 0xBC}
	camBlue   = colour.Colour{B: 0xBC}
	camRed    = colour.Colour{R: 0xBC}
	camAmber  = colour.Colour{R: 0xBC, G: 0x78}
)

// colours is the effect's colours, or the Kraken's defaults where it was
// given none.
func colours(p Params, most int, defaults ...colour.Colour) []colour.Colour {
	if got := p.palette(most); len(got) > 0 {
		return got
	}
	return defaults
}

// at is where a light sits: its thing, its place round or along it from 0 to
// 1, its place along its zone's chain of things from 0 to 1, and which thing
// of its zone it is, of how many.
type at struct {
	thing, ofZone, things int
	// index is the light's place in its thing, of size.
	index, size int
	pos, chain  float64
}

/*
layout is each light's place. A light in no thing, and every light where no
thing is given, is one strip of the whole canvas, so an effect still draws on
a device nobody has described.
*/
func layout(keys []lighting.Key, p Params) []at {
	out := make([]at, len(keys))
	things := p.Things
	if len(things) == 0 {
		all := make([]int, len(keys))
		for i := range all {
			all[i] = i
		}
		things = []Thing{{Lights: all}}
	}
	inZone := map[int][]int{}
	for t, th := range things {
		inZone[th.Zone] = append(inZone[th.Zone], t)
	}
	for t, th := range things {
		zone := inZone[th.Zone]
		index := 0
		for k, other := range zone {
			if other == t {
				index = k
			}
		}
		n := float64(max(len(th.Lights), 1))
		for k, light := range th.Lights {
			if light < 0 || light >= len(out) {
				continue
			}
			pos := float64(k) / n
			out[light] = at{
				thing: t, ofZone: index, things: len(zone), index: k, size: len(th.Lights),
				pos: pos, chain: (float64(index) + pos) / float64(len(zone)),
			}
		}
	}
	return out
}

// phase is how far through its period an effect is at t, from 0 to 1.
func phase(t, period time.Duration) float64 {
	if period <= 0 {
		return 0
	}
	return float64(t%period) / float64(period)
}

// krakenPeriod is an effect's period at the speed asked for, or at CAM's
// where none was.
func krakenPeriod(name string, p Params) time.Duration {
	if p.Speed == nil {
		return time.Duration(krakenDefault[name]) * krakenPace[name]
	}
	slowest, fastest := periods(name)
	return pace(p, slowest, fastest)
}

// fading is every light one colour, crossfading through the colours in turn.
func fading(t time.Duration, keys []lighting.Key, p Params) []lighting.Pixel {
	cs := colours(p, cycleColours, camPurple, camGreen, camBlue)
	step := krakenPeriod(Fading, p)
	c := along(cs, phase(t, step*time.Duration(len(cs))))
	return paint(keys, p, func(int) colour.Colour { return c })
}

// spectrumWave is the whole spectrum round each thing, turning clockwise.
func spectrumWave(t time.Duration, keys []lighting.Key, p Params) []lighting.Pixel {
	turn := phase(t, krakenPeriod(SpectrumWave, p))
	place := layout(keys, p)
	return paint(keys, p, func(i int) colour.Colour { return hue(360 * (place[i].pos - turn)) })
}

/*
coveringMarquee is one colour sweeping clockwise along a zone's chain of
things and covering the other, then the other sweeping back over it.
*/
func coveringMarquee(t time.Duration, keys []lighting.Key, p Params) []lighting.Pixel {
	cs := colours(p, 2, camBlue, camGreen)
	a, b := cs[0], cs[len(cs)-1]
	sweep := krakenPeriod(CoveringMarquee, p)
	front := phase(t, sweep)
	if int(t/sweep)%2 == 1 {
		a, b = b, a
	}
	place := layout(keys, p)
	return paint(keys, p, func(i int) colour.Colour {
		if place[i].chain < front {
			return a
		}
		return b
	})
}

/*
alternating is blocks of four lights in the two colours moving clockwise, half
a block a step's time, each thing alike. It moves smoothly, each light blended
by how much of it each block covers: stepped two lights at a time it looked
choppier than the Kraken (spec 077, tuning).
*/
func alternating(t time.Duration, keys []lighting.Key, p Params) []lighting.Pixel {
	cs := colours(p, 2, camPurple, colour.Colour{G: 0xBC, B: 0x83})
	const block = 4.0
	shift := float64(t) / float64(max(krakenPeriod(Alternating, p), time.Millisecond)) * block / 2
	place := layout(keys, p)
	return paint(keys, p, func(i int) colour.Colour {
		return blend(cs[len(cs)-1], cs[0], firstShare(float64(place[i].index)-shift, block))
	})
}

/*
firstShare is how much of the light from x to x+1 lies in the first colour's
blocks, which repeat every two blocks and start at 0.
*/
func firstShare(x, block float64) float64 {
	period := 2 * block
	x = math.Mod(x, period)
	if x < 0 {
		x += period
	}
	in := func(from, to float64) float64 {
		// length of [from, to) inside [0, block) and [period, period+block)
		overlap := 0.0
		for _, start := range []float64{0, period} {
			lo, hi := math.Max(from, start), math.Min(to, start+block)
			if hi > lo {
				overlap += hi - lo
			}
		}
		return overlap
	}
	return in(x, x+1)
}

/*
pulse is a quick rise to full and a smooth fall to dark, with a moment dark
between pulses, every light together; several colours take a pulse each.
*/
func pulse(t time.Duration, keys []lighting.Key, p Params) []lighting.Pixel {
	period := krakenPeriod(Pulse, p)
	x := phase(t, period)
	var level float64
	// Tuned beside the Kraken: the fade takes most of the pulse, and the
	// moment dark is short.
	switch {
	case x < 0.10:
		level = x / 0.10
	case x < 0.97:
		level = (1 + math.Cos(math.Pi*(x-0.10)/0.87)) / 2
	}
	cs := p.palette(cycleColours)
	return paint(keys, p, func(i int) colour.Colour {
		c := p.base(i)
		if len(cs) > 0 {
			c = cs[int(t/period)%len(cs)]
		}
		return scaled(c, level)
	})
}

/*
noise is a smooth wander from 0 to 1 for each light, its own and not its
neighbours': value noise on a hash of the light and the step, eased between
steps.
*/
func noise(light int, x float64) float64 {
	step := math.Floor(x)
	f := x - step
	f = f * f * (3 - 2*f)
	v := func(n float64) float64 {
		h := uint64(light)*0x9E3779B97F4A7C15 ^ uint64(int64(n))*0xBF58476D1CE4E5B9 //nolint:gosec // a hash, not a quantity
		h ^= h >> 31
		h *= 0x94D049BB133111EB
		h ^= h >> 29
		return float64(h%10000) / 10000
	}
	return v(step) + (v(step+1)-v(step))*f
}

// candle is each light flickering on its own in quick, slight dips.
func candle(t time.Duration, keys []lighting.Key, p Params) []lighting.Pixel {
	c := colours(p, 1, camAmber)[0]
	x := float64(t) / float64(max(krakenPeriod(Candle, p), time.Millisecond))
	// Dips of up to half, tuned beside the Kraken: a third looked too subtle.
	return paint(keys, p, func(i int) colour.Colour { return scaled(c, 1-0.5*noise(i, x)) })
}

// starryNight is each light twinkling on its own between dim and bright.
func starryNight(t time.Duration, keys []lighting.Key, p Params) []lighting.Pixel {
	c := colours(p, 1, camPurple)[0]
	x := float64(t) / float64(max(krakenPeriod(StarryNight, p), time.Millisecond))
	return paint(keys, p, func(i int) colour.Colour {
		n := noise(i+7919, x)
		return scaled(c, 0.35+0.65*n*n)
	})
}

/*
rainbowFlow is a third of the spectrum spread along a zone's chain, moving
clockwise, its band drifting round the wheel so later turns show more of it.
*/
func rainbowFlow(t time.Duration, keys []lighting.Key, p Params) []lighting.Pixel {
	period := krakenPeriod(RainbowFlow, p)
	turn := phase(t, period)
	drift := 360 * float64(t) / float64(6*period)
	place := layout(keys, p)
	return paint(keys, p, func(i int) colour.Colour {
		// Out through the band and back, so there is no seam where its ends
		// meet: a band once round showed one, chunky beside the Kraken.
		x := math.Mod(place[i].chain-turn+1, 1)
		return hue(drift + 120*(1-math.Abs(2*x-1)))
	})
}

// superRainbow is the whole spectrum as a smooth gradient along a zone's
// chain, turning clockwise.
func superRainbow(t time.Duration, keys []lighting.Key, p Params) []lighting.Pixel {
	turn := phase(t, krakenPeriod(SuperRainbow, p))
	place := layout(keys, p)
	return paint(keys, p, func(i int) colour.Colour { return hue(360 * (place[i].chain - turn)) })
}

/*
rainbowPulse is two patches of colour circling a ring clockwise, fading to
black between them, their colours going round the wheel; on a zone of several
things, each has one patch of its own colour, staggered thing to thing.
*/
func rainbowPulse(t time.Duration, keys []lighting.Key, p Params) []lighting.Pixel {
	period := krakenPeriod(RainbowPulse, p)
	turn := phase(t, period)
	wheel := 360 * float64(t) / float64(4*period)
	place := layout(keys, p)
	return paint(keys, p, func(i int) colour.Colour {
		a := place[i]
		if a.things > 1 {
			stagger := float64(a.ofZone) / float64(a.things)
			return scaled(hue(wheel+120*float64(a.ofZone)), patch(a.pos-turn-stagger, 0))
		}
		near := patch(a.pos-turn, 0)
		far := patch(a.pos-turn, 0.5)
		if near >= far {
			return scaled(hue(wheel), near)
		}
		return scaled(hue(wheel+180), far)
	})
}

// patch is a soft patch of light centred at centre round a loop, 1 at its
// middle and 0 a quarter of the way round.
func patch(pos, centre float64) float64 {
	d := math.Abs(math.Mod(pos-centre+1.5, 1) - 0.5)
	if d >= 0.25 {
		return 0
	}
	return (1 + math.Cos(math.Pi*d/0.25)) / 2
}

/*
taiChi is two halves chasing each other clockwise round a ring, blended where
they meet. On a zone of several things each is one colour covering the other
until it is full, and then the other, staggered thing to thing.
*/
func taiChi(t time.Duration, keys []lighting.Key, p Params) []lighting.Pixel {
	cs := colours(p, 2, camRed, camBlue)
	a, b := cs[0], cs[len(cs)-1]
	period := krakenPeriod(TaiChi, p)
	turn := phase(t, period)
	place := layout(keys, p)
	return paint(keys, p, func(i int) colour.Colour {
		x := place[i]
		if x.things > 1 {
			stagger := float64(x.ofZone) / float64(x.things)
			cycle := phase(t+time.Duration(stagger*float64(period)), 2*period) * 2
			from, to := a, b
			if cycle >= 1 {
				from, to, cycle = b, a, cycle-1
			}
			return blend(from, to, edge(x.pos, cycle))
		}
		return blend(a, b, halves(x.pos-turn))
	})
}

/*
waterCooler is Tai Chi standing still, dim, with a bright band turning
clockwise over it. On a zone of several things each is one half's colour
with its own band, staggered thing to thing.
*/
func waterCooler(t time.Duration, keys []lighting.Key, p Params) []lighting.Pixel {
	cs := colours(p, 2, camRed, camBlue)
	a, b := cs[0], cs[len(cs)-1]
	turn := phase(t, krakenPeriod(WaterCooler, p))
	place := layout(keys, p)
	return paint(keys, p, func(i int) colour.Colour {
		x := place[i]
		base := blend(a, b, halves(x.pos))
		band := patch(x.pos-turn, 0)
		if x.things > 1 {
			base = a
			if x.ofZone%2 == 1 {
				base = b
			}
			band = patch(x.pos-turn-float64(x.ofZone)/float64(x.things), 0)
		}
		return scaled(base, 0.3+0.7*band)
	})
}

/*
loading is a lap of light: its head runs clockwise from the top while its
tail, fading to black, follows; at the top the head waits until the tail has
caught it and the ring is dark, and it starts again. Staggered thing to thing
on a zone of several.
*/
func loading(t time.Duration, keys []lighting.Key, p Params) []lighting.Pixel {
	c := colours(p, 1, camPurple)[0]
	period := krakenPeriod(Loading, p)
	place := layout(keys, p)
	return paint(keys, p, func(i int) colour.Colour {
		x := place[i]
		lap := phase(t, period)
		if x.things > 1 {
			lap = phase(t+time.Duration(float64(x.ofZone)/float64(x.things)*float64(period)), period)
		}
		head := math.Min(1, lap/0.6)
		tail := math.Max(0, (lap-0.4)/0.6)
		if head <= tail {
			return colour.Black
		}
		// How much of the light, from pos to the next, is lit, so the head
		// and the tail glide between lights rather than jump: lights switched
		// whole were choppy beside the Kraken (spec 077, tuning).
		width := 1 / float64(max(x.size, 1))
		lit := (math.Min(x.pos+width, head) - math.Max(x.pos, tail)) / width
		if lit <= 0 {
			return colour.Black
		}
		centre := math.Min(math.Max(x.pos+width/2, tail), head)
		return scaled(c, math.Min(1, lit)*(centre-tail)/(head-tail))
	})
}

// halves is how far a place round a loop is into the second of two halves,
// blended over a twelfth either side of where they meet: 0 in the first, 1 in
// the second.
func halves(pos float64) float64 {
	pos = math.Mod(pos+1, 1)
	const soft = 1.0 / 12
	switch {
	case pos < soft:
		return 0.5 - pos/(2*soft)
	case pos < 0.5-soft:
		return 0
	case pos < 0.5+soft:
		return (pos - (0.5 - soft)) / (2 * soft)
	case pos < 1-soft:
		return 1
	default:
		return 1 - (pos-(1-soft))/(2*soft)
	}
}

// edge is the covering colour's share at pos, its front at cover: 1 behind
// the front, 0 ahead of it, blended over a sixth.
func edge(pos, cover float64) float64 {
	const soft = 1.0 / 6
	return math.Min(1, math.Max(0, (cover-pos)/soft+0.5))
}

// blend is a towards b by f, from 0 to 1, channel by channel.
func blend(a, b colour.Colour, f float64) colour.Colour {
	mix := func(x, y uint8) uint8 { return uint8(math.Round(float64(x) + (float64(y)-float64(x))*f)) }
	return colour.Colour{R: mix(a.R, b.R), G: mix(a.G, b.G), B: mix(a.B, b.B)}
}
