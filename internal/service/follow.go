package service

import (
	"maps"
	"math"
	"slices"
	"strings"

	"github.com/ushineko/hotaru/internal/colour"
	"github.com/ushineko/hotaru/internal/config"
	"github.com/ushineko/hotaru/internal/devices"
	"github.com/ushineko/hotaru/internal/render"
	"github.com/ushineko/hotaru/internal/scenes"
)

/*
Following (spec 077 R3.2): a device whose rule says follow takes its leader's
effect, colours and speed whenever a request gives the leader one, and the two
start in step (R3.5).

The request is what changes: each follower is given the leader's effect,
translated to what it can show, unless the request names an effect for it
itself, which wins. A device with no follow rule is untouched.
*/

/*
withFollowers is req with each follower of a leader it gives an effect to
given that effect too, and whether any was. The followers are added to the
request's devices where it names some.

Repeated until nothing more follows, so a follower of a follower takes the
first leader's effect. A loop of follows is never given one, because none of
its devices is given an effect for the others to take.
*/
func withFollowers(cfg *config.Config, found []devices.Device, req Request) (Request, bool) {
	followed := false
	effects := map[string]scenes.Effect{}
	for name, e := range req.Effects {
		effects[name] = e
	}
	for range found {
		added := false
		for i := range found {
			follower := &found[i]
			rule := devices.RuleFor(cfg, follower.Name)
			if rule.Follow == "" || follower.HandedTo != "" || effectFor(effects, follower.Name).Named() {
				continue
			}
			leader := leaderOf(found, rule.Follow, follower.Name)
			if leader == nil {
				continue
			}
			given := effectFor(effects, leader.Name)
			if !given.Named() && req.Mode != "" && named(req.Devices, leader.Name) {
				given = scenes.Effect{Mode: req.Mode}
			}

			if !given.Named() {
				continue
			}
			taken, ok := translate(given, leader, follower)
			if !ok {
				continue
			}
			effects[follower.Name] = taken
			if len(given.Palette()) == 0 && !mode0(follower, taken.Mode) {
				/*
					No colours given: the follower takes the ones the leader
					is lit in, known once the leader has been, whatever the
					request named it by (spec 077 R3.2).
				*/
				if req.fromLeader == nil {
					req.fromLeader = map[string]string{}
				}
				req.fromLeader[follower.Name] = leader.Name
			}
			if len(req.Devices) > 0 && !named(req.Devices, follower.Name) {
				req.Devices = append(slices.Clone(req.Devices), follower.Name)
			}
			followed, added = true, true
		}
		if !added {
			break
		}
	}
	if followed {
		req.Effects = effects
		req.Restart = true // started in step (R3.5)
	}
	return req, followed
}

// mode0 is whether a device's mode takes no colours of its own.
func mode0(device *devices.Device, mode string) bool {
	m, ok := device.Mode(mode)
	if !ok {
		return true
	}
	_, most := m.Slots()
	return most == 0
}

/*
withColours is effects with a follower's effect given colours, as many as its
mode takes, and one colour in every slot of a firmware effect that takes more,
as translate does.
*/
func withColours(effects map[string]scenes.Effect, follower *devices.Device, colours []colour.Colour) map[string]scenes.Effect {
	effect := effectFor(effects, follower.Name)
	mode, ok := follower.Mode(effect.Mode)
	if !ok || len(effect.Palette()) > 0 {
		return effects
	}
	named := make([]string, 0, len(colours))
	for _, c := range colours {
		named = append(named, c.String())
	}
	effect.SetPalette(fitPalette(named, mode))
	out := maps.Clone(effects)
	out[follower.Name] = effect
	return out
}

/*
fitPalette is colours as a mode takes them: as many as it takes, and one
colour in every slot of a firmware effect that takes more, so it stays one
colour (the card filled the rest with its own; E4).
*/
func fitPalette(colours []string, mode devices.Mode) []string {
	_, most := mode.Slots()
	if len(colours) > most {
		colours = colours[:most]
	}
	if mode.Stored && len(colours) == 1 && most > 1 {
		colours = slices.Repeat(colours, most)
	}
	return colours
}

// leaderOf is the device a follow rule names, by any part of its name, other
// than the follower and any name a device was formerly called.
func leaderOf(found []devices.Device, name, follower string) *devices.Device {
	for i := range found {
		d := &found[i]
		if d.Name != follower && d.HandedTo == "" &&
			strings.Contains(strings.ToLower(d.Name), strings.ToLower(strings.TrimSpace(name))) {
			return d
		}
	}
	return nil
}

