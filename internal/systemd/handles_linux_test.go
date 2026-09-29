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
