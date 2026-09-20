package service

import (
	"context"
	"fmt"
)

/*
State is how well hotaru can see the machine's lighting.

Four states rather than a boolean, because they have four different remedies
and telling them apart is the difference between a program a stranger can fix
and one they uninstall. A server that is not running, a server running with
nothing attached, and a server showing hardware that scope excludes all look
identical from "no lights changed".
*/
type State string

const (
	// StateUnreachable is nothing listening. The remedy is to start the server.
	StateUnreachable State = "unreachable"
	// StateNoDevices is a server that answered and knows of no hardware at
	// all. Usually permissions, or a server that started before the devices
	// did -- OpenRGB enumerates once, at startup.
	StateNoDevices State = "no-devices"
	// StateNoneInScope is hardware present with the rules file excluding all
	// of it: a configuration problem, and the only one of the four that is.
	StateNoneInScope State = "none-in-scope"
	// StateHealthy is hotaru seeing devices it is allowed to drive.
	StateHealthy State = "healthy"
)

// Health is what hotaru can see, and what to do if that is not enough.
type Health struct {
	State    State
	Address  string
	Protocol uint32
	Devices  int
	InScope  int
	Detail   string
}

// OK reports whether hotaru can drive anything.
func (h Health) OK() bool { return h.State == StateHealthy }

/*
Health looks, and says what to do about what it finds.

Every unhealthy state carries a remedy in the user's terms. "Connection refused
to 127.0.0.1:6742" is a true sentence that helps nobody; "no OpenRGB server is
running" is the same fact, actionable.
*/
func (s *Service) Health(ctx context.Context) Health {
	cfg, client, addr := s.current()
	health := Health{Address: addr}

	if client == nil {
		health.State = StateUnreachable
		health.Detail = fmt.Sprintf("no OpenRGB server at %s. Start it, and lighting works from then on.", addr)
		return health
	}
	health.Protocol = client.ProtocolVersion()

	found, err := client.Devices(ctx)
	if err != nil {
		health.State = StateUnreachable
		health.Detail = fmt.Sprintf("the OpenRGB server at %s stopped answering: %v", addr, err)
		return health
	}
	health.Devices = len(found)

	if len(found) == 0 {
		health.State = StateNoDevices
		health.Detail = "the OpenRGB server is running and knows of no devices. " +
			"It detects hardware once, when it starts, so a device connected since then is invisible until it restarts."
		return health
	}

	for _, device := range found {
		if cfg.InScope(device.Name) {
			health.InScope++
		}
	}
	if health.InScope == 0 {
		health.State = StateNoneInScope
		health.Detail = fmt.Sprintf("%d devices are present and scope excludes all of them. "+
			"Check the scope list in the rules file, or remove it to drive everything.", len(found))
		return health
	}

	health.State = StateHealthy
	health.Detail = fmt.Sprintf("%d of %d devices are in scope", health.InScope, len(found))
	return health
}
