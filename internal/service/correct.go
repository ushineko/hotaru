package service

import (
	"context"
	"strings"
	"sync"

	"github.com/ushineko/hotaru/internal/colour"
	"github.com/ushineko/hotaru/internal/config"
	"github.com/ushineko/hotaru/internal/devices"
	"github.com/ushineko/hotaru/internal/openrgb"
)

/*
corrected is the client with each device's colour profile applied (spec 066).

Every write passes through here on its way to the hardware, so a scene, a
`light set`, a preview and a reconcile are all corrected without knowing it.
Everything above this works in the colour that was meant: the read-back that
confirms a write, the listing the window draws, the base a partial scene
composes onto.

**Reads are translated by memory, not arithmetic.** A profile sends many
colours to one: at full value, #400000 and #800000 are both written #ff0000.
So the last thing asked for and the last thing written are kept per device,
and a light, or a mode's colour, the device reports in the written colour is
reported in the asked-for one. One in any other colour was changed by
something else, and is reported as the device gives it.
*/
type corrected struct {
	openrgb.Client
	cfg  *config.Config
	kept *corrections
}

// corrections is what was asked for and what was written, per device. It
// lives on the Service, because a corrected client is made per call.
type corrections struct {
	mu      sync.Mutex
	devices map[string]*correction
}

type correction struct {
	asked, wrote []colour.Colour
	// modes are a mode's own colours, by the mode's name in lower case.
	modes map[string][2][]colour.Colour
}

// profiled is whether any rule has a colour profile. With none the client is
// not wrapped, and nothing behaves differently from before spec 066.
func profiled(cfg *config.Config) bool {
	if cfg == nil {
		return false
	}
	for _, rule := range cfg.Devices {
		if rule.Colour != nil {
			return true
		}
	}
	return false
}

func (c *corrected) profile(device string) devices.Profile {
	return devices.RuleFor(c.cfg, device).Colour
}

func (c *corrected) SetMode(ctx context.Context, device, mode string, style openrgb.Style) error {
	profile := c.profile(device)
	if profile.Zero() || len(style.Palette()) == 0 {
		return c.Client.SetMode(ctx, device, mode, style) //nolint:wrapcheck // the client says which server
	}
	asked := style.Palette()
	written := style
	if style.Colour != nil {
		first := profile.Apply(*style.Colour)
		written.Colour = &first
	}
	written.Colours = profile.Colours(style.Colours)
	if err := c.Client.SetMode(ctx, device, mode, written); err != nil {
		return err //nolint:wrapcheck // the client says which server
	}
	c.kept.mode(device, mode, asked, profile.Colours(asked))
	return nil
}

func (c *corrected) SetFrame(ctx context.Context, device string, frame devices.Frame) error {
	profile := c.profile(device)
	if profile.Zero() {
		return c.Client.SetFrame(ctx, device, frame) //nolint:wrapcheck // the client says which server
	}
	written := devices.Frame{Device: frame.Device, Colours: profile.Colours(frame.Colours)}
	if err := c.Client.SetFrame(ctx, device, written); err != nil {
		return err //nolint:wrapcheck // the client says which server
	}
	c.kept.frame(device, frame.Colours, written.Colours)
	return nil
}

func (c *corrected) Devices(ctx context.Context) ([]devices.Device, error) {
	found, err := c.Client.Devices(ctx)
	if err != nil {
		return nil, err //nolint:wrapcheck // the client says which server
	}
	for i := range found {
		c.kept.translate(&found[i])
	}
	return found, nil
}

func (c *corrected) Device(ctx context.Context, name string) (devices.Device, error) {
	found, err := c.Client.Device(ctx, name)
	if err != nil {
		return found, err //nolint:wrapcheck // the client says which server
	}
	c.kept.translate(&found)
	return found, nil
}

func (k *corrections) entry(device string) *correction {
	if k.devices == nil {
		k.devices = map[string]*correction{}
	}
	e, ok := k.devices[device]
	if !ok {
		e = &correction{modes: map[string][2][]colour.Colour{}}
		k.devices[device] = e
	}
	return e
}

func (k *corrections) frame(device string, asked, wrote []colour.Colour) {
	k.mu.Lock()
	defer k.mu.Unlock()
	e := k.entry(device)
	e.asked = append([]colour.Colour(nil), asked...)
	e.wrote = wrote
}

func (k *corrections) mode(device, mode string, asked, wrote []colour.Colour) {
	k.mu.Lock()
	defer k.mu.Unlock()
	k.entry(device).modes[strings.ToLower(mode)] = [2][]colour.Colour{asked, wrote}
}

// translate reports a device's colours as they were asked for, where it
// holds what was written for them.
func (k *corrections) translate(d *devices.Device) {
	k.mu.Lock()
	defer k.mu.Unlock()
	e, ok := k.devices[d.Name]
	if !ok {
		return
	}
	d.Colours = back(d.Colours, e.asked, e.wrote)
	for i := range d.Modes {
		kept, ok := e.modes[strings.ToLower(d.Modes[i].Name)]
		if !ok {
			continue
		}
		d.Modes[i].Colours = back(d.Modes[i].Colours, kept[0], kept[1])
		if got := back([]colour.Colour{d.Modes[i].Colour}, kept[0], kept[1]); len(got) == 1 {
			d.Modes[i].Colour = got[0]
		}
	}
}

/*
back is reported colours with each one that matches what was written in its
place given as what was asked for there.

By position, because the same written colour can stand for different asked
ones on different lights. A new list, so a device a caller holds is not
changed behind its back.
*/
func back(reported, asked, wrote []colour.Colour) []colour.Colour {
	if reported == nil {
		return nil
	}
	out := append([]colour.Colour(nil), reported...)
	for i := range min(len(out), len(asked), len(wrote)) {
		if out[i] == wrote[i] {
			out[i] = asked[i]
		}
	}
	return out
}
