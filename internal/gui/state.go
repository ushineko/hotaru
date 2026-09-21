/*
Package gui is hotaru's window.

A client and nothing else. Every fact it shows and every action it offers is a
request over the same socket the CLI uses, so it holds no device handle, cannot
reach hardware even by mistake, and a second copy of it cannot contend with the
first. That is asserted by a test over the import graph rather than left to
discipline -- the same guarantee the CLI carries, for the same reason.

It owns its view state and nothing else: window geometry, colour scheme, the
section it was last on. The service owns the rules, the scenes and the desired
state, and never reads this program's file. See spec 017.
*/
package gui

import (
	"context"
	"sync"
	"time"

	"github.com/ushineko/hotaru/internal/api"
)

/*
Machine is what the window knows, refreshed on a timer.

One snapshot behind one lock, rather than each section fetching what it needs.
Two sections asking the same question of the service at different moments show
a machine that was never in that state, and the status bar reading one of them
while a section reads the other is how a window starts disagreeing with itself.
*/
type Machine struct {
	mu sync.RWMutex

	health  api.Health
	status  api.Status
	devices []api.Device
	cooling api.Cooling
	keys    api.KeysResponse
	scenes  []api.Scene

	// err is what went wrong reaching the service, which is the ordinary
	// first-run answer rather than a fault: nothing is running yet.
	err error
	// at is when this snapshot was taken, for the status bar.
	at time.Time
}

// Snapshot is everything the window draws from, copied out under the lock.
type Snapshot struct {
	Health api.Health
	// Status carries the last scene applied and what the panel is showing,
	// which the System section names.
	Status  api.Status
	Devices []api.Device
	Cooling api.Cooling
	Keys    api.KeysResponse
	Scenes  []api.Scene
	Err     error
	At      time.Time
}

/*
Same reports whether two snapshots would draw the same window.

The window rebuilds its section to show a change, and rebuilding is not free
to look at: a list rebuilt under a pointer jumps, and a scene somebody was
about to click moves out from under them. So the poll only rebuilds when
something actually moved -- which on an idle machine is never.

Taken is deliberately not compared. Every poll has a new one and none of them
are a change in the machine.
*/
func (s Snapshot) Same(other Snapshot) bool {
	if (s.Err == nil) != (other.Err == nil) || !sameHealth(s.Health, other.Health) {
		return false
	}
	if len(s.Devices) != len(other.Devices) || len(s.Scenes) != len(other.Scenes) {
		return false
	}
	for i := range s.Devices {
		if !sameDevice(s.Devices[i], other.Devices[i]) {
			return false
		}
	}
	for i := range s.Scenes {
		if s.Scenes[i].Name != other.Scenes[i].Name {
			return false
		}
	}
	return sameCooling(s.Cooling, other.Cooling)
}

// sameHealth compares what the Service section draws: the verdict and the
// counts, not the remedy list, which moves only when the verdict does.
func sameHealth(a, b api.Health) bool {
	return a.State == b.State && a.Detail == b.Detail &&
		a.Devices == b.Devices && a.InScope == b.InScope && a.Address == b.Address
}

// sameDevice compares what the window draws about one: its state and its
// colours, not the whole struct, which carries a mode list that never moves.
func sameDevice(a, b api.Device) bool {
	if a.Name != b.Name || a.ActiveMode != b.ActiveMode || a.InScope != b.InScope {
		return false
	}
	if (a.Preview == nil) != (b.Preview == nil) {
		return false
	}
	if len(a.Colours) != len(b.Colours) {
		return false
	}
	for i := range a.Colours {
		if a.Colours[i] != b.Colours[i] {
			return false
		}
	}
	return true
}

// sameCooling compares the numbers on screen. Coolant moves in tenths and the
// window shows tenths, so this is what a reader would call a change.
func sameCooling(a, b api.Cooling) bool {
	return a.Absent == b.Absent && a.Device == b.Device &&
		a.Coolant == b.Coolant && a.PumpRPM == b.PumpRPM && a.FanRPM == b.FanRPM
}

// Read takes a consistent copy.
func (m *Machine) Read() Snapshot {
	m.mu.RLock()
	defer m.mu.RUnlock()

	devices := make([]api.Device, len(m.devices))
	copy(devices, m.devices)
	scenes := make([]api.Scene, len(m.scenes))
	copy(scenes, m.scenes)
	return Snapshot{
		Health: m.health, Status: m.status, Devices: devices, Cooling: m.cooling,
		Keys: m.keys, Scenes: scenes, Err: m.err, At: m.at,
	}
}

/*
Refresh asks the service everything the window shows, in one pass.

A failure to reach the service replaces the snapshot rather than being merged
into it: a window showing last minute's devices next to "not running" is
telling two stories at once, and the stale half looks current.
*/
func (m *Machine) Refresh(ctx context.Context, client *api.Client) {
	health, err := client.Health(ctx)
	status, _ := client.Status(ctx)
	if err != nil {
		m.mu.Lock()
		m.err, m.at = err, time.Now()
		m.devices, m.cooling, m.keys, m.scenes = nil, api.Cooling{}, api.KeysResponse{}, nil
		m.status = api.Status{}
		m.mu.Unlock()
		return
	}

	// The rest are best-effort: a machine with no cooler answers the cooling
	// route with Absent set, and a machine whose scenes file will not parse
	// has no keys to report. Neither is a reason to show nothing.
	devices, _ := client.Devices(ctx)
	cooling, _ := client.Cooling(ctx)
	keys, _ := client.Keys(ctx)
	// Fetched here rather than while a section is being built: an HTTP call
	// inside a builder is a call made on every rebuild, and the window
	// rebuilds for reasons that have nothing to do with scenes.
	scenes, _ := client.Scenes(ctx)

	m.mu.Lock()
	m.health, m.status, m.devices, m.cooling, m.keys, m.scenes = health, status, devices, cooling, keys, scenes
	m.err, m.at = nil, time.Now()
	m.mu.Unlock()
}

// Reachable reports whether the last refresh found the service.
func (m *Machine) Reachable() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.err == nil
}
