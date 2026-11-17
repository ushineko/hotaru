package service

import (
	"context"
	"fmt"
	"strings"

	"github.com/ushineko/hotaru/internal/devices"
)

/*
State is how well hotaru can see the machine's lighting.

Three states rather than a boolean, because they have different remedies: a
machine where hotaru finds nothing to draw, one where the rules exclude all of
it, and one it can drive.
*/
type State string

const (
	/*
		StateNoDevices is nothing found to draw: no device sanshoku drives
		is plugged in, or one is and hotaru may not open it.
	*/
	StateNoDevices State = "no-devices"
	// StateNoneInScope is hardware present with the rules file excluding all
	// of it: a configuration problem.
	StateNoneInScope State = "none-in-scope"
	// StateHealthy is hotaru seeing devices it is allowed to drive.
	StateHealthy State = "healthy"
)

// Health is what hotaru can see, and what to do if that is not enough.
type Health struct {
	State   State
	Devices int
	InScope int
	Detail  string

	// Remedies are things a person could do about it, each a sentence with a
	// command or a file in it. Offered, never performed.
	Remedies []string

	// Canvases are the devices hotaru draws, and what each is doing: a frame
	// rate while an effect moves, holding while it does not.
	Canvases []CanvasState
}

// OK reports whether hotaru can drive anything.
func (h Health) OK() bool { return h.State == StateHealthy }

// noDevicesRemedies is what a person can check when nothing is found.
var noDevicesRemedies = []string{
	"Check that the device is one hotaru supports: docs/hardware.md lists them.",
	"Check that hotaru's udev rules are installed (packaging/60-hotaru.rules); " +
		"without them a device is found and cannot be opened.",
}

/*
Health looks, and says what to do about what it finds. Every unhealthy state
carries a remedy in the user's terms.
*/
func (s *Service) Health(ctx context.Context) Health {
	cfg, client := s.current()
	health := Health{Canvases: s.Canvases()}

	found, _ := client.Devices(ctx)
	drawn := found[:0:0]
	for _, device := range found {
		if device.HandedTo == "" {
			drawn = append(drawn, device)
		}
	}
	health.Devices = len(drawn)
	if len(drawn) == 0 {
		health.State = StateNoDevices
		health.Detail = "hotaru has found no device to draw on."
		health.Remedies = noDevicesRemedies
		return health
	}

	for _, device := range drawn {
		if cfg.InScope(device.Name) {
			health.InScope++
		}
	}
	if health.InScope == 0 {
		health.State = StateNoneInScope
		health.Detail = fmt.Sprintf("%d devices are present and scope excludes all of them. "+
			"Check the scope list in the rules file, or remove it to drive everything.", len(drawn))
		return health
	}

	health.State = StateHealthy
	health.Detail = fmt.Sprintf("%d of %d devices are in scope", health.InScope, len(drawn))
	if short := s.misfits(drawn); len(short) > 0 {
		/*
			Still healthy: hotaru can drive every one of these. But a device
			whose LED count differs from its recorded colours shows part of
			what was asked for, and nothing else on the screen says so (spec
			069).
		*/
		health.Detail += ". " + strings.Join(short, ". ")
		health.Remedies = append(health.Remedies,
			"A header's length is a `lengths` rule; check it against the strip, then apply a scene again.")
	}
	return health
}

// misfits says, for each device whose LED count differs from the colours
// recorded for it, both numbers.
func (s *Service) misfits(found []devices.Device) []string {
	desired := s.Desired()
	var out []string
	for _, device := range found {
		kept, ok := desired.Devices[device.Name]
		if !ok || len(kept.Colours) == 0 || len(refit(&device, kept)) == device.LEDCount {
			continue
		}
		out = append(out, fmt.Sprintf("%s has an LED count of %d and its recorded colours are for %d",
			device.Name, device.LEDCount, len(kept.Colours)))
	}
	return out
}
