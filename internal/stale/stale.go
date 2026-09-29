/*
Package stale asks whether the OpenRGB server is addressing a device that is
no longer there.

Here rather than in the service, and that is a deliberate exception to where
health is worked out. Every other verdict about the machine is the service's,
because the service is the thing holding the connection. This one cannot be:
reading another process's descriptors needs to be in the same mount namespace
as it, and the service runs under ProtectSystem, ProtectHome and PrivateTmp,
each of which is enough on its own to have every link come back permission
denied. Measured on the machine, one option at a time -- see spec 058.

Relaxing the unit was the alternative. It was not taken, because the six
options that would have to go include the one whose comment in that unit
records it catching a real bug (spec 016), and because this state is a
convenience rather than a repair: the bounce fixes the machine, and the bounce
works from inside the sandbox. Being told is worth less than the guardrail.

So the clients ask. Both of them -- the command line and the window -- are
ordinary processes of the user's, in the user's namespace, and can simply look.
*/
package stale

import (
	"context"
	"strings"
	"sync"

	"github.com/ushineko/hotaru/internal/systemd"
)

/*
State is what this verdict is called on the wire and on screen.

service.StateStale spells the same thing, and a test asserts they agree. The
two cannot simply share one constant without the clients importing the service
they exist to avoid linking against.
*/
const State = "stale"

// Healthy is the state this replaces when it applies. A server already known
// to be in trouble has a better answer than this one.
const Healthy = "healthy"

// Verdict is what to say when the server is holding a connection to something
// that has gone, and is empty when it is not or cannot be told.
type Verdict struct {
	Stale    bool
	Detail   string
	Remedies []string
}

/*
Checker answers the question, remembering where to look.

Finding the server's PID is several subprocesses and the answer changes only
when the server restarts; reading its descriptors is /proc and costs nothing.
A window asking every couple of seconds should not be running systemctl every
couple of seconds, so the expensive half is kept and the cheap half repeated.

The zero value works. A caller that asks once can ignore all of this.
*/
type Checker struct {
	mu  sync.Mutex
	pid int
}

/*
Apply puts this verdict onto a health report, and leaves one alone that
already has something better to say.

A server that is unreachable, knows of no devices, or has everything out of
scope is in a state with its own remedy, reached without looking at any
descriptor. This only ever replaces the verdict that says nothing is wrong.
*/
func (v Verdict) Apply(state, detail string, remedies []string) (string, string, []string) {
	if !v.Stale || state != Healthy {
		return state, detail, remedies
	}
	return State, v.Detail, append(append([]string(nil), v.Remedies...), remedies...)
}

// Check is whether the server has a dead connection to any of these devices.
// The names are what the server calls them, for saying which one moved.
func (c *Checker) Check(ctx context.Context, devices []string) Verdict {
	held := c.held(ctx)
	if !held.Known || !held.Deleted {
		return Verdict{}
	}
	return Verdict{
		Stale:  true,
		Detail: detail(named(held.Orphaned, devices)),
		// One way out, offered whether or not this user can take it: rescan
		// restarts the server where it is theirs to restart, and says what to
		// type where it is not. A remedy that first made someone work out
		// which of those they had would not be a remedy.
		Remedies: []string{
			"Have the server look again with `hotaru light rescan`. It restarts OpenRGB, " +
				"which takes a few seconds, and puts the lights back afterwards.",
		},
	}
}

// held reuses the PID it found last time for as long as that PID is still an
// OpenRGB server, and asks systemd again when it is not.
func (c *Checker) held(ctx context.Context) systemd.Handles {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.pid > 0 {
		if held := systemd.HeldBy(c.pid); held.Known {
			return held
		}
	}
	pid, ok := systemd.ServerPID(ctx)
	if !ok {
		c.pid = 0
		return systemd.Handles{}
	}
	c.pid = pid
	return systemd.HeldBy(pid)
}

/*
named is which of the devices the server knows about are the ones that moved.

The kernel and OpenRGB do not agree on what hardware is called -- a keyboard
the server lists as "Keychron K4 HE" appears to the kernel as "Keychron
Keychron K4 HE" -- so this matches on one name containing the other rather than
on equality. Anything matching nothing is dropped: most of the HID on a machine
is a webcam, a headset or a power supply that the server has no reason to hold,
and naming those would be worse than naming nothing.
*/
func named(moved, devices []string) []string {
	var out []string
	for _, device := range devices {
		for _, name := range moved {
			if looksLike(device, name) {
				out = append(out, device)
				break
			}
		}
	}
	return out
}

func looksLike(device, hid string) bool {
	a := strings.ToLower(strings.Join(strings.Fields(device), " "))
	b := strings.ToLower(strings.Join(strings.Fields(hid), " "))
	return a != "" && b != "" && (strings.Contains(b, a) || strings.Contains(a, b))
}

/*
detail says what happened in the terms it happened in.

A device that can be named is named, because "the keyboard" is what the person
is looking at. One that cannot still gets the sentence: the server is holding a
connection to something that has gone, and that is true whether or not the
hardware left a name behind when it went.
*/
func detail(moved []string) string {
	const cause = "A device was unplugged and plugged back in since the server started. " +
		"It detects hardware once, when it starts, so it is still writing to the device that went."

	if len(moved) == 0 {
		return "the OpenRGB server is holding a connection to a device that is no longer there. " + cause
	}
	return "the OpenRGB server is still addressing " + strings.Join(moved, ", ") +
		" at a connection that has gone. " + cause
}
