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
	devices []api.Device
	cooling api.Cooling
	keys    api.KeysResponse

	// err is what went wrong reaching the service, which is the ordinary
	// first-run answer rather than a fault: nothing is running yet.
	err error
	// at is when this snapshot was taken, for the status bar.
	at time.Time
}

// Snapshot is everything the window draws from, copied out under the lock.
type Snapshot struct {
	Health  api.Health
	Devices []api.Device
	Cooling api.Cooling
	Keys    api.KeysResponse
	Err     error
	At      time.Time
}

// Read takes a consistent copy.
func (m *Machine) Read() Snapshot {
	m.mu.RLock()
	defer m.mu.RUnlock()

	devices := make([]api.Device, len(m.devices))
	copy(devices, m.devices)
	return Snapshot{
		Health: m.health, Devices: devices, Cooling: m.cooling,
		Keys: m.keys, Err: m.err, At: m.at,
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
	if err != nil {
		m.mu.Lock()
		m.err, m.at = err, time.Now()
		m.devices, m.cooling, m.keys = nil, api.Cooling{}, api.KeysResponse{}
		m.mu.Unlock()
		return
	}

	// The rest are best-effort: a machine with no cooler answers the cooling
	// route with Absent set, and a machine whose scenes file will not parse
	// has no keys to report. Neither is a reason to show nothing.
	devices, _ := client.Devices(ctx)
	cooling, _ := client.Cooling(ctx)
	keys, _ := client.Keys(ctx)

	m.mu.Lock()
	m.health, m.devices, m.cooling, m.keys = health, devices, cooling, keys
	m.err, m.at = nil, time.Now()
	m.mu.Unlock()
}

// Reachable reports whether the last refresh found the service.
func (m *Machine) Reachable() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.err == nil
}
