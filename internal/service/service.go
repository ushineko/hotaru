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
	"time"

	"github.com/ushineko/hotaru/internal/colour"
	"github.com/ushineko/hotaru/internal/config"
	"github.com/ushineko/hotaru/internal/devices"
	"github.com/ushineko/hotaru/internal/openrgb"
	"github.com/ushineko/hotaru/internal/queue"
	"github.com/ushineko/hotaru/internal/state"
)

// Service performs hotaru's operations against one OpenRGB server.
type Service struct {
	mu       sync.RWMutex
	cfg      *config.Config
	client   openrgb.Client
	addr     string
	rules    string
	recorder Recorder
	queue    *queue.Set
	env      Environment
}

/*
SetQueue routes every write through one goroutine per device.

Without one, writes run on the caller's goroutine: correct, and fine for a test
or a one-shot command. With one, a reconcile and a user's scene cannot
interleave on the same device, and a write that is superseded before it runs is
replaced rather than queued behind the thing that countermanded it.
*/
func (s *Service) SetQueue(q *queue.Set) {
	s.mu.Lock()
	s.queue = q
	s.mu.Unlock()
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

// SetRulesPath is where the rules file lives, so the service can re-read it
// when asked. Empty means there is no file to reload, which is the ordinary
// case on a machine that has never been configured.
func (s *Service) SetRulesPath(path string) {
	s.mu.Lock()
	s.rules = path
	s.mu.Unlock()
}

/*
Reload re-reads the rules file.

Returns what was wrong with it, entry by entry, rather than refusing: a typo in
the third rule should cost that rule and nothing else, and the caller decides
how loudly to say so.
*/
func (s *Service) Reload() ([]config.Problem, error) {
	s.mu.RLock()
	path := s.rules
	s.mu.RUnlock()
	if path == "" {
		return nil, nil
	}

	cfg, problems, err := config.Load(path)
	if err != nil {
		return nil, err
	}
	s.SetConfig(cfg)
	return problems, nil
}

// RulesPath is the file Reload reads, for a status that says where settings
// came from.
func (s *Service) RulesPath() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.rules
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

	// Mode is a mode to prefer over the usual order. The wizard uses it once
	// it has found, by asking a person, which mode actually lights a device --
	// a question about the physical world, since a board can accept Static,
	// report Static, and leave its addressable headers dark.
	//
	// Preferred rather than forced: a mode that cannot carry the frame is
	// still no use, and silently showing one colour where three were asked for
	// would be a worse answer than choosing a mode that works.
	Mode string
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
	Device string
	Mode   string
	// Applied is a write that was confirmed by reading the device back.
	Applied bool
	// Skipped is a device that could not express the request, and why.
	Skipped string
	// Superseded is a write replaced by a newer one for the same device before
	// it ran. Not a failure: lighting is a state, and the newer request is the
	// state that was wanted.
	Superseded bool
	Err        error
	Attempts   []Attempt
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
		results = append(results, s.applyOne(ctx, client, cfg, &device, assignments, req.Off, req.Mode))
	}
	return results, nil
}

func (s *Service) applyOne(ctx context.Context, client openrgb.Client, cfg *config.Config,
	device *devices.Device, assignments []devices.Assignment, off bool, preferred string,
) Result {
	result := Result{Device: device.Name}
	rule := devices.RuleFor(cfg, device.Name)

	var frame devices.Frame
	if off {
		frame = devices.Solid(device, colour.Black)
	} else {
		composed, problems := devices.Compose(device, rule, s.base(device), assignments)
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

	result = s.through(ctx, device.Name, func(ctx context.Context) Result {
		return s.writeFrame(ctx, client, device, frame, off, preferred)
	})
	if result.Applied && !off {
		s.remember(device.Name, result.Mode, frame)
	}
	if result.Applied && off {
		// A device deliberately turned off has nothing to restore: putting
		// black back at boot is not what anybody meant by "off".
		s.forget(device.Name)
	}
	return result
}

/*
through runs a device's write on that device's own queue, when there is one.

A superseded write is reported as such rather than as a failure: the user asked
for something, and then asked for something else, and the second answer is the
one that matters.
*/
func (s *Service) through(ctx context.Context, device string, write func(context.Context) Result) Result {
	s.mu.RLock()
	q := s.queue
	s.mu.RUnlock()
	if q == nil {
		return write(ctx)
	}

	var result Result
	outcome := q.Do(device, func(ctx context.Context) { result = write(ctx) })
	if outcome.Superseded {
		return Result{Device: device, Superseded: true}
	}
	return result
}

/*
writeFrame is the write-and-confirm half, shared by applying and reconciling.

Reconciling must not be a second implementation of this: the fall-through, the
read-back and the reasons a device is skipped are the same facts whoever asked.
*/
func (s *Service) writeFrame(ctx context.Context, client openrgb.Client,
	device *devices.Device, frame devices.Frame, off bool, preferred string,
) Result {
	result := Result{Device: device.Name}
	rule := devices.RuleFor(s.config(), device.Name)

	want := devices.Want{Off: off, PerLED: frame.PerLED()}
	candidates := device.SolidCandidates(rule, want)
	if preferred != "" {
		mode, ok := device.Mode(preferred)
		switch {
		case !ok:
			result.Skipped = fmt.Sprintf("no mode called %q; it advertises %s",
				preferred, strings.Join(device.ModeNames(), ", "))
			return result
		case want.PerLED && !mode.PerLED:
			// Asked for a mode that cannot show the frame. Resolution carries
			// on rather than showing one colour and reporting success.
		default:
			candidates = append([]string{mode.Name}, without(candidates, mode.Name)...)
		}
	}
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

/*
base is what a partial assignment composes onto: the LEDs nobody mentioned.

What hotaru last wrote is preferred over what the device reports, because a
device's reported colours are only its LED buffer while it is in a per-LED
mode. Observed on real hardware: a board in Static reports its *mode* colour,
and a cooler in Static reports black while visibly lit. Composing "the top fan
red" onto either would blank everything else and call it a scene.

Falling back to what the device says is still better than assuming black, and
assuming black is what happens when hotaru has never written to this device and
the device will not say -- which is honest: nothing here knows what those lights
are showing.
*/
func (s *Service) base(device *devices.Device) []colour.Colour {
	remembered := s.Desired().Devices[device.Name]
	if len(remembered.Colours) == device.LEDCount && device.LEDCount > 0 {
		return remembered.Colours
	}
	if len(device.Colours) == device.LEDCount {
		return device.Colours
	}
	return nil
}

// remember records what a user asked for, so it can be put back.
func (s *Service) remember(name, mode string, frame devices.Frame) {
	s.mu.RLock()
	recorder := s.recorder
	s.mu.RUnlock()
	if recorder == nil {
		return
	}
	_ = recorder.Record(name, state.Device{
		Mode:    mode,
		Colours: append([]colour.Colour(nil), frame.Colours...),
		Applied: time.Now(),
	})
}

func (s *Service) forget(name string) {
	s.mu.RLock()
	recorder := s.recorder
	s.mu.RUnlock()
	if recorder != nil {
		_ = recorder.Forget(name)
	}
}

func (s *Service) config() *config.Config {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.cfg
}

// without is a list minus one entry, so a preferred mode is not offered twice.
func without(list []string, drop string) []string {
	out := make([]string, 0, len(list))
	for _, item := range list {
		if !strings.EqualFold(item, drop) {
			out = append(out, item)
		}
	}
	return out
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