/*
translate is the leader's effect as the follower can show it: the same effect
where it has one of that name, its own drawn copy taking the leader's speed
(render.SpeedFor), else the follower's best fit (R3.6). Colours are cut to as
many as the follower's effect takes.
*/
func translate(given scenes.Effect, leader, follower *devices.Device) (scenes.Effect, bool) {
	speed := given.Speed
	if from, ok := leader.Mode(given.Mode); ok && speed == nil && from.Speed != nil {
		now := from.Speed.Now
		speed = &now
	}
	out := scenes.Effect{Mode: given.Mode}
	mode, ok := follower.Mode(given.Mode)
	if ok && !mode.Stored && follower.LEDCount <= 1 {
		/*
			One light: a drawn copy would be the effect's colour over time at
			the device's few frames a second (the RTX 4090 takes about four),
			where an effect of its own runs smoothly. Its best fit, where it
			has one (R3.6).
		*/
		if _, fits := bestFit(given.Mode, follower, true); fits {
			ok = false
		}
	}
	if !ok {
		name, found := bestFit(given.Mode, follower, false)
		if !found {
			return scenes.Effect{}, false
		}
		out.Mode = name
		mode, _ = follower.Mode(name)
		out.Speed = fitSpeed(leader, given.Mode, speed, mode)
	} else if speed != nil {
		from, _ := leader.Mode(given.Mode)
		if mode.Stored && from.Stored && sameRange(from.Speed, mode.Speed) {
			out.Speed = speed // the same firmware, in the same units
		} else if mode.Stored {
			// Another firmware's effect of the same name, in its own units:
			// the RTX 4090's Breathing for the Kraken's (E4).
			out.Speed = fitSpeed(leader, given.Mode, speed, mode)
		} else if s, ok := render.SpeedFor(given.Mode, *speed); ok {
			out.Speed = &s
		}
	}
	out.SetPalette(fitPalette(given.Palette(), mode))
	return out, true
}

/*
bestFits is, for each Kraken effect, the effects of a device's own that come
closest, best first (spec 077 R3.6), for a follower without the effect: the
MSI card's, as OpenRGB names them. Tuned by eye (E4).
*/
var bestFits = map[string][]string{
	render.Fading:          {"Color Cycle", "Breathing"},
	render.SpectrumWave:    {"Rainbow Wave"},
	render.CoveringMarquee: {"Flowing", "Patrolling"},
	render.Alternating:     {"Fade In", "Double Flashing"},
	render.Pulse:           {"Breathing"},
	render.Breathing:       {"Breathing"},
	render.Candle:          {"Breathing"},
	render.StarryNight:     {"Breathing"},
	render.RainbowFlow:     {"Rainbow Wave"},
	render.SuperRainbow:    {"Rainbow Wave"},
	render.RainbowPulse:    {"Rainbow Wave", "Magic"},
	render.TaiChi:          {"Fade In"},
	render.WaterCooler:     {"Fade In"},
	render.Loading:         {"Flowing", "Patrolling"},
}

// bestFit is the follower's closest effect to the leader's, or none; with
// own, only an effect of the follower's own firmware.
func bestFit(effect string, follower *devices.Device, own bool) (string, bool) {
	for _, name := range bestFits[effect] {
		if m, ok := follower.Mode(name); ok && (m.Stored || !own) {
			return m.Name, true
		}
	}
	return "", false
}

/*
fitSpeed is the follower's speed for a best fit: as far through its range as
the leader's is through the leader's, which the follower's driver takes to the
nearest it has.
*/
func fitSpeed(leader *devices.Device, effect string, speed *int, to devices.Mode) *int {
	from, ok := leader.Mode(effect)
	if !ok || speed == nil || from.Speed == nil || to.Speed == nil || from.Speed.Slowest == from.Speed.Fastest {
		return nil
	}
	fraction := float64(*speed-from.Speed.Slowest) / float64(from.Speed.Fastest-from.Speed.Slowest)
	fraction = math.Min(1, math.Max(0, fraction))
	out := int(math.Round(float64(to.Speed.Slowest) + fraction*float64(to.Speed.Fastest-to.Speed.Slowest)))
	return &out
}

// sameRange is whether two speeds are in the same units: the same range.
func sameRange(a, b *devices.Speed) bool {
	return a != nil && b != nil && a.Slowest == b.Slowest && a.Fastest == b.Fastest
}

