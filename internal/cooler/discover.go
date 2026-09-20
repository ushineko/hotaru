/*
Package cooler reads an NZXT liquid cooler and drives its screen, directly.

No liquidctl. A status reading costs about two milliseconds over
/dev/hidraw against a hundred and five through a Python interpreter, and the
screen's bulk endpoint is reachable through usbfs while usbhid keeps the HID
interface -- see spec 012, which has the measurements and the protocol.
*/
package cooler

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

/*
Known is the hardware this package will drive.

A list rather than a vendor match. The Kraken family differs by product: the
protocol here was read off an Elite V2 on firmware 1.2.0, and a cooler that
merely shares a vendor id is not the same device. hotaru declines what it does
not recognise rather than guessing at somebody's pump.
*/
var Known = map[uint16]string{
	0x3012: "NZXT Kraken Elite V2",
}

// nzxt is the vendor every id above belongs to.
const nzxt = 0x1E71

/*
Device is one cooler, and the two paths needed to talk to it.

Both are found, never configured. The prototype behind this spec hardcoded
/dev/hidraw7, which is right on one machine on one boot and wrong everywhere
else -- hidraw numbers move when anything else is plugged in.
*/
type Device struct {
	Product uint16
	Name    string

	// HID is the character device carrying status and control.
	HID string
	// USB is the usbfs node whose bulk endpoint carries screen data.
	USB string
}

// Find locates a supported cooler, or reports that there is none.
func Find() (Device, error) { return find("/sys", "/dev") }

func find(sysRoot, devRoot string) (Device, error) {
	nodes, err := filepath.Glob(filepath.Join(sysRoot, "class", "hidraw", "hidraw*"))
	if err != nil {
		return Device{}, fmt.Errorf("look for hidraw devices: %w", err)
	}
	for _, node := range nodes {
		vendor, product, ok := hidID(filepath.Join(node, "device", "uevent"))
		if !ok || vendor != nzxt {
			continue
		}
		name, supported := Known[product]
		if !supported {
			continue
		}
		usb, err := usbNode(devRoot, node)
		if err != nil {
			return Device{}, err
		}
		return Device{
			Product: product,
			Name:    name,
			HID:     filepath.Join(devRoot, filepath.Base(node)),
			USB:     usb,
		}, nil
	}
	return Device{}, ErrNoCooler
}

// ErrNoCooler is a machine with no cooler this package knows how to drive,
// which is an ordinary state and not a failure: everything else still works.
var ErrNoCooler = fmt.Errorf("no supported liquid cooler")

// hidID reads HID_ID=0003:00001E71:00003012 from a uevent.
func hidID(path string) (vendor, product uint16, ok bool) {
	b, err := os.ReadFile(path) //nolint:gosec // a path under sysfs, built here
	if err != nil {
		return 0, 0, false
	}
	for line := range strings.SplitSeq(string(b), "\n") {
		rest, found := strings.CutPrefix(line, "HID_ID=")
		if !found {
			continue
		}
		parts := strings.Split(rest, ":")
		if len(parts) != 3 {
			return 0, 0, false
		}
		v, err1 := strconv.ParseUint(parts[1], 16, 16)
		p, err2 := strconv.ParseUint(parts[2], 16, 16)
		if err1 != nil || err2 != nil {
			return 0, 0, false
		}
		return uint16(v), uint16(p), true
	}
	return 0, 0, false
}

/*
usbNode walks up from a hidraw node to the USB device it belongs to, and builds
the usbfs path from its bus and device numbers.

Up rather than across: the same physical cooler appears as a hidraw character
device and as a USB device, and the only reliable link between them is the
sysfs tree that contains both.
*/
func usbNode(devRoot, hidraw string) (string, error) {
	dir, err := filepath.EvalSymlinks(filepath.Join(hidraw, "device"))
	if err != nil {
		return "", fmt.Errorf("resolve %s: %w", hidraw, err)
	}
	for range 8 { // an interface, a device, and room for an unusual topology
		bus, err1 := os.ReadFile(filepath.Join(dir, "busnum")) //nolint:gosec // sysfs
		dev, err2 := os.ReadFile(filepath.Join(dir, "devnum")) //nolint:gosec // sysfs
		if err1 == nil && err2 == nil {
			return filepath.Join(devRoot, "bus", "usb",
				fmt.Sprintf("%03d", atoi(bus)), fmt.Sprintf("%03d", atoi(dev))), nil
		}
		parent := filepath.Dir(dir)
		if parent == dir || parent == "/" {
			break
		}
		dir = parent
	}
	return "", fmt.Errorf("no USB device above %s", hidraw)
}

func atoi(b []byte) int {
	n, _ := strconv.Atoi(strings.TrimSpace(string(b)))
	return n
}
