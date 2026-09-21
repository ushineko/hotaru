package cooler

import (
	"fmt"
	"os"
	"syscall"
	"unsafe"
)

/*
Enough of Linux usbfs to claim one interface and write to a bulk endpoint.

No libusb and no cgo: these are ioctls on /dev/bus/usb/BBB/DDD, and the whole
of what hotaru needs is three of them. The screen's data does not go over HID
-- that interface carries control only -- so this is the other half of talking
to the cooler.

The interface it claims has no kernel driver bound, so nothing is detached and
usbhid keeps the HID interface hotaru is already reading. Both handles are open
at once, in one process, which is what makes a dashboard possible at all.
*/
const (
	claimInterface   = 0x8004550F // _IOR('U', 15, unsigned int)
	releaseInterface = 0x80045510 // _IOR('U', 16, unsigned int)
	bulkTransfer     = 0xC0185502 // _IOWR('U', 2, struct usbdevfs_bulktransfer)
)

/*
chunk is how much of an image goes in one bulk transfer.

Not a limit on what can be sent -- the panel holds about 24 MB and an animation
happily fills it -- but on what one ioctl is asked to carry. usbfs allocates a
single contiguous buffer per transfer, so a 19 MB write is a 19 MB kernel
allocation; splitting it costs nothing on a stream endpoint and the device has
already been told how many packets to expect.

A multiple of the 1024-byte packet the protocol counts in, so a chunk boundary
is never inside a packet.

Found by a scene: four of this desk's nine animations are between 9 and 20 MB,
and every one of them failed on a cap that only ever saw a 22 KB dashboard
frame.
*/
const chunk = 1 << 20

// bulk is struct usbdevfs_bulktransfer, laid out as the kernel expects.
type bulk struct {
	endpoint uint32
	length   uint32
	timeout  uint32
	_        uint32 // padding, so the pointer lands on an eight-byte boundary
	data     uintptr
}

// usb is a claimed USB interface.
type usb struct {
	f     *os.File
	iface uint32
}

/*
openUSB claims an interface on a USB device.

The interface number is the vendor-defined one carrying the bulk endpoint --
interface 0 on this cooler, where the HID control interface is 1. Claiming is
what stops two programs writing image data at once; nothing detaches a driver,
because none is bound to it.
*/
func openUSB(path string, iface uint32) (*usb, error) {
	f, err := os.OpenFile(path, os.O_RDWR, 0) //nolint:gosec // a path from discovery
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", path, err)
	}
	u := &usb{f: f, iface: iface}
	//nolint:gosec // unsafe is how an ioctl argument is passed; iface is local
	if err := u.ioctl(claimInterface, unsafe.Pointer(&iface)); err != nil {
		_ = f.Close()
		return nil, fmt.Errorf("claim interface %d of %s: %w", iface, path, err)
	}
	return u, nil
}

// G103: unsafe is how an ioctl argument is passed, and every pointer here is
// to a local struct or slice that outlives the call.
//
//nolint:gosec // see above
func (u *usb) ioctl(request uintptr, arg unsafe.Pointer) error {
	if _, _, errno := syscall.Syscall(syscall.SYS_IOCTL, u.f.Fd(), request, uintptr(arg)); errno != 0 {
		return errno
	}
	return nil
}

/*
write sends one transfer to an OUT endpoint.

The count the kernel reports is checked rather than assumed. A short write
means the device received part of an image and will draw whatever was already
in that memory for the rest, which looks like a corrupted panel and reports as
a success.
*/
func (u *usb) write(endpoint uint8, data []byte, timeoutMS uint32) error {
	for len(data) > chunk {
		if err := u.transfer(endpoint, data[:chunk], timeoutMS); err != nil {
			return err
		}
		data = data[chunk:]
	}
	return u.transfer(endpoint, data, timeoutMS)
}

// transfer is one bulk write, which the kernel does in one allocation.
//
// by the chunk above.
//
//nolint:gosec // unsafe is the ioctl calling convention; the length is bounded
func (u *usb) transfer(endpoint uint8, data []byte, timeoutMS uint32) error {
	if len(data) == 0 {
		return nil
	}
	transfer := bulk{
		endpoint: uint32(endpoint),
		length:   uint32(len(data)),
		timeout:  timeoutMS,
		data:     uintptr(unsafe.Pointer(&data[0])),
	}
	sent, _, errno := syscall.Syscall(syscall.SYS_IOCTL, u.f.Fd(), bulkTransfer,
		uintptr(unsafe.Pointer(&transfer)))
	if errno != 0 {
		return fmt.Errorf("write %d bytes to endpoint %#02x: %w", len(data), endpoint, errno)
	}
	if int(sent) != len(data) {
		return fmt.Errorf("short write to endpoint %#02x: sent %d of %d", endpoint, sent, len(data))
	}
	return nil
}

// Close releases the interface and the device.
func (u *usb) Close() error {
	iface := u.iface
	//nolint:gosec // as above
	_ = u.ioctl(releaseInterface, unsafe.Pointer(&iface))
	if err := u.f.Close(); err != nil {
		return fmt.Errorf("close the cooler's USB interface: %w", err)
	}
	return nil
}
