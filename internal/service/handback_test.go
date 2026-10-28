package service_test

import (
	"testing"

	"github.com/stretchr/testify/require"
	kraken "github.com/ushineko/hotaru/internal/cooler"
	"github.com/ushineko/hotaru/internal/cooler/coolertest"
	"github.com/ushineko/hotaru/internal/devices"
	"github.com/ushineko/hotaru/internal/openrgb"
	"github.com/ushineko/hotaru/internal/service"
)

const ring = "NZXT Kraken Ring"

/*
handbackDesk is the cooler's ring, listed by OpenRGB on the node the cooler is read
at, and a strip on another node.
*/
func handbackDesk() *openrgb.Fake {
	lit := func(name, node string) devices.Device {
		return devices.Device{
			Name: name, Location: "HID: " + node, LEDCount: 2,
			Modes:      []devices.Mode{{Name: "Direct", PerLED: true}},
			Zones:      []devices.Zone{{Name: "Ring", First: 0, Count: 2}},
			ActiveMode: "Direct",
		}
	}
	return openrgb.NewFake(lit(ring, "/dev/hidraw-fake"), lit("Generic Strip", "/dev/hidraw-other"))
}

func TestRetouchSendsTheCoolersLightsTheirRecordedFrame(t *testing.T) {
	// AC2: the device on the cooler's node, and only it, gets its frame again.
	server := handbackDesk()
	svc := service.New(nil, server, "")
	svc.SetRecorder(recorder(t))
	svc.SetCooler(kraken.New(coolertest.New()))
	_, err := svc.Apply(t.Context(), service.Request{Assignments: append(
		solid(ring, "teal"), solid("Generic Strip", "red")...)})
	require.NoError(t, err)
	server.Writes = nil

	restore, err := svc.Retouch(t.Context())
	require.NoError(t, err)

	require.Equal(t, 1, restore.Applied)
	require.Len(t, server.Writes, 1)
	require.Equal(t, ring, server.Writes[0].Device)
	require.Equal(t, svc.Desired().Devices[ring].Frame(ring), server.Writes[0].Frame)
}

func TestRetouchWritesNothingItWasNotAskedFor(t *testing.T) {
	// AC3: the inert rule. Nothing recorded for the ring, or no cooler, is
	// no write.
	server := handbackDesk()
	svc := service.New(nil, server, "")
	svc.SetRecorder(recorder(t))
	_, err := svc.Apply(t.Context(), service.Request{Assignments: solid("Generic Strip", "red")})
	require.NoError(t, err)
	server.Writes = nil

	_, err = svc.Retouch(t.Context())
	require.NoError(t, err)
	require.Empty(t, server.Writes, "no cooler, and a device was written to")

	svc.SetCooler(kraken.New(coolertest.New()))
	_, err = svc.Retouch(t.Context())
	require.NoError(t, err)
	require.Empty(t, server.Writes, "the ring was never asked for, and was written to")
}

func TestAskingForTheReadoutSaysTheScreenWasHandedBack(t *testing.T) {
	// AC4: the switch the firmware answers with the green LED is signalled.
	svc := service.New(nil, handbackDesk(), "")
	svc.SetCooler(kraken.New(coolertest.NewPanel()))

	require.NoError(t, svc.Draw(t.Context(), service.Screen{Readout: true}))

	select {
	case <-svc.Handbacks():
	default:
		t.Fatal("asking for the readout did not signal a handback")
	}
}
