//go:build linux

package systemd

import (
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

// keychron is the machine this was found on: one keyboard, two interfaces, the
// server driving the second. Exactly the shape that a per-node count gets
// wrong.
var keychron = []hidNode{
	{node: "hidraw15", name: "Keychron Keychron K4 HE", key: "/sys/devices/pci0000:00/usb1/1-5/1-5.2"},
	{node: "hidraw16", name: "Keychron Keychron K4 HE", key: "/sys/devices/pci0000:00/usb1/1-5/1-5.2"},
}

/*
A device is held if the server holds any of its nodes.

The Keychron presents two and OpenRGB drives one. Reporting the other as an
orphan would say a working keyboard had moved, which is the failure mode a
health check cannot afford.
*/
func TestADeviceWithSeveralNodesIsHeldIfAnyIsHeld(t *testing.T) {
	held := ungrouped(keychron, map[string]bool{"/dev/hidraw16": true})
	require.Empty(t, held, "a keyboard the server was driving was reported as an orphan")

	none := ungrouped(keychron, map[string]bool{})
	require.Equal(t, []string{"Keychron Keychron K4 HE"}, none,
		"a device with no descriptor on any node should be reported once, not per node")
}

/*
TestATouchedNodeIsNotAReplug is spec 058 AC5.

The first design compared device-node timestamps against the server's start
time. On this machine, on 25 Sep, the AURA and Kraken node mtimes both moved
with no USB re-attach anywhere in the journal -- a session or ACL event touched
them -- while both devices were working, and hidraw numbers are recycled
besides. That comparison would have called two healthy devices stale.

So the timestamps are not consulted, and this is what says so. The evidence is
the descriptor: a node is removed or it is not.
*/
func TestATouchedNodeIsNotAReplug(t *testing.T) {
	source, err := os.ReadFile("handles_linux.go")
	require.NoError(t, err)

	for _, banned := range []string{"ModTime", "mtime", "Stat(", "ctime"} {
		require.NotContains(t, string(source), banned,
			"a device node's timestamps moved without a replug on the machine this was written for; "+
				"staleness cannot be inferred from them")
	}
}

// A removed node is the evidence, and a live one is not.
func TestOnlyARemovedDescriptorCounts(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(dir+"/uevent", []byte(
		"DRIVER=hid-generic\nHID_ID=0003:00003434:00000E40\nHID_NAME=Keychron Keychron K4 HE\n"), 0o600))
	require.Equal(t, "Keychron Keychron K4 HE", hidName(dir+"/uevent"))
	require.Empty(t, hidName(dir+"/absent"), "a node that cannot be identified is not reasoned about")
}

/*
Three machines, three answers.

A user unit is this user's own. A system unit is root's, and reachable only
where somebody has already granted this user the command without a password --
which is their decision, made before hotaru ever ran. Everything else gets the
command to type, because a daemon that stopped to ask for a password would be
asking at a terminal nobody is watching.
*/
func TestHowAUnitGetsRestartedDependsOnWhoseItIs(t *testing.T) {
	user := Facts{Installed: true, Unit: "openrgb-server.service", User: true}
	system := Facts{Installed: true, Unit: "openrgb.service"}

	t.Run("a user unit needs nothing", func(t *testing.T) {
		privileged, args, ok := restartPlan(user, false)
		require.True(t, ok)
		require.False(t, privileged, "a user unit was escalated for no reason")
		require.Equal(t, []string{"--user", "restart", "openrgb-server.service"}, args)
	})

	t.Run("a system unit with passwordless root is ours to bounce", func(t *testing.T) {
		privileged, args, ok := restartPlan(system, true)
		require.True(t, ok)
		require.True(t, privileged)
		require.Equal(t, []string{"-n", "systemctl", "restart", "openrgb.service"}, args)
		require.Contains(t, args, "-n", "a bounce must never be able to sit on a password prompt")
	})

	t.Run("a system unit without it is advice", func(t *testing.T) {
		_, _, ok := restartPlan(system, false)
		require.False(t, ok, "a unit needing a password was going to be restarted anyway")
		require.Equal(t, "sudo systemctl restart openrgb.service", system.RestartCommand(),
			"the command offered instead has to be the one that works")
	})

	t.Run("nothing installed is nothing to restart", func(t *testing.T) {
		_, _, ok := restartPlan(Facts{}, true)
		require.False(t, ok)
	})
}

// Whatever else changes, the bounce never prompts: -n is not optional.
func TestABounceNeverAsksForAPassword(t *testing.T) {
	source, err := os.ReadFile("bounce_linux.go")
	require.NoError(t, err)
	require.NotContains(t, string(source), "askpass")
	require.Contains(t, string(source), `"-n", "true"`,
		"the probe for passwordless root must itself be unable to prompt")
}

/*
A refusal to read a descriptor is not a report that nothing is wrong.

The bug this is here for was found on the machine, on 29 Sep, with a keyboard
actually replugged and the server actually holding a removed descriptor. From
inside the service's sandbox the directory listed all forty-two entries and
every single readlink came back permission denied -- so the first version saw
no deleted node, no held node, and announced `healthy: 6 of 6` with complete
confidence while the keyboard sat dead.

Listing is not reading. Any mount namespace does this: ProtectSystem,
ProtectHome, PrivateTmp and ProtectControlGroups each produce it on their own,
measured one at a time.
*/
func TestALinkItMayNotReadIsNotAnAnswer(t *testing.T) {
	paths := []string{"/proc/1/fd/0", "/proc/1/fd/1"}

	refused := func(string) (string, error) { return "", os.ErrPermission }
	_, deleted, known := resolve(paths, refused)
	require.False(t, known, "being refused every descriptor was reported as having looked")
	require.False(t, deleted)

	// A descriptor that closes between the listing and the read is ordinary,
	// and must not take the whole answer down with it.
	raced := func(path string) (string, error) {
		if path == "/proc/1/fd/0" {
			return "", os.ErrNotExist
		}
		return "/dev/hidraw9", nil
	}
	open, _, known := resolve(paths, raced)
	require.True(t, known, "one closed descriptor was mistaken for not being allowed to look")
	require.Equal(t, map[string]bool{"/dev/hidraw9": true}, open)
}

// The deleted marker is what the whole state rests on, so it is read exactly.
func TestTheDeletedMarkerIsWhatCounts(t *testing.T) {
	links := map[string]string{
		"a": "/dev/hidraw17 (deleted)",
		"b": "/dev/hidraw9",
		"c": "/run/user/1000/hotaru/hotaru.sock",
		"d": "/dev/hidraw6",
	}
	open, deleted, known := resolve([]string{"a", "b", "c", "d"},
		func(p string) (string, error) { return links[p], nil })

	require.True(t, known)
	require.True(t, deleted, "a removed device node was not noticed")
	require.Equal(t, map[string]bool{"/dev/hidraw9": true, "/dev/hidraw6": true}, open,
		"a deleted node must not count as held, and a socket is not a device")
}
