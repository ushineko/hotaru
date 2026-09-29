/*
Package systemd finds out what this machine could do about a problem.

hotaru does not fix anything here. It looks at what is installed and what is
running, and turns "nothing is happening" into a sentence with a command in it.
Starting a daemon or enabling a user manager at boot is the user's decision,
and a program that made it for them would be the other kind of annoying.

Everything is build-tagged. A machine without systemd loses these suggestions
and nothing else -- the remedies are advice, and advice is optional in a way
that lighting is not.
*/
package systemd

import (
	"context"
	"errors"
)

/*
ErrNotRestartable is the server being one this user cannot bounce: a system
unit, a server started by hand, or a machine without systemd.

Not a failure of the operation so much as the operation not applying. The
caller's move is to offer RestartCommand, which is a sentence somebody can act
on, rather than to report that something went wrong.
*/
var ErrNotRestartable = errors.New("this OpenRGB server is not one hotaru can restart")

// Facts are what could be done about an unreachable OpenRGB server.
type Facts struct {
	// Installed is whether an OpenRGB unit exists at all. Absent means the
	// package is not installed, which is a different problem from a daemon
	// that is not running.
	Installed bool

	// Unit is the unit that exists, which differs by machine: the Arch package
	// ships a system unit, this developer's machine runs a user one, and other
	// people start the server by hand. Naming the one that is there beats
	// guessing.
	Unit string

	// User is whether Unit is a user unit rather than a system one, because
	// the command to start it differs.
	User bool

	// Active is whether it is running.
	Active bool

	// Lingering is whether this user's services start at boot. Without it, the
	// lights come back at login rather than at boot, which is a surprise worth
	// explaining once.
	Lingering bool
}

/*
Remedies are the things a person could do, in the order worth trying.

Each is a sentence and a command. A remedy nobody can act on is not a remedy:
"the OpenRGB server is not running" is an observation, and
"systemctl --user start openrgb-server" is a remedy.
*/
func (f Facts) Remedies() []string {
	var out []string

	switch {
	case !f.Installed:
		out = append(out, "OpenRGB does not appear to be installed. Lighting needs its server running.")
	case !f.Active:
		out = append(out, "The OpenRGB server is installed and not running. Start it with `"+f.startCommand()+"`.")
	}

	if !f.Lingering {
		out = append(out, "Your services do not start until you log in. "+
			"For lighting to come back at boot, enable lingering with `loginctl enable-linger $USER`.")
	}
	return out
}

/*
RestartCommand is what a person would type to bounce the server themselves.

Offered whether or not hotaru can do it: a system unit needs root, and asking
for a password from a lighting daemon is not a thing this program does. See
spec 058 R7.
*/
func (f Facts) RestartCommand() string {
	if f.User {
		return "systemctl --user restart " + f.Unit
	}
	return "sudo systemctl restart " + f.Unit
}

// Restartable is whether hotaru could do it without asking for a password.
// A system unit is somebody's to restart deliberately, not this program's.
func (f Facts) Restartable() bool { return f.Installed && f.User }

func (f Facts) startCommand() string {
	if f.User {
		return "systemctl --user start " + f.Unit
	}
	return "sudo systemctl start " + f.Unit
}

// Look reports what this machine offers. A machine that cannot be asked
// returns zero facts, and the remedies are then silent rather than wrong.
func Look(ctx context.Context) Facts { return look(ctx) }

/*
Bounce restarts the OpenRGB server, and is the one thing in this package that
changes the machine rather than describing it.

The package's rule is unchanged where it was written for: nothing here enables
a unit, turns on lingering, or decides at boot on somebody's behalf. That is
configuration, and it stays theirs. This is a restart asked for by a person
running a command whose whole purpose is to perform one, which is a different
question from whether a program should quietly reconfigure a machine.

Only a user unit. A system one needs root, and RestartCommand says so instead.
*/
func Bounce(ctx context.Context, f Facts) error { return bounce(ctx, f) }

// unit is one OpenRGB unit this machine has, and whether it is running.
type unit struct {
	name   string
	user   bool
	active bool
}

/*
choose picks the unit to talk about.

An active one settles it. Otherwise the **first** in candidate order wins,
because that list is ordered by what is worth trying: a later match must not
overwrite an earlier one.

It did, and the result was a wrong answer in the one message hotaru offers when
lighting is not working. A machine with the user unit installed but stopped,
and the system unit merely present and disabled, was told to
`sudo systemctl start openrgb.service` -- wrong scope, wrong unit, and with a
sudo in front of it.
*/
func choose(seen []unit, lingering bool) Facts {
	facts := Facts{Lingering: lingering}
	for _, u := range seen {
		if u.active {
			return Facts{Installed: true, Unit: u.name, User: u.user, Active: true, Lingering: lingering}
		}
		if !facts.Installed {
			facts.Installed = true
			facts.Unit = u.name
			facts.User = u.user
		}
	}
	return facts
}

/*
Handles are what the running server has open, for telling a device that moved
from one that was never there.

OpenRGB detects hardware once, when it starts. A device unplugged and plugged
back in since then leaves it holding a descriptor for a node the kernel has
removed, while the device itself is on a new node the server knows nothing
about. Neither the device count nor the scope changes, so the listing looks
exactly as it always did and every write goes nowhere. See spec 058.
*/
type Handles struct {
	// Deleted is whether the server holds a descriptor for a device node that
	// has been removed. Exact rather than inferred: a node is deleted or it is
	// not, and this is the fact the state rests on.
	Deleted bool

	// Orphaned are the names of devices present with no descriptor in the
	// server at all. The same fault from the other side, and the only side
	// that can say which device it was -- a deleted node has no name left.
	//
	// A device with several nodes counts as held if the server holds any of
	// them: a keyboard offering two interfaces, with the server driving one,
	// is a working keyboard and not an orphan.
	Orphaned []string

	// Known is whether any of this could be looked at. A server started by
	// hand, or one running as root, cannot be inspected by this user, and
	// saying nothing is the honest answer rather than a guess.
	Known bool
}

/*
Held is what the OpenRGB server on this machine has open.

Not known is the ordinary answer on most machines. The Arch package ships a
system unit, whose descriptors belong to root, and a server somebody started
themselves has no unit to find a PID through. Both return zero facts, and the
health state then says what it has always said.
*/
func Held(ctx context.Context) Handles { return held(ctx) }
