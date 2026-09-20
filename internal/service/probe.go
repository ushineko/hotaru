package service

import (
	"context"
	"fmt"
	"strings"

	"github.com/ushineko/hotaru/internal/devices"
)

/*
Finding is what probing learned about one device.

Written to be read by a person: this is the output of a command someone runs
when their lights are not doing what they expected, and the useful part is
rarely the mode list. It is which modes the device claimed and then ignored.
*/
type Finding struct {
	Device string
	Modes  []ModeFinding
	Zones  []ZoneFinding

	// NoOffMode is a fact, not a judgement: the device advertises no Off mode,
	// so "off" resolves to writing black. Whether that is right depends on the
	// hardware -- black is off for a strip and a dead backlight for a
	// keyboard -- so the probe reports it and lets a person decide.
	NoOffMode bool

	// Suggested is the rule hotaru would write for this device, or empty when
	// the device needs no correction at all. Most do not.
	Suggested string

	// Err is a device that could not be probed, which is worth saying rather
	// than omitting the device from the report.
	Err error
}

// ModeFinding is one mode, and whether the device honoured it.
type ModeFinding struct {
	Name string
	// PerLED is what the device says about the mode: whether it accepts a
	// colour per LED, and so whether it can show more than one at a time.
	PerLED bool
	// Tried is whether hotaru set this mode to find out what happened.
	Tried bool
	// Took is whether the device was in this mode afterwards. A false here
	// with Tried true is the interesting case: accepted, and not honoured.
	Took bool
}

// ZoneFinding is a zone and its size, which is what segment naming starts from.
type ZoneFinding struct {
	Name  string
	Shape devices.Shape
	First int
	Count int
}

/*
Probe finds out what each device can actually do.

It sets modes, so it is a command a person runs deliberately rather than
something that happens to them. Every device is put back exactly as it was
found -- mode and colours -- because a diagnostic that leaves the lights
different has cost more than it explained.

What it cannot do is tell whether the lights physically changed. A board that
accepts Static and lights only its onboard LED reports Static either way, and
no amount of reading will say otherwise. That is what the mapping wizard is
for: it asks a person to look.
*/
func (s *Service) Probe(ctx context.Context, only []string) ([]Finding, error) {
	cfg, client, addr := s.current()
	if client == nil {
		return nil, unreachable(addr)
	}
	found, err := client.Devices(ctx)
	if err != nil {
		return nil, err
	}

	var out []Finding
	for i := range found {
		device := found[i]
		if !cfg.InScope(device.Name) || !named(only, device.Name) {
			continue
		}
		out = append(out, s.probeOne(ctx, client, &device))
	}
	return out, nil
}

func (s *Service) probeOne(ctx context.Context, client openrgbClient, device *devices.Device) Finding {
	finding := Finding{Device: device.Name}
	for _, zone := range device.Zones {
		finding.Zones = append(finding.Zones, ZoneFinding{
			Name: zone.Name, Shape: zone.Shape, First: zone.First, Count: zone.Count,
		})
	}

	_, hasOff := device.Mode("off")
	finding.NoOffMode = !hasOff

	// What to try: the modes that could carry a solid colour. Effects are left
	// alone -- setting Rainbow Wave to see whether it takes would be a light
	// show nobody asked for.
	candidates := device.SolidCandidates(devices.Rule{}, devices.Want{})
	before := *device

	var took []string
	for _, mode := range candidates {
		result := ModeFinding{Name: mode, Tried: true}
		if m, ok := device.Mode(mode); ok {
			result.PerLED = m.PerLED
		}

		if err := client.SetMode(ctx, device.Name, mode, nil); err != nil {
			finding.Modes = append(finding.Modes, result)
			continue
		}
		after, err := client.Device(ctx, device.Name)
		if err != nil {
			finding.Err = err
			break
		}
		result.Took = strings.EqualFold(after.ActiveMode, mode)
		if result.Took {
			took = append(took, mode)
		}
		finding.Modes = append(finding.Modes, result)
	}

	// Put it back exactly as it was found. A diagnostic that leaves the lights
	// different has cost more than it explained.
	if before.ActiveMode != "" {
		if err := client.SetMode(ctx, device.Name, before.ActiveMode, nil); err != nil && finding.Err == nil {
			finding.Err = fmt.Errorf("could not put %s back into %s: %w", device.Name, before.ActiveMode, err)
		}
	}
	if len(before.Colours) == before.LEDCount && before.LEDCount > 0 {
		if err := client.SetFrame(ctx, device.Name, before.Showing()); err != nil && finding.Err == nil {
			finding.Err = fmt.Errorf("could not put %s's colours back: %w", device.Name, err)
		}
	}

	finding.Suggested = suggest(took, candidates, finding.NoOffMode)
	return finding
}

/*
suggest is the rule hotaru would write, or nothing.

Nothing is the common and correct answer. A device whose first choice worked
needs no rule, and a probe that suggested one for every device would turn a
diagnostic into a configuration generator -- which is how a rules file ends up
full of lines nobody understands and nobody can safely delete.
*/
func suggest(took, tried []string, noOffMode bool) string {
	var lines []string

	// Only worth saying when the default order would have picked wrong: the
	// device's first preference is a mode it does not honour.
	if len(took) > 0 && len(tried) > 0 && !strings.EqualFold(took[0], tried[0]) {
		// The modes that worked first, then the ones that did not, as
		// fallbacks. Dropping them entirely would leave a device with one
		// option and nowhere to go if firmware changes under it.
		order := append(lower(took), missing(lower(tried), lower(took))...)
		lines = append(lines, fmt.Sprintf("solid_modes: [%s]", strings.Join(order, ", ")))
	}

	if noOffMode {
		// A question, not an instruction. The probe cannot see whether this
		// device is a keyboard whose backlight should stay on.
		lines = append(lines,
			"# this device has no Off mode, so `off` writes black to it.",
			"# if that blanks something that should stay lit, add:",
			"# never_blank: true")
	}

	if len(lines) == 0 {
		return ""
	}
	return strings.Join(lines, "\n")
}

// missing is the entries of all that are not in some, in order.
func missing(all, some []string) []string {
	have := make(map[string]bool, len(some))
	for _, s := range some {
		have[s] = true
	}
	var out []string
	for _, s := range all {
		if !have[s] {
			out = append(out, s)
		}
	}
	return out
}

func lower(in []string) []string {
	out := make([]string, len(in))
	for i, s := range in {
		out[i] = strings.ToLower(s)
	}
	return out
}

// openrgbClient is the part of the OpenRGB client this file uses, named so the
// signature above does not import the package for one type.
type openrgbClient interface {
	SetMode(ctx context.Context, device, mode string, brightness *int) error
	SetFrame(ctx context.Context, device string, frame devices.Frame) error
	Device(ctx context.Context, name string) (devices.Device, error)
}
