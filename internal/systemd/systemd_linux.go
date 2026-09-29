//go:build linux

package systemd

import (
	"context"
	"os"
	"os/exec"
	"strings"
	"time"
)

// askTimeout bounds every question. systemctl on a busy machine can take a
// moment, and a health check that hangs is worse than one that says less.
const askTimeout = 2 * time.Second

/*
candidates are the OpenRGB units seen in the wild, in the order worth trying.

The Arch package ships a system unit. This developer's machine runs a user unit
of its own, carrying the enumeration gate from the old project's specs 026 and
033. Other people start the server by hand, and for them none of this applies.
*/
var candidates = []struct {
	name string
	user bool
}{
	{"openrgb-server.service", true},
	{"openrgb.service", true},
	{"openrgb.service", false},
}

func look(ctx context.Context) Facts {
	facts := Facts{Lingering: lingering(ctx)}

	var seen []unit
	for _, candidate := range candidates {
		state, found := unitState(ctx, candidate.name, candidate.user)
		if !found {
			continue
		}
		seen = append(seen, unit{candidate.name, candidate.user, state == "active"})
	}
	return choose(seen, facts.Lingering)
}

// unitState is a unit's ActiveState, and whether the unit exists at all.
func unitState(ctx context.Context, unit string, user bool) (string, bool) {
	args := []string{"show", unit, "--property=ActiveState,LoadState"}
	if user {
		args = append([]string{"--user"}, args...)
	}
	out, err := systemctl(ctx, args...)
	if err != nil {
		return "", false
	}

	var active, load string
	for _, line := range strings.Split(out, "\n") {
		key, value, ok := strings.Cut(strings.TrimSpace(line), "=")
		if !ok {
			continue
		}
		switch key {
		case "ActiveState":
			active = value
		case "LoadState":
			load = value
		}
	}
	// A unit that was never installed loads as "not-found", and systemctl
	// still exits zero -- so the answer is in the output, not the status.
	return active, load != "" && load != "not-found"
}

func lingering(ctx context.Context) bool {
	user := os.Getenv("USER")
	if user == "" {
		return false
	}
	out, err := loginctl(ctx, "show-user", user, "--property=Linger")
	if err != nil {
		return false
	}
	return strings.TrimSpace(out) == "Linger=yes"
}

/*
systemctl and loginctl are the only programs this package runs.

Written as two functions with literal names rather than one with a parameter,
so there is no path by which a program name could come from anywhere but here.
The arguments are built from this file's own constants and the USER
environment variable, which is read for a name and never for a command.
*/
func systemctl(ctx context.Context, args ...string) (string, error) {
	return systemctlFor(ctx, askTimeout, args...)
}

// systemctlFor is systemctl with a deadline of its own, because asking a
// question and waiting out a restart are not the same length of operation.
func systemctlFor(ctx context.Context, timeout time.Duration, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// G204: the program is a literal and every argument is built from this
	// file's own constants. There is no shell, so an argument is one argv
	// element whatever it contains.
	out, err := exec.CommandContext(ctx, "systemctl", args...).Output() //nolint:gosec
	if err != nil {
		return "", err //nolint:wrapcheck // the caller only asks whether it worked
	}
	return string(out), nil
}

/*
sudoFor runs something as root, and only ever with -n in the arguments the
caller built.

A third function with a literal name, for the reason the other two have one:
there is no path by which a program name could come from anywhere but this
file. What it is asked to run comes from restartPlan, which builds it from a
unit name this package chose from its own candidate list.
*/
func sudoFor(ctx context.Context, timeout time.Duration, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// G204: the program is a literal and every argument is built from this
	// package's own constants. There is no shell, so an argument is one argv
	// element whatever it contains.
	out, err := exec.CommandContext(ctx, "sudo", args...).Output() //nolint:gosec
	if err != nil {
		return "", err //nolint:wrapcheck // the caller only asks whether it worked
	}
	return string(out), nil
}

func loginctl(ctx context.Context, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, askTimeout)
	defer cancel()

	// G204: as above. The one value from outside is $USER, passed as a single
	// argv element to a program that takes a user name there.
	out, err := exec.CommandContext(ctx, "loginctl", args...).Output() //nolint:gosec
	if err != nil {
		return "", err //nolint:wrapcheck // the caller only asks whether it worked
	}
	return string(out), nil
}
