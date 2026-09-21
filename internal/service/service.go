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
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/ushineko/hotaru/internal/colour"
	"github.com/ushineko/hotaru/internal/config"
	"github.com/ushineko/hotaru/internal/cooler"
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
	cooler   Cooler
	panel    Dashboard
	scenes   SceneStore

	// leases maps a device to the preview held over it. One device, one
	// preview: see preview.go.
	leases map[string]*Lease
}

/*
Dashboard is the thing drawing the cooler's screen, if anything is.

The screen has one picture on it, so it has one author at a time. hotaru's own
dashboard is the default author and steps aside when somebody asks for
something else: a picture that is replaced two seconds later was not shown.
*/
type Dashboard interface {
	// Hold stops the dashboard drawing. Somebody else has the panel.
	Hold()
	// Release gives it back, and the dashboard redraws at once.
	Release()
}

// SetDashboard gives the service the dashboard to stand down, where there is
// one. A machine with no cooler, or a build that never started one, has none.
func (s *Service) SetDashboard(d Dashboard) {
	s.mu.Lock()
	s.panel = d
	s.mu.Unlock()
}

/*
Cooler is the liquid cooler, if this machine has one.

An interface so the service does not depend on how one is opened, and so a
machine with no cooler is the nil case rather than a special one: every method
below reports absence as an ordinary answer.
*/
type Cooler interface {
	Status(ctx context.Context) (cooler.Status, error)
	Device() cooler.Device

	// The screen, where the cooler has one. Every command here shares the
	// control channel with Status, so one owner serialises both.
	Show(ctx context.Context, gif []byte) error
	Readout(ctx context.Context) error
	Appearance(ctx context.Context, brightness, degrees int) error
}

/*
SetCooler gives the service a cooler to read.

Optional, always. No cooler means telemetry is absent and everything else --
lighting, reconciliation, the API -- is unaffected, which is spec 012's
degradation rule and the reason this is a setter rather than a constructor
argument.
*/
func (s *Service) SetCooler(c Cooler) {
	s.mu.Lock()
	s.cooler = c
	s.mu.Unlock()
}

/*
Cooling is the cooler's reading, or why there is none.

ErrNoCooler is the ordinary answer on a machine without one, and callers are
expected to show that as an absence rather than a fault.
*/
func (s *Service) Cooling(ctx context.Context) (cooler.Status, cooler.Device, error) {
	s.mu.RLock()
	c := s.cooler
	s.mu.RUnlock()

	if c == nil {
		return cooler.Status{}, cooler.Device{}, cooler.ErrNoCooler
	}
	status, err := c.Status(ctx)
	if err != nil {
		return cooler.Status{}, c.Device(), err
	}
	return status, c.Device(), nil
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
	/*
		Preview is the draft somebody is holding on this device, if any.

		Said out loud rather than left implicit: a machine showing something
		nobody chose is the confusion this project keeps running into, and a
		device whose re-assertion is suspended looks identical to one that is
		simply behaving.
	*/
	Preview *Lease
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
		view := View{
			Device:  device,
			InScope: cfg.InScope(device.Name),
			Rule:    devices.RuleFor(cfg, device.Name),
		}
		if lease, held := s.Previewing(device.Name); held {
			view.Preview = lease
		}
		out = append(out, view)
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

	/*
		Effects prefer a mode per device, keyed by any part of a device's name.

		What a scene carries, and the reason Mode above is not enough: a scene
		wants the keyboard rippling under typing and the cooler steady, which
		is two different modes in one application. Preferred rather than
		forced, exactly as Mode is.
	*/
	Effects map[string]string

	/*
		Preview writes without remembering.

		What the mapping wizard lights is a question, not an intention: it is
		asking which fan is green, and recording that would make "put the
		lights back" put the questions back. Desired state is what somebody
		asked their machine to look like, and a preview is by definition not
		that yet.
	*/
	Preview bool

	// Brightness overrides the rule's for this write. Nil leaves the rule in
	// charge, which is the ordinary case.
	Brightness *int

	// Exactly stops the fall-through. A caller asking what one particular mode
	// does needs an answer about that mode: trying the next candidate and
	// reporting it as a success answers a question nobody asked, and looks to
	// the caller like the mode they named having worked.
	Exactly bool
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

	// Unconfirmed is a device that accepted everything and will not say what
	// it is showing. Neither a success nor a failure: some hardware reports a
	// stale buffer while displaying exactly what it was sent.
	Unconfirmed string

	// Problems are assignments that named something this device does not have.
	// Reported even when the write succeeded, because "everything blue, except
	// the top fan" with the fan misspelled is a device that went blue all over
	// and said it worked.
	Problems []string
}

// Attempt is one mode hotaru tried, and what the device did with it.
type Attempt struct {
	Mode     string
	Accepted bool   // the server took the write
	Active   string // what the device said it was in afterwards
	Showing  bool   // and whether it was showing the colours it was sent
	Settled  bool   // and whether that took a second write after the mode change
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
		results = append(results, s.applyOne(ctx, client, cfg, &device, assignments, req))
	}
	return results, nil
}

