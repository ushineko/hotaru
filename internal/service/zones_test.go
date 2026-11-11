package service_test

import (
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/ushineko/hotaru/internal/colour"
	"github.com/ushineko/hotaru/internal/devices"
	"github.com/ushineko/hotaru/internal/openrgb"
	"github.com/ushineko/hotaru/internal/service"
	"github.com/ushineko/hotaru/internal/state"
)

const hero = "ASUS ROG MAXIMUS Z790 HERO"

/*
headers is the board as measured for spec 069: one fixed mainboard light and
two addressable headers of 16, each of which can be given 0 to 500.
*/
func headers() devices.Device {
	return devices.Device{
		Name:     hero,
		LEDCount: 33,
		Modes:    []devices.Mode{{Name: "Direct", PerLED: true}},
		Zones: []devices.Zone{
			{Name: "Aura Mainboard", First: 0, Count: 1, Min: 1, Max: 1},
			{Name: "Addressable RGB Header 1", First: 1, Count: 16, Min: 0, Max: 500},
			{Name: "Addressable RGB Header 2", First: 17, Count: 16, Min: 0, Max: 500},
		},
		ActiveMode: "Direct",
	}
}

// emptied is what OpenRGB 1.1 came back with: both headers at 0.
func emptied(t *testing.T, server *openrgb.Fake) {
	t.Helper()
	for _, zone := range []string{"Addressable RGB Header 1", "Addressable RGB Header 2"} {
		require.NoError(t, server.ResizeZone(t.Context(), hero, zone, 0))
	}
	server.Resizes = nil
}

func TestASceneRecordsTheSizeOfEveryZone(t *testing.T) {
	// AC3.
	server := openrgb.NewFake(headers())
	svc := service.New(nil, server, "")
	svc.SetRecorder(recorder(t))

	_, err := svc.Apply(t.Context(), service.Request{Assignments: solid(hero, "teal")})
	require.NoError(t, err)

	require.Equal(t, map[string]int{
		"Aura Mainboard": 1, "Addressable RGB Header 1": 16, "Addressable RGB Header 2": 16,
	}, svc.Desired().Devices[hero].Zones)
}

func TestARestoreGivesEmptiedHeadersBackTheirSize(t *testing.T) {
	// AC4: the board after the OpenRGB 1.1 upgrade.
	server := openrgb.NewFake(headers())
	svc := service.New(nil, server, "")
	svc.SetRecorder(recorder(t))
	_, err := svc.Apply(t.Context(), service.Request{Assignments: append(
		solid(hero, "teal"), solid(hero+"/Addressable RGB Header 2[15]", "red")...)})
	require.NoError(t, err)
	wanted := svc.Desired().Devices[hero].Colours

	emptied(t, server)

	restore, err := svc.Reconcile(t.Context(), nil)
	require.NoError(t, err)
	require.Equal(t, 1, restore.Applied)
	require.Equal(t, []openrgb.Resize{
		{Device: hero, Zone: "Addressable RGB Header 1", Size: 16},
		{Device: hero, Zone: "Addressable RGB Header 2", Size: 16},
	}, server.Resizes)
	require.Equal(t, []string{
		"Addressable RGB Header 1 from 0 to 16 LEDs",
		"Addressable RGB Header 2 from 0 to 16 LEDs",
	}, restore.Results[0].Resized)
	require.Empty(t, restore.Results[0].Problems)

	showing, _ := server.Showing(hero)
	require.Equal(t, wanted, showing.Colours, "the whole recorded frame, the last header LED included")
}

func TestARestoreLeavesAloneWhatItCannotOrShouldNotResize(t *testing.T) {
	// AC5 and AC6.
	cases := map[string]struct {
		zones map[string]int
		setup func(t *testing.T, server *openrgb.Fake)
	}{
		"a zone that is not resizable": {
			zones: map[string]int{"Aura Mainboard": 2},
		},
		"a recorded size past the zone's maximum": {
			zones: map[string]int{"Addressable RGB Header 1": 501},
		},
		"a zone larger than recorded": {
			zones: map[string]int{"Addressable RGB Header 1": 8},
		},
		"a state file from before zone sizes": {
			zones: nil,
			setup: emptied,
		},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			server := openrgb.NewFake(headers())
			if c.setup != nil {
				c.setup(t, server)
			}
			store := recorder(t)
			require.NoError(t, store.Record(hero, state.Device{
				Mode:    "Direct",
				Colours: make([]colour.Colour, 34),
				Zones:   c.zones,
			}))
			svc := service.New(nil, server, "")
			svc.SetRecorder(store)

			restore, err := svc.Reconcile(t.Context(), nil)
			require.NoError(t, err)
			require.Empty(t, server.Resizes)
			require.Empty(t, restore.Results[0].Resized)

			device, _ := server.Device(t.Context(), hero)
			require.Contains(t, strings.Join(restore.Results[0].Problems, "\n"),
				hero+" has an LED count of "+strconv.Itoa(device.LEDCount)+" and its recorded colours are for 34",
				"a frame that does not fit is said")
		})
	}
}

func TestHealthNamesADeviceItsColoursNoLongerFit(t *testing.T) {
	// AC7.
	server := openrgb.NewFake(headers())
	svc := service.New(nil, server, "")
	svc.SetRecorder(recorder(t))
	_, err := svc.Apply(t.Context(), service.Request{Assignments: solid(hero, "teal")})
	require.NoError(t, err)
	require.Empty(t, svc.Health(t.Context()).Remedies, "nothing to say while the zones match")

	emptied(t, server)

	health := svc.Health(t.Context())
	require.Equal(t, service.StateHealthy, health.State)
	require.Contains(t, health.Detail, hero+" has an LED count of 1 and its recorded colours are for 33")
	require.Contains(t, strings.Join(health.Remedies, "\n"), "hotaru reconcile")
}

func TestASceneDoesNotResizeAZone(t *testing.T) {
	// AC8: a zone made smaller in OpenRGB on purpose stays that size, and a
	// target past its end is a problem rather than a resize.
	server := openrgb.NewFake(headers())
	svc := service.New(nil, server, "")
	svc.SetRecorder(recorder(t))
	_, err := svc.Apply(t.Context(), service.Request{Assignments: solid(hero, "teal")})
	require.NoError(t, err)

	require.NoError(t, server.ResizeZone(t.Context(), hero, "Addressable RGB Header 1", 8))
	server.Resizes = nil

	results, err := svc.Apply(t.Context(), service.Request{Assignments: append(
		solid(hero, "teal"), solid(hero+"/Addressable RGB Header 1[15]", "red")...)})
	require.NoError(t, err)
	require.Empty(t, server.Resizes)
	require.True(t, results[0].Applied)
	require.NotEmpty(t, results[0].Problems)
	require.Equal(t, 8, svc.Desired().Devices[hero].Zones["Addressable RGB Header 1"])
}
