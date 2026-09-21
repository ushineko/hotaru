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

	found, err := find(sysRoot, devRoot)
	require.NoError(t, err)
	require.Len(t, found, 1)
	require.Equal(t, "NZXT Kraken Elite V2", found[0].Name)
	require.Equal(t, filepath.Join(devRoot, "hidraw7"), found[0].HID)
	require.Equal(t, filepath.Join(devRoot, "bus", "usb", "001", "013"), found[0].USB)
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

// descriptor writes a report descriptor whose first item is a usage page.
func withDescriptor(t *testing.T, sysRoot, node string, bytes []byte) {
	t.Helper()
	require.NoError(t, os.WriteFile(
		filepath.Join(sysRoot, "class", "hidraw", node, "device", "report_descriptor"), bytes, 0o644))
}

func TestAVendorDefinedInterfaceIsTriedFirst(t *testing.T) {
	/*
		The discriminator HID actually provides. A control protocol lives on a
		vendor-defined usage page; a device's other collections declare
		standard ones, and sysfs offers no other way to tell them apart --
		the glob is lexical, so hidraw10 would otherwise be tried before
		hidraw7.

		An ordering, not a filter: the page says what an interface is *for*,
		and only the device can say which one will answer.
	*/
	sysRoot, devRoot := sysfs(t, "0003:00001E71:00003012", 1, 13)

	// A second node on the same USB device, its own interface, declaring the
	// generic desktop page -- which is how a device with a keyboard
	// collection alongside a control one appears.
	usb := filepath.Join(sysRoot, "devices", "usb1", "1-10")
	other := filepath.Join(usb, "1-10:1.2")
	require.NoError(t, os.MkdirAll(other, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(other, "uevent"),
		[]byte("DRIVER=usbhid\nHID_ID=0003:00001E71:00003012\n"), 0o644))

	second := filepath.Join(sysRoot, "class", "hidraw", "hidraw10")
	require.NoError(t, os.MkdirAll(second, 0o755))
	require.NoError(t, os.Symlink(other, filepath.Join(second, "device")))

	withDescriptor(t, sysRoot, "hidraw7", []byte{0x06, 0x00, 0xFF, 0x09, 0x01}) // vendor defined
	withDescriptor(t, sysRoot, "hidraw10", []byte{0x05, 0x01, 0x09, 0x06})      // generic desktop

	found, err := find(sysRoot, devRoot)
	require.NoError(t, err)
	require.Len(t, found, 2)
	require.Equal(t, filepath.Join(devRoot, "hidraw7"), found[0].HID,
		"the vendor-defined interface was not tried first")
	require.Equal(t, uint16(0xFF00), found[0].UsagePage)
}

func TestADescriptorThatCannotBeReadIsNotADisqualification(t *testing.T) {
	// A device whose descriptor hotaru cannot parse may still answer, and
	// refusing it would be preferring a guess to the device's own reply.
	sysRoot, devRoot := sysfs(t, "0003:00001E71:00003012", 1, 13)
	found, err := find(sysRoot, devRoot)
	require.NoError(t, err)
	require.Len(t, found, 1, "a node with no readable descriptor was dropped")
}