// leadersFirst is the devices with every leader a rule names before the
// devices that follow it, so a follower starts after its leader has (R3.5).
func leadersFirst(cfg *config.Config, found []devices.Device) []devices.Device {
	leads := map[string]bool{}
	for _, d := range found {
		if f := devices.RuleFor(cfg, d.Name).Follow; f != "" {
			if l := leaderOf(found, f, d.Name); l != nil {
				leads[l.Name] = true
			}
		}
	}
	out := slices.Clone(found)
	slices.SortStableFunc(out, func(a, b devices.Device) int {
		switch {
		case leads[a.Name] && !leads[b.Name]:
			return -1
		case leads[b.Name] && !leads[a.Name]:
			return 1
		}
		return 0
	})
	return out
}

/*
mergeDistance is how near two colours are to count as one when an effect
takes a scene's colours: near enough that nobody could tell them apart. A
picture's teals and greens a few steps apart are kept apart, because the
subtle spread is the scene (the maintainer, on the live run).
*/
const mergeDistance = 8

/*
sceneColours is up to most of a frame's colours for an effect that takes
several and was given none: the one most of the frame is, then each time the
one furthest from those chosen, until the rest are all but the same as one
already chosen; in the order the lights first show them. A frame from a
picture is many shades, and its first few lights' shades were not a fair
sample of its range (the live run).
*/
func sceneColours(frame devices.Frame, most int) []colour.Colour {
	distinct := frame.Distinct()
	if len(distinct) <= 1 || most <= 1 {
		return distinct[:min(len(distinct), max(most, 0))]
	}
	first := distinct[0]
	if dominant, ok := frame.Dominant(); ok && slices.Contains(distinct, dominant) {
		first = dominant
	}
	chosen := []colour.Colour{first}
	for len(chosen) < most {
		best, furthest := -1, 0.0
		for i, c := range distinct {
			near := math.Inf(1)
			for _, k := range chosen {
				near = math.Min(near, apart(c, k))
			}
			if near > furthest {
				best, furthest = i, near
			}
		}
		if best < 0 || furthest < mergeDistance {
			break
		}
		chosen = append(chosen, distinct[best])
	}
	slices.SortStableFunc(chosen, func(a, b colour.Colour) int {
		return slices.Index(distinct, a) - slices.Index(distinct, b)
	})
	return stretched(chosen)
}

// visibleSpread is how far apart the furthest two of an effect's colours from
// a scene are made to be, and stretchMost the most they are pushed for it.
const (
	visibleSpread = 120
	stretchMost   = 4.0
)

/*
stretched is a scene's colours pushed apart from their average, each in its
own direction, until the furthest two are visibly different: teal more teal,
olive more olive. A picture's colours on one device can sit so close that an
effect through them looks like no effect (space-2's teals and olives, a
shimmer nobody saw); the maintainer chose to keep them the scene's and make
them visible. Colours already far enough apart are left as they are.
*/
func stretched(colours []colour.Colour) []colour.Colour {
	if len(colours) < 2 {
		return colours
	}
	widest := 0.0
	var r, g, b float64
	for i, c := range colours {
		r, g, b = r+float64(c.R), g+float64(c.G), b+float64(c.B)
		for _, d := range colours[i+1:] {
			widest = math.Max(widest, apart(c, d))
		}
	}
	if widest >= visibleSpread || widest == 0 {
		return colours
	}
	n := float64(len(colours))
	r, g, b = r/n, g/n, b/n
	k := math.Min(stretchMost, visibleSpread/widest)
	push := func(v, mean float64) uint8 {
		return uint8(math.Round(math.Max(0, math.Min(255, mean+(v-mean)*k)))) //nolint:gosec // clamped to a byte
	}
	out := make([]colour.Colour, len(colours))
	for i, c := range colours {
		out[i] = colour.Colour{R: push(float64(c.R), r), G: push(float64(c.G), g), B: push(float64(c.B), b)}
	}
	return out
}

// apart is how far two colours are, in RGB.
func apart(a, b colour.Colour) float64 {
	dr, dg, db := float64(a.R)-float64(b.R), float64(a.G)-float64(b.G), float64(a.B)-float64(b.B)
	return math.Sqrt(dr*dr + dg*dg + db*db)
}

/*
coloursLitIn is the colours a leader's effect is lit in, for its followers:
the effect's own where it has them, else the scene's on the leader, as the
leader's own effect takes them.
*/
func (s *Service) coloursLitIn(cfg *config.Config, leader *devices.Device, assignments []devices.Assignment, req Request) []colour.Colour {
	effect := effectFor(req.Effects, leader.Name)
	if palette := effect.Palette(); len(palette) > 0 {
		var out []colour.Colour
		for _, named := range palette {
			if c, err := colour.Parse(named); err == nil {
				out = append(out, c)
			}
		}
		return out
	}
	frame, _ := devices.Compose(leader, devices.RuleFor(cfg, leader.Name), s.base(leader), assignments)
	return sceneColours(frame, 8)
}
