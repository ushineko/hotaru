package service

import (
	"context"

	"github.com/ushineko/hotaru/internal/devices"
	"github.com/ushineko/hotaru/internal/openrgb"
)

/*
absent is the OpenRGB server when there is none: a server with no devices
(spec 074). It stands under the devices hotaru draws itself, so that they are
listed, written and restored through the same calls as with a server, while a
write to anything else is refused as the server being unreachable.
*/
type absent struct{ addr string }

var _ openrgb.Client = absent{}

func (absent) Devices(context.Context) ([]devices.Device, error) { return nil, nil }

func (a absent) Device(context.Context, string) (devices.Device, error) {
	return devices.Device{}, unreachable(a.addr)
}

func (a absent) SetMode(context.Context, string, string, openrgb.Style) error {
	return unreachable(a.addr)
}

func (a absent) SetFrame(context.Context, string, devices.Frame) error {
	return unreachable(a.addr)
}

func (a absent) ResizeZone(context.Context, string, string, int) error {
	return unreachable(a.addr)
}

func (absent) ProtocolVersion() uint32 { return 0 }

func (absent) Gone() bool { return false }

func (absent) Close() error { return nil }

// serving is whether an OpenRGB server is connected. Health, probe and rescan
// are about the server itself, and ask this rather than whether there is a
// client: with canvases attached there always is one.
func (s *Service) serving() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.client != nil
}
