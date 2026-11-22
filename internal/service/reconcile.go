package service

import (
	"context"
	"fmt"
	"slices"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/ushineko/hotaru/internal/colour"
	"github.com/ushineko/hotaru/internal/config"
	"github.com/ushineko/hotaru/internal/devices"
	"github.com/ushineko/hotaru/internal/state"
)

/*
Recorder is where desired state is kept. The service records what a user asked
for; something else decides where that lives.
*/
type Recorder interface {
	Snapshot() state.Snapshot
	Record(name string, device state.Device) error
	Forget(name string) error
	RecordScene(name string) error
	RecordScreen(screen state.Screen, image []byte) error
}

// SetRecorder gives the service somewhere to remember what was asked for, and
// takes back the name of the scene last applied before a restart.
func (s *Service) SetRecorder(r Recorder) {
	s.mu.Lock()
	s.recorder = r
	if r != nil {
		saved := r.Snapshot()
		if s.applied == "" {
			s.applied = saved.Scene
		}
		if s.showing == "" && saved.Screen != nil {
			s.showing = saved.Screen.Showing
		}
	}
	s.mu.Unlock()
}

// Desired is what the lights should currently be showing.
func (s *Service) Desired() state.Snapshot {
	s.mu.RLock()
	recorder := s.recorder
	s.mu.RUnlock()
	if recorder == nil {
		return state.Snapshot{}
	}
	return recorder.Snapshot()
}

/*
Restore is the outcome of reconciling toward desired state.

Missing is the part that matters. A restore that reached fewer devices than
were recorded is **unfinished**, not successful: devices appear over the first
seconds of a boot, and a cold boot has been seen finding two devices of six.
Reporting that as a success is how a machine ends up half-lit with everything
claiming to be fine.
*/
type Restore struct {
	Results []Result
	Applied int
	Missing []string
	// Previewing are devices left alone because somebody is looking at a
	// draft on them. Not missing and not applied: deliberately untouched.
	Previewing []string
}

// Complete reports whether every remembered device was reached.
func (r Restore) Complete() bool { return len(r.Missing) == 0 }

/*
Reconcile puts the lights back to what was last asked for.

Not an apply: nothing here is a new user choice. Desired state is not rewritten,
timestamps do not move, and a caller cannot tell a reconcile from the original
request by looking at what was remembered — which is what keeps a re-assert from
being mistaken for an instruction.

With nothing recorded it writes nothing at all. A fresh install has nothing to
restore, so it touches no device, and that is the inert rule in one sentence
rather than a special case.
*/
func (s *Service) Reconcile(ctx context.Context, only []string) (Restore, error) {
	desired := s.Desired()
	if desired.Empty() {
		return Restore{}, nil
	}

	_, client := s.current()
	found, err := client.Devices(ctx)
	if err != nil {
		return Restore{}, err
	}

	present := make(map[string]*devices.Device, len(found))
	for i := range found {
		present[found[i].Name] = &found[i]
	}
	desired = adopted(desired, found, s.config())

	/*
		On a full restore, leaders before the devices that follow them, and
		each of them started again, so their effects begin in step as they
		did when applied (spec 077 R3.5).
	*/
	cfg := s.config()
	inStep := map[string]bool{}
	var order []string
	for _, d := range leadersFirst(cfg, found) {
		if _, ok := desired.Devices[d.Name]; ok {
			order = append(order, d.Name)
		}
		if len(only) > 0 {
			continue
		}
		// Every recorded effect starts again on a full restore, leader or
		// not, so they begin in step as they did when applied (spec 084).
		if kept, ok := desired.Devices[d.Name]; ok && kept.Mode != "" && kept.Mode != canvasStill {
			inStep[d.Name] = true
		}
		if f := followOf(cfg, found, d.Name); f != "" {
			inStep[d.Name] = true
			if l := leaderOf(found, f, d.Name); l != nil {
				inStep[l.Name] = true
			}
		}
	}
	for _, name := range sorted(desired.Names()) {
		if !slices.Contains(order, name) {
			order = append(order, name)
		}
	}

	var restore Restore
	for _, name := range order {
		if len(only) > 0 && !named(only, name) {
			continue
		}
		if lease, held := s.Previewing(name); held {
			/*
				Somebody is looking at a draft on this device.

				Re-assertion exists to correct hardware that forgets, and here
				it would correct the person instead: the G502's timer puts
				desired state back within sixty seconds, underneath whoever is
				deciding whether they like the colour. The lease ends and this
				device is reconciled then, by Release.
			*/
			_ = lease
			restore.Previewing = append(restore.Previewing, name)
			continue
		}
		device, here := present[name]
		if !here {
			// Recorded, and not found. The device may be plugged in later,
			// which is why this is reported rather than forgotten.
			restore.Missing = append(restore.Missing, name)
			continue
		}
		if device.HandedTo != "" {
			// Recorded under a name a canvas was formerly called: it is the
			// canvas that is restored (spec 073).
			continue
		}

		/*
			The mode is restored as well as the colours.

			Desired state has always recorded the mode hotaru settled on, and
			reconciling used to discard it and resolve again from scratch.
			That was invisible while every scene was a solid colour, and wrong
			as soon as one carries an effect: a keyboard rippling under typing
			would come back from a re-assert sitting in Direct.

			Insisted on, for the same reason: this mode is one the device took
			and hotaru recorded, so a frame it cannot show every colour of is
			not grounds to resolve past it and undo the effect.

			With the mode's own colour and speed where a scene named them, so
			a re-assert does not put the right mode back in the colour the
			frame reduces to. See spec 051.
		*/
		kept := desired.Devices[name]
		want := preference{mode: kept.Mode, insist: true, speed: kept.Speed, restart: inStep[name]}
		if _, has := device.Mode(kept.Mode); !has {
			// Recorded through OpenRGB, in a mode the canvas does not have
			// ("Direct", or a firmware mode OpenRGB drove): the colours, in
			// Static (spec 075).
			want = preference{mode: canvasStill, insist: true, restart: inStep[name]}
		}
		for _, named := range kept.Named() {
			if c, err := colour.Parse(named); err == nil {
				want.colours = append(want.colours, c)
			}
		}
		result := s.through(ctx, device.Name, func(ctx context.Context) Result {
			return s.restoreOne(ctx, client, device, kept, want)
		})
		if result.Applied {
			restore.Applied++
		}
		restore.Results = append(restore.Results, result)
	}
	if len(only) == 0 {
		/*
			A canvas already showing the recorded scene was skipped above as
			unchanged, but a full restore is when its device most likely lost
			it: a device that rebooted or slept says nothing through its
			receiver. One frame each.
		*/
		s.redraw(nil)
	}
	return restore, nil
}

