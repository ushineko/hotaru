package service

import (
	"math"
	"slices"
	"strings"

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
		if mode.Stored {
			out.Speed = speed // the same firmware, in the same units
		} else if s, ok := render.SpeedFor(given.Mode, *speed); ok {
			out.Speed = &s
		}
	}
	colours := given.Palette()
	_, most := mode.Slots()
	if len(colours) > most {
		colours = colours[:most]
	}
	if fitted := out.Mode != given.Mode; fitted && len(colours) == 1 && most > 1 {
		/*
			A one-colour effect on a best fit that takes more: the colour in
			every slot, so it stays one colour. Left short, the card fills the
			rest with its own (Breathing went red and green; E4).
		*/
		colours = slices.Repeat(colours, most)
	}
	out.SetPalette(colours)
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