func (s *Service) applyOne(ctx context.Context, client openrgb.Client, cfg *config.Config,
	device *devices.Device, assignments []devices.Assignment, req Request,
) Result {
	off, preferred, exactly := req.Off, req.Mode, req.Exactly
	result := Result{Device: device.Name}
	if effect := effectFor(req.Effects, device.Name); effect != "" {
		preferred = effect
		if _, has := device.Mode(effect); !has {
			/*
				Reported, and then ignored. A scene naming an effect this
				device does not have costs the effect and not the scene: the
				colours are still what somebody asked for, and a name that
				resolves to nothing must not be silently indistinguishable
				from one that worked -- which is the case an effect hotaru
				renders itself will arrive into.
			*/
			result.Problems = append(result.Problems,
				fmt.Sprintf("%s has no effect called %q", device.Name, effect))
			preferred = req.Mode
		}
	}
	rule := devices.RuleFor(cfg, device.Name)

	var frame devices.Frame
	if off {
		frame = devices.Solid(device, colour.Black)
	} else {
		composed, problems := devices.Compose(device, rule, s.base(device), assignments)
		for _, problem := range problems {
			result.Problems = append(result.Problems, problem.Error())
		}
		if len(problems) == len(assignments) {
			// Every assignment for this device failed to resolve; there is
			// nothing to write, and the reason is the useful part.
			result.Skipped = problems[0].Error()
			return result
		}
		frame = composed
	}

	problems := result.Problems
	result = s.through(ctx, device.Name, func(ctx context.Context) Result {
		return s.writeFrame(ctx, client, device, frame, off, preferred, exactly, req.Brightness)
	})
	result.Problems = problems
	if result.Applied && !off && !req.Preview {
		s.remember(device.Name, result.Mode, frame)
	}
	if result.Applied && off && !req.Preview {
		// A device deliberately turned off has nothing to restore: putting
		// black back at boot is not what anybody meant by "off".
		s.forget(device.Name)
	}
	return result
}

