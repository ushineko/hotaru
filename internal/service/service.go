/*
Package service is every operation hotaru can perform, with no view attached.

One core, and shells as adapters over it, so a capability cannot exist in the
CLI and not the GUI. Nothing here knows about HTTP, about Fyne, or about a
terminal: it takes a request and returns a result, and whether that result
becomes a table, a JSON body or a coloured fan is somebody else's business.

It is also where the two rules that make hotaru work on a stranger's machine
live. Scope defaults to every device present, so a machine with no
configuration is lit rather than ignored. And a write is read back, so a device
that accepts a mode without honouring it is discovered rather than assumed --
which is how the corrections in a rules file stop being a prerequisite.
*/
package service

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/ushineko/hotaru/internal/colour"
	"github.com/ushineko/hotaru/internal/config"
	"github.com/ushineko/hotaru/internal/devices"
	"github.com/ushineko/hotaru/internal/openrgb"
)

// Service performs hotaru's operations against one OpenRGB server.
type Service struct {
	mu     sync.RWMutex
	cfg    *config.Config
	client openrgb.Client
	addr   string
}

// New is a service over a client. A nil client is a server that is not there,
// which is an ordinary state rather than an error: every operation reports it
// and none of them panic.
func New(cfg *config.Config, client openrgb.Client, address string) *Service {
	if cfg == nil {
		cfg = &config.Config{}
	}
	if address == "" {
		address = openrgb.DefaultAddress
	}
	return &Service{cfg: cfg, client: client, addr: address}
}

// SetClient swaps the connection, for a server that came back.
func (s *Service) SetClient(client openrgb.Client) {
	s.mu.Lock()
	s.client = client
	s.mu.Unlock()
}

// SetConfig swaps the configuration, for a file that was reloaded.
func (s *Service) SetConfig(cfg *config.Config) {
	if cfg == nil {
		cfg = &config.Config{}
	}
	s.mu.Lock()
	s.cfg = cfg
	s.mu.Unlock()
}

func (s *Service) current() (*config.Config, openrgb.Client, string) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.cfg, s.client, s.addr
}

/*
View is a device as hotaru sees it: what it reports, plus what hotaru would do
with it.

InScope and Rule are here rather than left to the caller because every shell
needs them and they are derived the same way each time -- and because "why is
this device not changing?" is a question the listing should answer on its own.
*/
type View struct {
	Device  devices.Device
	InScope bool
	Rule    devices.Rule
}

// List is every device the server has, with scope and rules resolved.
func (s *Service) List(ctx context.Context) ([]View, error) {
	cfg, client, addr := s.current()
	if client == nil {
		return nil, unreachable(addr)
	}

	found, err := client.Devices(ctx)
	if err != nil {
		return nil, err
	}

	out := make([]View, 0, len(found))
	for _, device := range found {
		out = append(out, View{
			Device:  device,
			InScope: cfg.InScope(device.Name),
			Rule:    devices.RuleFor(cfg, device.Name),
		})
	}
	return out, nil
}

/*
Request is what to apply.

Assignments are targets and colours; Off turns devices off instead. Devices
narrows to particular hardware by name substring, for a caller who means one
device rather than everything in scope.
*/
type Request struct {
	// Colour, when set, applies to every device the request covers, before
	// any assignments. "Everything blue, except the top fan" is this plus one
	// assignment, which is how a caller says it and how it is stored.
	Colour *colour.Colour

	Assignments []devices.Assignment
	Off         bool
	Devices     []string
}

/*
Result is what happened to one device.

Three outcomes, kept apart on purpose. Applied is a write that was confirmed by
reading the device back. Skipped is a device that could not express the request
and said why -- not a failure, and not something to retry. Err is the server or
the hardware going wrong.

A caller reports all three per device, because "the scene worked" is not a
useful thing to say about six devices when one of them is dark.
*/
type Result struct {
	Device   string
	Mode     string
	Applied  bool
	Skipped  string
	Err      error
	Attempts []Attempt
}

// Attempt is one mode hotaru tried, and what the device did with it.
type Attempt struct {
	Mode     string
	Accepted bool   // the server took the write
	Active   string // what the device said it was in afterwards
	Why      string // why this attempt was abandoned, if it was
}

