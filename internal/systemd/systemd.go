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
unit on a machine where root would ask for a password, a server started by
hand, or a machine without systemd.

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

Offered whether or not hotaru can do it. Where it can -- a user unit, or a
system one this user already holds passwordless root over -- it does it and
this is never seen. Where it cannot, this is the whole of the help available,
because a lighting daemon that stopped to ask for a password would be asking
at a terminal nobody is looking at. See spec 058 R7.
*/
func (f Facts) RestartCommand() string {
	if f.User {
		return "systemctl --user restart " + f.Unit
	}
	return "sudo systemctl restart " + f.Unit
}

/*
restartPlan is how this unit would be bounced, and whether it can be.

Three machines, three answers. A user unit is this user's own and needs nothing.
A system unit needs root, which is reachable only where somebody has already
decided it should be: `sudo -n` succeeds when this user has been granted the
command without a password, and fails instantly rather than prompting when they
have not. Anything else cannot be restarted from here, and gets the command
instead.

Pure, and separate from running it, because which of the three a machine is is
the part worth being sure about.
*/
func restartPlan(f Facts, sudo bool) (privileged bool, args []string, ok bool) {
	switch {
	case !f.Installed:
		return false, nil, false
	case f.User:
		return false, []string{"--user", "restart", f.Unit}, true
	case sudo:
		// -n again at the point of use: this must never be the thing that
		// blocks a lighting daemon on a password prompt nobody can see.
		return true, []string{"-n", "systemctl", "restart", f.Unit}, true
	default:
		return false, nil, false
	}
}

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

Not known is the ordinary answer on plenty of machines. A system unit's
descriptors belong to root; a server somebody started by hand has no unit to
find a PID through; and a caller inside a mount namespace is refused every
link it tries to read. All of them return zero facts, and the caller then says
what it would have said before any of this existed.

Asking costs several subprocesses, because finding the unit and its PID is a
question for systemd. A caller doing this on a timer should use ServerPID once
and HeldBy after that.
*/
func Held(ctx context.Context) Handles {
	pid, ok := ServerPID(ctx)
	if !ok {
		return Handles{}
	}
	return HeldBy(pid)
}

/*
ServerPID is the process the OpenRGB server is running as.

The expensive half: it asks systemd which unit this machine has and what it is
running as, which is four or five subprocesses. The answer changes only when
the server restarts.
*/
func ServerPID(ctx context.Context) (int, bool) { return serverPID(ctx) }

/*
HeldBy is what one process has open, by PID.

The cheap half, and the one worth repeating: /proc only, no subprocesses, so a
window polling every couple of seconds can afford to ask. A PID that is not an
OpenRGB server -- gone, or reused by something else since it was looked up --
is not known rather than wrong.
*/
func HeldBy(pid int) Handles { return heldBy(pid) }
