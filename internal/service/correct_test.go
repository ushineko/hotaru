package service_test

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/ushineko/hotaru/internal/colour"
	"github.com/ushineko/hotaru/internal/config"
	"github.com/ushineko/hotaru/internal/devices"
	"github.com/ushineko/hotaru/internal/openrgb"
	"github.com/ushineko/hotaru/internal/service"
)

/*
Spec 066: a device whose lights wash out is written a corrected colour, and
everything above the write still sees the colour that was meant.

The profile is the one that matched the RTX 4090 to the fans in the scene
ice1: #80aad1 is written #003eff.
*/

func washesOut() *config.Config {
	full, green := 1.0, 2.15
	return &config.Config{Devices: []config.DeviceRule{{
		Match:  "graphics",
		Colour: &config.ColourProfile{Saturation: &full, Value: &full, Curve: &config.Curve{Green: &green}},
	}}}
}

// listed is a device's colours as the service lists them.
func listed(t *testing.T, svc *service.Service, name string) devices.Device {
	t.Helper()
	views, err := svc.List(t.Context())
	require.NoError(t, err)
	for _, view := range views {
		if view.Device.Name == name {
			return view.Device
		}
	}
	t.Fatalf("%s is not listed", name)
	return devices.Device{}
}

func TestACorrectedDeviceIsWrittenCorrectedAndReportsWhatWasMeant(t *testing.T) {
	server := openrgb.NewFake(graphics())
	svc := service.New(washesOut(), server, "")
	svc.SetRecorder(recorder(t))

	results, err := svc.Apply(t.Context(), service.Request{Assignments: solid("Graphics", "#80aad1")})
	require.NoError(t, err)
	require.Len(t, results, 1)
	require.True(t, results[0].Applied, results[0].Skipped)
	require.Empty(t, results[0].Unconfirmed, "the corrected write was not recognised on the way back")

	written := server.Writes[len(server.Writes)-1].Frame.Colours
	require.Equal(t, "#003eff", written[0].String(), "the device was not written the corrected colour")

	device := listed(t, svc, graphicsName)
	for _, c := range device.Colours {
		require.Equal(t, "#80aad1", c.String(), "the listing shows what was written, not what was meant")
	}
	require.Equal(t, "#80aad1", svc.Desired().Devices[graphicsName].Colours[0].String(),
		"the record is of what was asked for")
}

func TestAModesOwnColoursAreCorrected(t *testing.T) {
	server := openrgb.NewFake(graphics())
	svc := service.New(washesOut(), server, "")

	results, err := svc.Apply(t.Context(), service.Request{
		Assignments: solid("Graphics", "white"),
		Effects:     twoColours("Breathing", "#80aad1", "#ff0000"),
	})
	require.NoError(t, err)
	require.True(t, results[0].Applied, results[0].Skipped)
	require.Empty(t, results[0].Unconfirmed)

	last := server.Modes[len(server.Modes)-1]
	require.Equal(t, []colour.Colour{colour.MustParse("#003eff"), colour.MustParse("#ff0000")}, last.Colours)

	device := listed(t, svc, graphicsName)
	breathing, ok := device.Mode("Breathing")
	require.True(t, ok)
	require.Equal(t, []colour.Colour{colour.MustParse("#80aad1"), colour.MustParse("#ff0000")}, breathing.Colours)
	require.Equal(t, "#80aad1", breathing.Colour.String())
}

func TestADeviceWithoutAProfileIsWrittenAsAsked(t *testing.T) {
	server := openrgb.NewFake(graphics(), board())
	svc := service.New(washesOut(), server, "")

	_, err := svc.Apply(t.Context(), service.Request{Assignments: solid("ASUS", "#80aad1")})
	require.NoError(t, err)
	for _, w := range server.Writes {
		require.Equal(t, "ASUS ROG MAXIMUS Z790 HERO", w.Device)
		require.Equal(t, "#80aad1", w.Frame.Colours[0].String())
	}
}

func TestALightChangedElsewhereIsReportedAsItIs(t *testing.T) {
	server := openrgb.NewFake(graphics())
	svc := service.New(washesOut(), server, "")
	_, err := svc.Apply(t.Context(), service.Request{Assignments: solid("Graphics", "#80aad1")})
	require.NoError(t, err)

	// Something other than hotaru turns the middle light green.
	corrected := colour.MustParse("#003eff")
	require.NoError(t, server.SetFrame(t.Context(), graphicsName, devices.Frame{
		Device: graphicsName, Colours: []colour.Colour{corrected, colour.MustParse("green"), corrected},
	}))

	got := listed(t, svc, graphicsName).Colours
	require.Equal(t, []string{"#80aad1", "#00ff00", "#80aad1"},
		[]string{got[0].String(), got[1].String(), got[2].String()})
}
