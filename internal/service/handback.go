package service

import (
	"context"
	"strings"
)

/*
The cooler's own lights, after its screen changes hands.

The Kraken's firmware can turn one ring LED green some seconds after the
screen goes to its own readout, and the next frame puts it right. A frame
sent 0.3 s after the change did not stop it; one sent 12 s after did (hotaru
#24, measured with someone watching). The LED is the cooler's and the frame
is OpenRGB's, and nothing reports the change, so the recorded frame is sent
once more after the firmware has had time to finish.

The same LED stayed green after a reboot, when the service handed the screen
back as it stopped and restored the lights soon after it started again. That
case is inferred to be the same one and has not been measured.
*/

// Handbacks receives each time the screen is asked for the cooler's own
// readout. One pending signal is kept; more than one says nothing more.
func (s *Service) Handbacks() <-chan struct{} { return s.handback }

func (s *Service) handedBack() {
	select {
	case s.handback <- struct{}{}:
	default:
	}
}

/*
Retouch sends the cooler's own lights their recorded frame again.

The cooler's lights are the OpenRGB devices on the cooler's own node: the
Kraken's ring is listed at "HID: /dev/hidrawN", the node the cooler is read
at. With no cooler, none of them on the server, or nothing recorded for them,
nothing is written.
*/
func (s *Service) Retouch(ctx context.Context) (Restore, error) {
	s.mu.RLock()
	c := s.cooler
	s.mu.RUnlock()
	if c == nil {
		return Restore{}, nil
	}
	node := c.Device().HID
	if node == "" {
		return Restore{}, nil
	}
	_, client, addr := s.current()
	if client == nil {
		return Restore{}, unreachable(addr)
	}
	found, err := client.Devices(ctx)
	if err != nil {
		return Restore{}, err
	}
	recorded := s.Desired().Devices
	var names []string
	for _, device := range found {
		if _, kept := recorded[device.Name]; kept && onNode(device.Location, node) {
			names = append(names, device.Name)
		}
	}
	if len(names) == 0 {
		return Restore{}, nil
	}
	return s.Reconcile(ctx, names)
}

// onNode reports whether an OpenRGB location ("HID: /dev/hidraw7") is the
// node a sanshoku device was opened at ("/dev/hidraw7").
func onNode(location, node string) bool {
	where, ok := strings.CutPrefix(location, "HID:")
	return ok && strings.TrimSpace(where) == node
}