/*
Missing is the recorded devices not present now, sorted: what an unfinished
restore is waiting for. It writes nothing, so the reconciler can look for a
device that has turned up without restarting every effect (hotaru #240).
*/
func (s *Service) Missing(ctx context.Context) ([]string, error) {
	desired := s.Desired()
	if desired.Empty() {
		return nil, nil
	}
	_, client := s.current()
	found, err := client.Devices(ctx)
	if err != nil {
		return nil, err
	}
	desired = adopted(desired, found, s.config())
	present := make(map[string]bool, len(found))
	for _, d := range found {
		present[d.Name] = true
	}
	var out []string
	for _, name := range sorted(desired.Names()) {
		if !present[name] {
			out = append(out, name)
		}
	}
	return out, nil
}

/*
adopted is desired state with each canvas that has nothing recorded under its
own name given the record under its former one (spec 073), where that record
has a colour for each of the canvas's lights. The canvas draws a light at a
time in its Static mode; OpenRGB's mode and zone sizes mean nothing to it. The
recording itself is unchanged: the next write records the canvas under its own
name, and the former record stays for a rollback.
*/
func adopted(desired state.Snapshot, found []devices.Device, cfg *config.Config) state.Snapshot {
	var out map[string]state.Device
	for _, device := range found {
		if device.Canvas == nil {
			continue
		}
		if _, own := desired.Devices[device.Name]; own {
			continue
		}
		former := devices.RuleFor(cfg, device.Name).Formerly
		if former == nil {
			continue
		}
		for _, twin := range found {
			if twin.HandedTo != device.Name || !strings.EqualFold(twin.Name, former.Name) {
				continue
			}
			kept, ok := desired.Devices[twin.Name]
			if !ok || len(kept.Colours) != device.LEDCount {
				continue
			}
			if out == nil {
				out = make(map[string]state.Device, len(desired.Devices)+1)
				for name, d := range desired.Devices {
					out[name] = d
				}
			}
			out[device.Name] = state.Device{Mode: canvasStill, Colours: kept.Colours, Applied: kept.Applied}
			break
		}
	}
	if out != nil {
		desired.Devices = out
	}
	return desired
}

// canvasStill is the canvas mode that takes a colour per light.
const canvasStill = "Static"

/*
restoreOne writes one device's recorded state back, fitted to its zones as
they are now.
*/
func (s *Service) restoreOne(ctx context.Context, client machine,
	device *devices.Device, kept state.Device, want preference,
) Result {
	var problems []string
	kept.Colours = refit(device, kept)
	if len(kept.Colours) != device.LEDCount {
		/*
			Written anyway. The LEDs the frame and the device share are still
			the right colours; but it is said, because "restored" is not true
			of the rest.
		*/
		problems = append(problems, fmt.Sprintf(
			"%s has an LED count of %d and its recorded colours are for %d",
			device.Name, device.LEDCount, len(kept.Colours)))
	}
	result := s.writeFrame(ctx, client, device, kept.Frame(device.Name), false, want, nil)
	result.Problems = append(result.Problems, problems...)
	return result
}

