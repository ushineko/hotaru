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
	"sort"
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

	// UsagePage is what the HID report descriptor declares this interface is
	// for. Vendor-defined pages carry control protocols; a device's other
	// interfaces -- a keyboard collection, a consumer-control one -- declare
	// standard pages and are not what hotaru wants to talk to.
	UsagePage uint16
}

/*
Find lists every node that might be a supported cooler.

Candidates, not an answer. One device commonly exposes several hidraw nodes --
on the development machine a Logitech receiver has three, a keyboard two, and
this cooler had two earlier the same day -- and they are indistinguishable from
sysfs. Worse, the glob is lexical, so hidraw10 sorts before hidraw7 and "the
first match" is a coin toss that lands differently after a reboot.

Which one actually answers is settled by asking it. See pick.
*/
func Find() ([]Device, error) { return find("/sys", "/dev") }

func find(sysRoot, devRoot string) ([]Device, error) {
	nodes, err := filepath.Glob(filepath.Join(sysRoot, "class", "hidraw", "hidraw*"))
	if err != nil {
		return nil, fmt.Errorf("look for hidraw devices: %w", err)
	}
	var found []Device
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
			// A node whose USB device cannot be found is not a candidate, and
			// not a reason to abandon the others: sysfs is not guaranteed to
			// look the way this machine's does.
			continue
		}
		found = append(found, Device{
			Product:   product,
			Name:      name,
			HID:       filepath.Join(devRoot, filepath.Base(node)),
			USB:       usb,
			UsagePage: usagePage(filepath.Join(node, "device", "report_descriptor")),
		})
	}
	if len(found) == 0 {
		return nil, ErrNoCooler
	}
	/*
		Vendor-defined interfaces first.

		This is the discriminator HID actually provides, and the one hidapi
		exposes as usage_page: a control protocol lives on a vendor-defined
		page, while a device's other collections declare standard ones. It is
		an ordering rather than a filter, because it says what an interface is
		*for* and not whether this particular firmware will answer on it --
		which only the device can say.
	*/
	sort.SliceStable(found, func(i, j int) bool {
		return vendorDefined(found[i].UsagePage) && !vendorDefined(found[j].UsagePage)
	})
	return found, nil
}

// vendorDefined is the usage page range reserved for whatever a vendor likes,
// which is where control protocols are found.
func vendorDefined(page uint16) bool { return page >= 0xFF00 }

/*
usagePage reads the first Usage Page item from a HID report descriptor.

The descriptor is a byte stream of items: 0x06 introduces a two-byte usage
page, 0x05 a one-byte one, and the first of them describes the collection this
interface exposes. Anything unreadable reports zero, which sorts last rather
than failing -- a descriptor hotaru cannot parse is not a reason to refuse a
device that would have answered.
*/
func usagePage(path string) uint16 {
	b, err := os.ReadFile(path) //nolint:gosec // a path under sysfs, built here
	if err != nil || len(b) < 2 {
		return 0
	}
	switch b[0] {
	case 0x06:
		if len(b) >= 3 {
			return uint16(b[1]) | uint16(b[2])<<8
		}
	case 0x05:
		return uint16(b[1])
	}
	return 0
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
