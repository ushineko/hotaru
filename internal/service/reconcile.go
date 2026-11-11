package service

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/ushineko/hotaru/internal/colour"
	"github.com/ushineko/hotaru/internal/devices"
	"github.com/ushineko/hotaru/internal/openrgb"
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
were recorded is **unfinished**, not successful: OpenRGB enumerates once at
server start, and a cold boot has been seen finding two devices out of six.
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

	_, client, addr := s.current()
	if client == nil {
		return Restore{}, unreachable(addr)
	}
	found, err := client.Devices(ctx)
	if err != nil {
		return Restore{}, err
	}

	present := make(map[string]*devices.Device, len(found))
	for i := range found {
		present[found[i].Name] = &found[i]
	}

	var restore Restore
	for _, name := range sorted(desired.Names()) {
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
		if !here && s.handedByRule(name) {
			/*
				An OpenRGB listing a canvas device draws, and not on the
				server now. It is not waited for: nothing would be written to
				it if it came back. OpenRGB's exit reboots the first canvas
				device, so a restarted server often starts before the device
				is back and does not list it at all.
			*/
			continue
		}
		if !here {
			// Recorded, and not on the server. The device may enumerate later,
			// which is why this is reported rather than forgotten.
			restore.Missing = append(restore.Missing, name)
			continue
		}
		if device.HandedTo != "" {
			// Recorded before hotaru drew on it, and OpenRGB's no longer: it
			// is the canvas device that is restored now (spec 060 R1.3).
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
		want := preference{mode: kept.Mode, insist: true, speed: kept.Speed}
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
			it: OpenRGB's exit reboots the first canvas device, and through
			its receiver nothing reports that. One frame each.
		*/
		s.mu.RLock()
		all := s.drawnNow()
		s.mu.RUnlock()
		for _, d := range all {
			d.Redraw()
		}
	}
	return restore, nil
}

/*
restoreOne writes one device's recorded state back, after giving its zones
back the sizes the frame was recorded with.

A zone's size lives in the OpenRGB server, not in the hardware, and a server
can come back without it: OpenRGB 1.1 did not read the sizes 1.0 had saved,
and a board's four addressable headers came up at 0 LEDs. The recorded frame
was then written to the one LED left, the write succeeded, and the board sat
in its firmware rainbow while every report said it was restored (spec 069).
*/
func (s *Service) restoreOne(ctx context.Context, client openrgb.Client,
	device *devices.Device, kept state.Device, want preference,
) Result {
	device, resized, problems := regrow(ctx, client, device, kept.Zones)
	if len(kept.Colours) != device.LEDCount {
		/*
			Written anyway. The LEDs the frame and the device share are still
			the right colours, and a device left in the server's power-on
			state is no better; but it is said, because "restored" is not
			true of the rest.
		*/
		problems = append(problems, fmt.Sprintf(
			"%s has an LED count of %d and its recorded colours are for %d; "+
				"a zone may have lost its size in OpenRGB",
			device.Name, device.LEDCount, len(kept.Colours)))
	}
	result := s.writeFrame(ctx, client, device, kept.Frame(device.Name), false, want, nil)
	result.Resized = resized
	result.Problems = append(result.Problems, problems...)
	return result
}

/*
regrow gives each of a device's zones that is smaller than recorded the size
it was recorded with, and returns the device as the server reports it after.

Only where the zone's size is one a person sets, the recorded size is within
what the zone allows, and the zone is smaller now. Never smaller: a zone with
more LEDs than recorded is somebody's change, not a loss.
*/
func regrow(ctx context.Context, client openrgb.Client, device *devices.Device,
	recorded map[string]int,
) (*devices.Device, []string, []string) {
	var resized, problems []string
	for _, zone := range device.Zones {
		want, ok := recorded[zone.Name]
		if !ok || zone.Count >= want || !zone.Fits(want) {
			continue
		}
		if err := client.ResizeZone(ctx, device.Name, zone.Name, want); err != nil {
			problems = append(problems, err.Error())
			continue
		}
		resized = append(resized, fmt.Sprintf("%s from %d to %d LEDs", zone.Name, zone.Count, want))
	}
	if len(resized) == 0 {
		return device, nil, problems
	}
	after, err := client.Device(ctx, device.Name)
	if err != nil {
		return device, resized, append(problems, err.Error())
	}
	return &after, resized, problems
}

/*
Reasserter re-sends colours to devices that do not hold them.

Some hardware forgets: a wireless mouse restores its onboard colour when it
wakes, and OpenRGB sets a volatile effect it has no way to make stick. The only
remedy is to send it again, on an interval, forever.

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
		// A canvas holds what it was sent and does not drift, so it is never
		// re-asserted; its twin is not hotaru's to write (spec 060 R3.6).
		if view.Device.Canvas != nil || view.Device.HandedTo != "" {
			continue
		}
		if view.InScope && view.Rule.Reassert > 0 {
			out[view.Device.Name] = view.Rule.Reassert
		}
	}
	return out, nil
}

func sorted(in []string) []string {
	out := append([]string(nil), in...)
	sort.Strings(out)
	return out
}
