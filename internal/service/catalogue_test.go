package service_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ushineko/hotaru/internal/canvas/canvastest"
	"github.com/ushineko/hotaru/internal/colour"
	"github.com/ushineko/hotaru/internal/devices"
	"github.com/ushineko/hotaru/internal/scenes"
	"github.com/ushineko/hotaru/internal/service"
	"github.com/ushineko/hotaru/internal/service/servicetest"
	"github.com/ushineko/sanshoku/lighting"
)

// tunedCooler is a cooler whose effects take up to three colours and five
// speeds, a period, as the Kraken's catalogue gives them (spec 077 R2).
func tunedCooler() *canvastest.Firmware {
	f := canvastest.NewFirmware(canvastest.NewBoard("Tuned Cooler", canvastest.Zone("Ring", 4)),
		"Fading", "Spectrum Wave", "Off")
	f.Colourless = []string{"Spectrum Wave", "Off"}
	f.MaxColours, f.Speeds = 3, []int{80, 60, 40, 20, 10}
	return f
}

func modeNamed(t *testing.T, v service.View, name string) devices.Mode {
	t.Helper()
	m, ok := v.Device.Mode(name)
	require.True(t, ok, "no mode %q", name)
	return m
}

// A firmware effect is listed with its catalogue's colours and speeds, and a
// colourless one takes none (R2.1).
func TestTheCatalogueIsListed(t *testing.T) {
	svc := servicetest.New(t, nil, tunedCooler())
	v := viewOf(t, svc, "Tuned Cooler")
	fading := modeNamed(t, v, "Fading")
	assert.True(t, fading.Stored)
	assert.Equal(t, [2]int{1, 3}, [2]int{fading.ColoursMin, fading.ColoursMax})
	require.NotNil(t, fading.Speed)
	assert.Equal(t, devices.Speed{Slowest: 80, Fastest: 10, Now: 40}, *fading.Speed)
	wave := modeNamed(t, v, "Spectrum Wave")
	assert.False(t, wave.ModeColour)
	assert.Zero(t, wave.ColoursMax)
}

// A scene's effect reaches the device with all its colours and its speed, and
// the listing reads them back (R2.2, R2.3).
func TestAnEffectsColoursAndSpeedReachTheDevice(t *testing.T) {
	cooler := tunedCooler()
	svc := servicetest.New(t, nil, cooler)
	svc.SetRecorder(recorder(t))
	speed := 20
	red := colour.MustParse("red")
	_, err := svc.Apply(t.Context(), service.Request{
		Colour: &red,
		Effects: map[string]scenes.Effect{"tuned": {
			Mode: "Fading", Colours: []string{"#ff0000", "#00ff00", "#0000ff"}, Speed: &speed,
		}},
	})
	require.NoError(t, err)
	colours, sp := cooler.Settings()
	assert.Equal(t, []lighting.Colour{{R: 255}, {G: 255}, {B: 255}}, colours)
	require.NotNil(t, sp)
	assert.Equal(t, 20, *sp)

	fading := modeNamed(t, viewOf(t, svc, "Tuned Cooler"), "Fading")
	assert.Len(t, fading.Colours, 3)
	assert.Equal(t, 20, fading.Speed.Now)

	kept := svc.Desired().Devices["Tuned Cooler"]
	assert.Equal(t, "Fading", kept.Mode)
	assert.Equal(t, []string{"#ff0000", "#00ff00", "#0000ff"}, kept.ModeColours)
	require.NotNil(t, kept.Speed)
	assert.Equal(t, 20, *kept.Speed)
}

/*
After a restart, an effect held in memory is put back with its colours and
speed, where an effect in storage that wears is not written without a person
(R2.3; sanshoku spec 034).
*/
func TestAnEffectInMemoryIsPutBackAfterARestart(t *testing.T) {
	store := recorder(t)
	first := servicetest.New(t, nil, tunedCooler())
	first.SetRecorder(store)
	red, speed := colour.MustParse("red"), 10
	_, err := first.Apply(t.Context(), service.Request{
		Colour: &red,
		Effects: map[string]scenes.Effect{"tuned": {
			Mode: "Fading", Colours: []string{"#ff0000", "#0000ff"}, Speed: &speed,
		}},
	})
	require.NoError(t, err)

	// The service starts again, and the cooler has lost its effect with its
	// power.
	again := tunedCooler()
	second := servicetest.New(t, nil, again)
	second.SetRecorder(store)
	_, err = second.Reconcile(t.Context(), nil)
	require.NoError(t, err)
	running, _ := again.Running()
	assert.Equal(t, "Fading", running)
	colours, sp := again.Settings()
	assert.Equal(t, []lighting.Colour{{R: 255}, {B: 255}}, colours)
	require.NotNil(t, sp)
	assert.Equal(t, 10, *sp)
}