/*
refit is a record's colours laid out for the device's zones as they are now.

A canvas's zones need not have the sizes OpenRGB recorded: a header's length
is a rule's now, and OpenRGB had it wrong (spec 075). Each recorded zone is
stretched or shrunk over the zone's length, so a fan's gradient goes round
the whole fan, and the zones after it start where they now start. A record
whose sizes do not add up to its colours, or that fits already, is returned
as it is.
*/
func refit(device *devices.Device, kept state.Device) []colour.Colour {
	if len(kept.Zones) == 0 || len(kept.Colours) == device.LEDCount {
		return kept.Colours
	}
	sizes := make([]int, len(device.Zones))
	total := 0
	for i, zone := range device.Zones {
		sizes[i] = zone.Count
		if n, ok := kept.Zones[zone.Name]; ok {
			sizes[i] = n
		}
		total += sizes[i]
	}
	if total != len(kept.Colours) {
		return kept.Colours
	}
	out := make([]colour.Colour, 0, device.LEDCount)
	from := 0
	for i, zone := range device.Zones {
		was := kept.Colours[from : from+sizes[i]]
		from += sizes[i]
		for j := range zone.Count {
			if len(was) == 0 {
				out = append(out, colour.Colour{})
				continue
			}
			out = append(out, was[j*len(was)/zone.Count])
		}
	}
	return out
}

/*
Reasserter re-sends colours to devices that do not hold them.

Some hardware forgets: a wireless mouse restores its onboard colour when it
wakes. The only remedy is to send it again, on an interval, forever.

Scoped to devices whose rule asks for it. Re-asserting everything would write
constantly to hardware that has no such problem, and a write nobody needs is
still a write that can collide with something else.
*/
type Reasserter struct {
	mu   sync.Mutex
	last map[string]time.Time
}

// NewReasserter tracks when each device was last re-sent.
func NewReasserter() *Reasserter { return &Reasserter{last: map[string]time.Time{}} }

/*
Due is the devices whose re-assert interval has elapsed.

A device is due the first time it is seen with a rule: hotaru has no idea what
happened while it was not running, and the cheapest way to find out is to send
what should be there.
*/
func (r *Reasserter) Due(now time.Time, rules map[string]time.Duration) []string {
	r.mu.Lock()
	defer r.mu.Unlock()

	var due []string
	for name, interval := range rules {
		if interval <= 0 {
			continue
		}
		last, seen := r.last[name]
		if !seen || now.Sub(last) >= interval {
			due = append(due, name)
			r.last[name] = now
		}
	}
	sort.Strings(due)
	return due
}

// Forget drops a device, so it is due again when it returns.
func (r *Reasserter) Forget(name string) {
	r.mu.Lock()
	delete(r.last, name)
	r.mu.Unlock()
}

/*
ReassertRules is the interval per device, for devices that have one.

Read from the configuration against the devices actually present, so a rule for
hardware this machine does not have costs nothing.
*/
func (s *Service) ReassertRules(ctx context.Context) (map[string]time.Duration, error) {
	views, err := s.List(ctx)
	if err != nil {
		return nil, err
	}
	out := map[string]time.Duration{}
	for _, view := range views {
		// A name a canvas was formerly called is not a device to write.
		if view.Device.HandedTo != "" {
			continue
		}
		if view.InScope && view.Rule.Reassert > 0 {
			out[view.Device.Name] = view.Rule.Reassert
		}
	}
	return out, nil
}

/*
Reassert restores the named devices, and sends each canvas among them the
frame it holds again: a device whose rule asks for re-assertion forgets what
it was sent (a mouse that wakes on its own lighting), so the frame it holds
is the one to send (spec 077 R5.1). A firmware effect is not written again by
it. Nothing else re-sends a held frame.
*/
func (s *Service) Reassert(ctx context.Context, names []string) (Restore, error) {
	restore, err := s.Reconcile(ctx, names)
	s.redraw(names)
	return restore, err
}

// redraw sends the canvases named, or every one for none, their frame again.
func (s *Service) redraw(names []string) {
	s.mu.RLock()
	all := s.drawnNow()
	s.mu.RUnlock()
	for _, d := range all {
		if len(names) == 0 || named(names, d.Name()) {
			d.Redraw()
		}
	}
}

func sorted(in []string) []string {
	out := append([]string(nil), in...)
	sort.Strings(out)
	return out
}
