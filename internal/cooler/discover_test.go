package cooler

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

// sysfs builds enough of a /sys tree to be found in, so discovery is tested
// without hardware and without a machine that happens to have a cooler.
func sysfs(t *testing.T, hidID string, bus, dev int) (sysRoot, devRoot string) {
	t.Helper()
	sysRoot, devRoot = filepath.Join(t.TempDir(), "sys"), filepath.Join(t.TempDir(), "dev")

	usb := filepath.Join(sysRoot, "devices", "usb1", "1-10")
	require.NoError(t, os.MkdirAll(usb, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(usb, "busnum"), []byte(itoa(bus)+"\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(usb, "devnum"), []byte(itoa(dev)+"\n"), 0o644))

	iface := filepath.Join(usb, "1-10:1.1")
	require.NoError(t, os.MkdirAll(iface, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(iface, "uevent"),
		[]byte("DRIVER=usbhid\nHID_ID="+hidID+"\n"), 0o644))

	node := filepath.Join(sysRoot, "class", "hidraw", "hidraw7")
	require.NoError(t, os.MkdirAll(node, 0o755))
	require.NoError(t, os.Symlink(iface, filepath.Join(node, "device")))
	return sysRoot, devRoot
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var out []byte
	for ; n > 0; n /= 10 {
		out = append([]byte{byte('0' + n%10)}, out...)
	}
	return string(out)
}

func TestACoolerIsFoundByItsIdsRatherThanItsPath(t *testing.T) {
	/*
		The prototype behind spec 012 hardcoded /dev/hidraw7. That is right on
		one machine on one boot: hidraw numbers move when anything else is
		plugged in, and they are different on every other machine.
	*/
	sysRoot, devRoot := sysfs(t, "0003:00001E71:00003012", 1, 13)

	device, err := find(sysRoot, devRoot)
	require.NoError(t, err)
	require.Equal(t, "NZXT Kraken Elite V2", device.Name)
	require.Equal(t, filepath.Join(devRoot, "hidraw7"), device.HID)
	require.Equal(t, filepath.Join(devRoot, "bus", "usb", "001", "013"), device.USB)
}

func TestACoolerNobodyHasWrittenDownIsDeclined(t *testing.T) {
	/*
		The protocol here was read off one product on one firmware. A cooler
		that merely shares NZXT's vendor id is a different device, and guessing
		at somebody's pump is worse than saying so.
	*/
	sysRoot, devRoot := sysfs(t, "0003:00001E71:00002007", 1, 13) // a Kraken X53
	_, err := find(sysRoot, devRoot)
	require.ErrorIs(t, err, ErrNoCooler)
}

func TestAMachineWithNoCoolerIsNotAFailure(t *testing.T) {
	// Every other capability still works; this one is simply absent.
	sysRoot, devRoot := sysfs(t, "0003:0000046D:0000C547", 1, 4) // a Logitech receiver
	_, err := find(sysRoot, devRoot)
	require.ErrorIs(t, err, ErrNoCooler)
}

func TestAHidrawWithNothingAboveItIsSkipped(t *testing.T) {
	// Sysfs is not guaranteed to look the way this machine's does.
	sysRoot := filepath.Join(t.TempDir(), "sys")
	node := filepath.Join(sysRoot, "class", "hidraw", "hidraw0")
	require.NoError(t, os.MkdirAll(node, 0o755))

	_, err := find(sysRoot, t.TempDir())
	require.ErrorIs(t, err, ErrNoCooler)
}
