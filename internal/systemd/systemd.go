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

import "context"

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

func (f Facts) startCommand() string {
	if f.User {
		return "systemctl --user start " + f.Unit
	}
	return "sudo systemctl start " + f.Unit
}

// Look reports what this machine offers. A machine that cannot be asked
// returns zero facts, and the remedies are then silent rather than wrong.
func Look(ctx context.Context) Facts { return look(ctx) }

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
