package service

import (
	"context"
	"fmt"
	"strings"

	"github.com/ushineko/hotaru/internal/devices"
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
	// of it: a configuration problem, and the only one of the five that is.
	StateNoneInScope State = "none-in-scope"
	// StateStale is a server answering, listing the device, and holding a dead
	// descriptor for it -- a device unplugged and plugged back in since the
	// server started. The listing is identical to a working one, which is why
	// this needs a state of its own rather than showing up as healthy.
	StateStale State = "stale"
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

	// Remedies are things a person could do about it, each a sentence with a
	// command in it. Offered, never performed: starting a daemon or enabling a
	// user manager at boot is the user's decision, and a program that made it
	// for them would be the other kind of annoying.
	Remedies []string
}

/*
Environment is what this machine could do about a problem.

An interface because the answer comes from systemd on Linux and from nothing at
all elsewhere, and because a test should be able to describe a machine rather
than run on one.
*/
type Environment interface {
	Remedies(ctx context.Context) []string

	/*
		Stale is whether the server holds a descriptor for a device node that
		has been removed, with the names of any devices present that it holds
		no descriptor for at all.

		Two answers because they carry different weight. The first is exact and
		is what the state rests on: a node is deleted or it is not. The second
		is for naming the device and nothing else -- most of the HID hardware
		on a machine is not lighting, and a server that has never heard of a
		webcam is not a server in trouble. See spec 058 R3 and R4.
	*/
	Stale(ctx context.Context) (moved []string, stale bool)

	/*
		Bounce restarts the OpenRGB server.

		The one thing hotaru does to a machine rather than about it, and it
		happens only when somebody asks. A server this user may not restart
		returns *NotOurs carrying the command they would type instead, which
		is an answer rather than a failure.
	*/
	Bounce(ctx context.Context) error
}

// SetEnvironment gives health somewhere to get its remedies.
func (s *Service) SetEnvironment(e Environment) {
	s.mu.Lock()
	s.env = e
	s.mu.Unlock()
}

func (s *Service) remedies(ctx context.Context) []string {
	env := s.environment()
	if env == nil {
		return nil
	}
	return env.Remedies(ctx)
}

func (s *Service) stale(ctx context.Context) (moved []string, stale bool) {
	env := s.environment()
	if env == nil {
		return nil, false
	}
	return env.Stale(ctx)
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
		health.Remedies = s.remedies(ctx)
		return health
	}
	health.Protocol = client.ProtocolVersion()

	found, err := client.Devices(ctx)
	if err != nil {
		health.State = StateUnreachable
		health.Detail = fmt.Sprintf("the OpenRGB server at %s stopped answering: %v", addr, err)
		health.Remedies = s.remedies(ctx)
		return health
	}
	health.Devices = len(found)

	if len(found) == 0 {
		health.Remedies = s.remedies(ctx)
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

	/*
		A server can look exactly like this and still be driving nothing.

		Checked last, because every cheaper question has already been answered
		and because this is the only state a full, in-scope listing does not
		rule out. The device count and the scope are what they have always
		been; the descriptor is the only thing that changed.
	*/
	if moved, stale := s.stale(ctx); stale {
		health.State = StateStale
		health.Detail = staleDetail(whichMoved(moved, found))
		// One way out, offered whether or not this user can take it: rescan
		// restarts the server where it is theirs to restart, and says what to
		// type where it is not. A remedy that first made someone work out
		// which of those they had would not be a remedy.
		health.Remedies = append([]string{
			"Have the server look again with `hotaru light rescan`. It restarts OpenRGB, " +
				"which takes a few seconds, and puts the lights back afterwards.",
		}, s.remedies(ctx)...)
		return health
	}

	health.State = StateHealthy
	health.Detail = fmt.Sprintf("%d of %d devices are in scope", health.InScope, len(found))
	return health
}

/*
whichMoved is which of the devices the server knows about are the ones that
moved.

The kernel and OpenRGB do not agree on what hardware is called -- a keyboard
the server lists as "Keychron K4 HE" appears to the kernel as "Keychron
Keychron K4 HE" -- so this matches on one name containing the other rather than
on equality. Anything that matches nothing is dropped: most of the HID devices
on a machine are a webcam, a headset or a power supply, and naming those would
be worse than naming nothing.
*/
func whichMoved(moved []string, found []devices.Device) []string {
	var out []string
	for _, device := range found {
		for _, name := range moved {
			if looksLike(device.Name, name) {
				out = append(out, device.Name)
				break
			}
		}
	}
	return out
}

func looksLike(device, hid string) bool {
	a, b := strings.ToLower(strings.Join(strings.Fields(device), " ")),
		strings.ToLower(strings.Join(strings.Fields(hid), " "))
	return a != "" && b != "" && (strings.Contains(b, a) || strings.Contains(a, b))
}

/*
staleDetail says what happened in the terms it happened in.

A device that can be named is named, because "the keyboard" is what the person
is looking at. One that cannot still gets the sentence: the server is holding a
handle to something that has gone, and that is true whether or not the hardware
left a name behind when it went.
*/
func staleDetail(moved []string) string {
	const cause = "A device was unplugged and plugged back in since the server started. " +
		"It detects hardware once, when it starts, so it is still writing to the device that went."

	if len(moved) == 0 {
		return "the OpenRGB server is holding a connection to a device that is no longer there. " + cause
	}
	return fmt.Sprintf("the OpenRGB server is still addressing %s at a connection that has gone. %s",
		strings.Join(moved, ", "), cause)
}