/*
effectFor is the mode a request names for one device.

Matched by substring and case-insensitively, the way every device name a person
types is matched here: a scene says "keychron" and the hardware calls itself
"Keychron K4 HE".
*/
func effectFor(effects map[string]string, device string) string {
	for name, effect := range effects {
		if name == "" || effect == "" {
			continue
		}
		if strings.Contains(strings.ToLower(device), strings.ToLower(name)) {
			return effect
		}
	}
	return ""
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
	device *devices.Device, frame devices.Frame, off bool, preferred string, exactly bool,
	brightness *int,
) Result {
	result := Result{Device: device.Name}
	rule := devices.RuleFor(s.config(), device.Name)

	// A caller may override the rule's brightness for one write. The wizard
	// does, to show somebody what dimmer looks like before writing a rule.
	if brightness != nil {
		rule.Brightness = brightness
	}
	want := devices.Want{Off: off, PerLED: frame.PerLED()}
	candidates := device.SolidCandidates(rule, want)
	if preferred != "" {
		mode, ok := device.Mode(preferred)
		switch {
		case !ok:
			result.Skipped = fmt.Sprintf("no mode called %q; it advertises %s",
				preferred, strings.Join(device.ModeNames(), ", "))
			return result
		case want.PerLED && !mode.PerLED && !exactly:
			// Asked for a mode that cannot show the frame. Resolution carries
			// on rather than showing one colour and reporting success.
		case exactly:
			candidates = []string{mode.Name}
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

	// A mode that carries its own colour is given one, where the frame has a
	// single colour to give. Without it the device shows whatever the vendor
	// left in the mode, and writing the buffer afterwards changes nothing it
	// displays -- see spec 009.
	var modeColour *colour.Colour
	if uniform, ok := frame.Uniform(); ok && !off {
		modeColour = &uniform
	}

	for _, mode := range candidates {
		attempt := Attempt{Mode: mode}

		/*
			The mode packet goes every time, including when the device is
			already in that mode.

			It looks redundant and is not. On an NZXT cooler it is what
			commits the frame: with the packet suppressed, the ring and the
			fans ignored every write and kept whatever they were showing,
			while the rest of the machine changed colour around them. Measured
			by building both ways and looking at the case. See spec 010.
		*/
		if err := client.SetMode(ctx, device.Name, mode, rule.Brightness, modeColour); err != nil {
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
		attempt.Showing = showing(after, frame)

		if !attempt.Showing && strings.EqualFold(after.ActiveMode, mode) {
			/*
				The mode took and the colours did not. Before concluding the
				device ignores them, write them once more.

				A mode change is not instant on every bus. Over SMBus to a
				stick of DDR5 it is slow enough that a frame written
				immediately afterwards lands while the controller is still
				changing mode, and is discarded -- which is why OpenRGB's own
				CLI, which sends the two as separate commands with a round
				trip between them, lights hardware that hotaru could not.
			*/
			if again := s.settle(ctx, client, device.Name, frame); again != nil {
				attempt.Showing = showing(*again, frame)
				attempt.Settled = attempt.Showing
			}
		}
		result.Attempts = append(result.Attempts, attempt)

		if !strings.EqualFold(after.ActiveMode, mode) {
			// Accepted and not honoured. Nothing said so; only this read did.
			continue
		}
		if !attempt.Showing {
			/*
				The mode took, and the device does not report the colours it
				was sent.

				Not a failure. Sticks of DDR5 were observed reporting #000000
				while visibly red and white -- the writes had landed and the
				server's buffer was stale -- so refusing to call that applied
				would mark working hardware broken. The other reading, that the
				write went nowhere, is equally consistent with what can be seen
				from here, and nothing in the protocol distinguishes them.

				So it is recorded and reported, and the user decides. A person
				looking at the machine can tell in a second what no amount of
				reading back will settle.
			*/
			result.Unconfirmed = fmt.Sprintf(
				"%s took the mode and does not report the colours it was sent; "+
					"look at it to see whether the write arrived", device.Name)
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

/*
settleDelay is how long a slow controller is given to finish changing mode
before its colours are written again.

Long enough for an SMBus device, short enough that a person setting a colour
does not notice. Only ever paid by a device that did not show the first frame.
*/
const settleDelay = 120 * time.Millisecond

// settle writes the frame once more, after a pause, and reads the device back.
func (s *Service) settle(ctx context.Context, client openrgb.Client,
	device string, frame devices.Frame,
) *devices.Device {
	select {
	case <-ctx.Done():
		return nil
	case <-time.After(settleDelay):
	}

	if err := client.SetFrame(ctx, device, frame); err != nil {
		return nil
	}
	after, err := client.Device(ctx, device)
	if err != nil {
		return nil
	}
	return &after
}

/*
showing reports whether a device is displaying the frame it was sent.

Checked only where the answer means anything: the device has to report one
colour per LED, and be in a mode that carries colours per LED. A device in
Static reports its mode's colour rather than its buffer, and a device that
reports nothing at all is simply not saying -- neither is evidence of a failed
write, and treating them as one would fail every write to hardware that works.
*/
func showing(after devices.Device, frame devices.Frame) bool {
	mode, known := after.Mode(after.ActiveMode)
	if !known {
		return true // not a question this device can answer
	}
	if mode.PerLED {
		if len(after.Colours) != len(frame.Colours) || len(frame.Colours) == 0 {
			return true
		}
		return after.Showing().Equal(frame)
	}
	/*
		A mode that carries its own colour is checked against that colour.

		This is the case the comment above used to wave through. It is true
		that such a device's buffer proves nothing; it does not follow that
		nothing can be checked, and the thing that can be checked is the thing
		the device is actually displaying.
	*/
	if mode.ModeColour {
		want, uniform := frame.Uniform()
		if !uniform {
			return true // a frame this mode was never going to show
		}
		return mode.Colour == want
	}
	return true
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

/*
Screen is what to put on the cooler's panel.

Exactly one of these is acted on, checked at the edge rather than here: a
request that says two things at once is a caller's mistake and should be
refused where it can still be described.
*/
type Screen struct {
	// Image is a GIF to display. A still picture is not retained by the
	// firmware; one frame of a GIF is.
	Image []byte
	// Readout hands the panel back to the cooler's own display.
	Readout bool
	// Dashboard gives the screen back to hotaru's own dashboard, after
	// something else has had it.
	Dashboard bool
	// Brightness and Orientation are the panel's own settings, which the
	// device keeps across restarts.
	Brightness  *int
	Orientation *int
}

/*
Draw acts on the cooler's screen.

Absence is reported the same way a reading is: a machine with no cooler has no
panel, and that is an ordinary answer rather than a failure.
*/
func (s *Service) Draw(ctx context.Context, what Screen) error {
	s.mu.RLock()
	c, panel := s.cooler, s.panel
	s.mu.RUnlock()

	if c == nil {
		return cooler.ErrNoCooler
	}
	/*
		Anything that puts a picture on the panel takes it from the dashboard
		first, and only asking for the dashboard back gives it up. Appearance
		is not a picture -- brightness and orientation apply to whatever is
		showing -- so it leaves the authorship alone.
	*/
	if panel != nil && (what.Readout || len(what.Image) > 0) {
		panel.Hold()
	}
	switch {
	case what.Dashboard:
		if panel == nil {
			return errors.New("there is no dashboard on this machine")
		}
		panel.Release()
		return nil
	case what.Readout:
		return c.Readout(ctx)
	case len(what.Image) > 0:
		return c.Show(ctx, what.Image)
	case what.Brightness != nil || what.Orientation != nil:
		brightness, orientation := 100, 0
		if what.Brightness != nil {
			brightness = *what.Brightness
		}
		if what.Orientation != nil {
			orientation = *what.Orientation
		}
		return c.Appearance(ctx, brightness, orientation)
	}
	return errors.New("nothing to do: give an image, a brightness, an orientation, or ask for the readout or the dashboard")
}
