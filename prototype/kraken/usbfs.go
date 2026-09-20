package main

import (
	"fmt"
	"os"
	"unsafe"

	"syscall"
)

// Linux usbfs, enough of it to claim one interface and write to a bulk
// endpoint. No libusb, no cgo: these are ioctls on /dev/bus/usb/BBB/DDD.
const (
	usbdevfsClaimInterface   = 0x8004550f // _IOR('U', 15, unsigned int)
	usbdevfsReleaseInterface = 0x80045510 // _IOR('U', 16, unsigned int)
	usbdevfsBulk             = 0xc0185502 // _IOWR('U', 2, struct usbdevfs_bulktransfer)
)

type bulkTransfer struct {
	ep      uint32
	length  uint32
	timeout uint32
	_       uint32 // padding to align the pointer on 64-bit
	data    uintptr
}

type usbDevice struct{ f *os.File }

func openUSB(path string) (*usbDevice, error) {
	f, err := os.OpenFile(path, os.O_RDWR, 0)
	if err != nil {
		return nil, err
	}
	return &usbDevice{f: f}, nil
}

func (d *usbDevice) ioctl(req uintptr, arg unsafe.Pointer) error {
	_, _, errno := syscall.Syscall(syscall.SYS_IOCTL, d.f.Fd(), req, uintptr(arg))
	if errno != 0 {
		return errno
	}
	return nil
}

func (d *usbDevice) claim(iface uint32) error {
	if err := d.ioctl(usbdevfsClaimInterface, unsafe.Pointer(&iface)); err != nil {
		return fmt.Errorf("claim interface %d: %w", iface, err)
	}
	return nil
}

func (d *usbDevice) release(iface uint32) error {
	return d.ioctl(usbdevfsReleaseInterface, unsafe.Pointer(&iface))
}

// bulkWrite sends one transfer to an OUT endpoint.
func (d *usbDevice) bulkWrite(ep uint8, data []byte, timeoutMS uint32) (int, error) {
	if len(data) == 0 {
		return 0, nil
	}
	bt := bulkTransfer{
		ep:      uint32(ep),
		length:  uint32(len(data)),
		timeout: timeoutMS,
		data:    uintptr(unsafe.Pointer(&data[0])),
	}
	// The ioctl returns the number of bytes actually transferred. Ignoring it
	// is how a short write looks like a successful one.
	n, _, errno := syscall.Syscall(syscall.SYS_IOCTL, d.f.Fd(), usbdevfsBulk, uintptr(unsafe.Pointer(&bt)))
	if errno != 0 {
		return 0, fmt.Errorf("bulk write to ep 0x%02x: %w", ep, errno)
	}
	return int(n), nil
}

func (d *usbDevice) Close() error { return d.f.Close() }