/*
Apply writes a request to every device it covers.

Per device: compose the frame, resolve the candidate modes, then try them in
order -- writing the mode, writing the frame, and reading the device back to see
whether what it says it is doing matches what it was asked to do. A device that
accepted a mode and did not take it falls through to the next candidate.

That read-back is what lets hotaru work on hardware nobody has written a rule
for. The ASUS board accepts Static, reports success, and leaves its addressable
headers dark; the fall-through finds Direct without being told, and the rules
file becomes an optimisation rather than a prerequisite.

**What it cannot catch**: a device already sitting in the mode it lies about.
The read-back compares the active mode, and a device that was in Static before
the write is in Static after it, so there is nothing to notice. The lie is
invisible for exactly as long as nothing else moves that device. For that case
the rules file earns its keep, and `probe` -- which sets a mode, looks, and
asks -- is how the rule gets written without the user reasoning it out.
*/
func (s *Service) Apply(ctx context.Context, req Request) ([]Result, error) {
	cfg, client, addr := s.current()
	if client == nil {
		return nil, unreachable(addr)
	}

	found, err := client.Devices(ctx)
	if err != nil {
		return nil, err
	}

	var results []Result
	for i := range found {
		device := found[i]
		if !cfg.InScope(device.Name) {
			continue
		}
		if !named(req.Devices, device.Name) {
			continue
		}

		assignments := forDevice(req.Assignments, device.Name)
		if req.Colour != nil {
			whole := devices.Assignment{Target: devices.Target{Device: device.Name}, Colour: *req.Colour}
			assignments = append([]devices.Assignment{whole}, assignments...)
		}
		if len(assignments) == 0 && !req.Off {
			continue
		}
		results = append(results, s.applyOne(ctx, client, cfg, &device, assignments, req.Off))
	}
	return results, nil
}

func (s *Service) applyOne(ctx context.Context, client openrgb.Client, cfg *config.Config,
	device *devices.Device, assignments []devices.Assignment, off bool,
) Result {
	result := Result{Device: device.Name}
	rule := devices.RuleFor(cfg, device.Name)

	var frame devices.Frame
	if off {
		frame = devices.Solid(device, colour.Black)
	} else {
		composed, problems := devices.Compose(device, rule, device.Colours, assignments)
		if len(problems) > 0 {
			// Every assignment for this device failed to resolve; there is
			// nothing to write, and the reason is the useful part.
			if len(problems) == len(assignments) {
				result.Skipped = problems[0].Error()
				return result
			}
		}
		frame = composed
	}

	want := devices.Want{Off: off, PerLED: frame.PerLED()}
	candidates := device.SolidCandidates(rule, want)
	if off {
		if _, err := device.Resolve(rule, want); err != nil {
			result.Skipped = reason(err)
			return result
		}
		candidates = offCandidates(device)
	}
	if len(candidates) == 0 {
		_, err := device.Resolve(rule, want)
		result.Skipped = reason(err)
		return result
	}

	for _, mode := range candidates {
		attempt := Attempt{Mode: mode}

		if err := client.SetMode(ctx, device.Name, mode, rule.Brightness); err != nil {
			attempt.Why = err.Error()
			result.Attempts = append(result.Attempts, attempt)
			continue
		}
		attempt.Accepted = true

		if !isOffMode(mode) {
			if err := client.SetFrame(ctx, device.Name, frame); err != nil {
				attempt.Why = err.Error()
				result.Attempts = append(result.Attempts, attempt)
				continue
			}
		}

		// The write is not finished until the device agrees it happened.
		after, err := client.Device(ctx, device.Name)
		if err != nil {
			attempt.Why = err.Error()
			result.Attempts = append(result.Attempts, attempt)
			result.Err = err
			return result
		}
		attempt.Active = after.ActiveMode
		result.Attempts = append(result.Attempts, attempt)

		if !strings.EqualFold(after.ActiveMode, mode) {
			// Accepted and not honoured. Nothing said so; only this read did.
			continue
		}

		result.Applied = true
		result.Mode = mode
		return result
	}

	result.Skipped = fmt.Sprintf("tried %s; none of them took", strings.Join(modesOf(result.Attempts), ", "))
	return result
}

func offCandidates(d *devices.Device) []string {
	var out []string
	for _, name := range devices.DefaultOffModes {
		if mode, ok := d.Mode(name); ok {
			out = append(out, mode.Name)
		}
	}
	return out
}

func isOffMode(mode string) bool { return strings.EqualFold(mode, "off") }

func modesOf(attempts []Attempt) []string {
	out := make([]string, 0, len(attempts))
	for _, a := range attempts {
		out = append(out, a.Mode)
	}
	return out
}

// forDevice is the assignments whose target names this device.
func forDevice(assignments []devices.Assignment, device string) []devices.Assignment {
	name := strings.ToLower(device)
	var out []devices.Assignment
	for _, a := range assignments {
		if strings.Contains(name, strings.ToLower(a.Target.Device)) {
			out = append(out, a)
		}
	}
	return out
}

// named reports whether a device is one the caller asked for. No names means
// every device in scope.
func named(wanted []string, device string) bool {
	if len(wanted) == 0 {
		return true
	}
	name := strings.ToLower(device)
	for _, want := range wanted {
		if strings.Contains(name, strings.ToLower(strings.TrimSpace(want))) {
			return true
		}
	}
	return false
}

func reason(err error) string {
	var unsupported *devices.Unsupported
	if err == nil {
		return ""
	}
	if ok := asUnsupported(err, &unsupported); ok {
		return unsupported.Why
	}
	return err.Error()
}
