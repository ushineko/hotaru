package service_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ushineko/hotaru/internal/canvas/canvastest"
	"github.com/ushineko/hotaru/internal/colour"
	"github.com/ushineko/hotaru/internal/config"
	"github.com/ushineko/hotaru/internal/devices"
	"github.com/ushineko/hotaru/internal/render"
	"github.com/ushineko/hotaru/internal/scenes"
	"github.com/ushineko/hotaru/internal/service"
	"github.com/ushineko/hotaru/internal/service/servicetest"
	"github.com/ushineko/sanshoku/lighting"
)

// leaderCooler is a cooler running the Kraken's effects in its firmware, at
// the Kraken's speeds.
func leaderCooler() *canvastest.Firmware {
	f := canvastest.NewFirmware(canvastest.NewBoard("NZXT Kraken Elite V2", canvastest.Ring("Ring", 24)),
		"Fading", "Spectrum Wave", "Tai Chi")
	f.Colourless = []string{"Spectrum Wave"}
	f.MaxColours, f.Speeds = 3, []int{350, 300, 250, 150, 80}
	return f
}

// krakenWithBreathing is the leader with Breathing of its own firmware, as
// the Kraken has.
func krakenWithBreathing() *canvastest.Firmware {
	f := canvastest.NewFirmware(canvastest.NewBoard("NZXT Kraken Elite V2", canvastest.Ring("Ring", 24)),
		"Fading", "Spectrum Wave", "Tai Chi", "Breathing")
	f.MaxColours, f.Speeds = 3, []int{350, 300, 250, 150, 80}
	return f
}

// msiCard is a graphics card with effects of its own, none of them the Kraken's
// but Breathing.
func msiCard() *canvastest.Firmware {
	f := canvastest.NewFirmware(canvastest.NewBoard("MSI GeForce RTX 4090", canvastest.Zone("GPU", 1)),
		"Breathing", "Color Cycle", "Rainbow Wave", "Fade In", "Flowing")
	f.Colourless = []string{"Rainbow Wave"}
	f.MaxColours, f.Speeds = 3, []int{4, 2, 1}
	return f
}

/*
A follower takes its leader's effect, colours and speed when the leader is
given one; a device with no follow rule is untouched; and the leader is
written first (spec 077 R3.2, R3.5).
*/
func TestAFollowerTakesTheLeadersEffect(t *testing.T) {
	cooler, gpu := leaderCooler(), msiCard()
	board := canvastest.NewBoard("ASUS ROG MAXIMUS Z790 HERO", canvastest.Zone("Addressable 1", 4))
	mat := canvastest.NewBoard("Corsair MM700", canvastest.Zone("Logo", 1))
	cfg := &config.Config{Devices: []config.DeviceRule{
		{Match: "maximus", Follow: "kraken"},
		{Match: "msi", Follow: "kraken"},
	}}
	svc := servicetest.New(t, cfg, board, gpu, mat, cooler)
	red := colour.MustParse("red")
	speed := 150
	results, err := svc.Apply(t.Context(), service.Request{
		Colour: &red,
		Effects: map[string]scenes.Effect{"kraken": {
			Mode: "Tai Chi", Colours: []string{"#ff0000", "#0000ff"}, Speed: &speed,
		}},
	})
	require.NoError(t, err)
	require.Equal(t, "NZXT Kraken Elite V2", results[0].Device, "the leader was not written first")

	aura := viewOf(t, svc, "ASUS ROG MAXIMUS Z790 HERO").Device
	assert.Equal(t, "Tai Chi", aura.ActiveMode, "the board did not follow")
	taiChi, _ := aura.Mode("Tai Chi")
	assert.Equal(t, []colour.Colour{red, colour.MustParse("blue")}, taiChi.Colours)
	want, _ := render.SpeedFor("Tai Chi", 150)
	require.NotNil(t, taiChi.Speed)
	assert.Equal(t, want, taiChi.Speed.Now, "the speed was not the leader's, translated")

	running, _ := gpu.Running()
	assert.Equal(t, "Fade In", running, "the card took no best fit")
	assert.NotEqual(t, "Tai Chi", viewOf(t, svc, "Corsair MM700").Device.ActiveMode, "a device with no follow rule followed")
}

// A scene that names an effect for a follower overrides the leader's for it.
func TestAFollowersOwnEffectWins(t *testing.T) {
	cooler := leaderCooler()
	board := canvastest.NewBoard("ASUS ROG MAXIMUS Z790 HERO", canvastest.Zone("Addressable 1", 4))
	cfg := &config.Config{Devices: []config.DeviceRule{{Match: "maximus", Follow: "kraken"}}}
	svc := servicetest.New(t, cfg, board, cooler)
	red := colour.MustParse("red")
	_, err := svc.Apply(t.Context(), service.Request{
		Colour: &red,
		Effects: map[string]scenes.Effect{
			"kraken":  {Mode: "Tai Chi"},
			"maximus": {Mode: "Spectrum"},
		},
	})
	require.NoError(t, err)
	assert.Equal(t, "Spectrum", viewOf(t, svc, "ASUS ROG MAXIMUS Z790 HERO").Device.ActiveMode)
}

// Fading on the leader is Color Cycle on a card that has no Fading, with the
// colours it takes (spec 077 R3.6).
func TestABestFitTakesTheColours(t *testing.T) {
	cooler, gpu := leaderCooler(), msiCard()
	cfg := &config.Config{Devices: []config.DeviceRule{{Match: "msi", Follow: "kraken"}}}
	svc := servicetest.New(t, cfg, gpu, cooler)
	red := colour.MustParse("red")
	_, err := svc.Apply(t.Context(), service.Request{
		Colour:  &red,
		Effects: map[string]scenes.Effect{"kraken": {Mode: "Fading", Colours: []string{"#ff0000", "#00ff00", "#0000ff"}}},
	})
	require.NoError(t, err)
	running, _ := gpu.Running()
	assert.Equal(t, "Color Cycle", running)
	colours, _ := gpu.Settings()
	assert.Equal(t, []lighting.Colour{{R: 255}, {G: 255}, {B: 255}}, colours)
}

// A follower of a follower takes the first leader's effect, and a loop of
// follows takes nothing (spec 077 R3.2).
func TestAChainFollowsAndALoopDoesNot(t *testing.T) {
	cooler := leaderCooler()
	board := canvastest.NewBoard("ASUS ROG MAXIMUS Z790 HERO", canvastest.Zone("Addressable 1", 4))
	mat := canvastest.NewBoard("Corsair MM700", canvastest.Zone("Logo", 1))
	pad := canvastest.NewBoard("Other Pad", canvastest.Zone("Logo", 2))
	other := canvastest.NewBoard("Other Strip", canvastest.Zone("Strip", 2))
	cfg := &config.Config{Devices: []config.DeviceRule{
		{Match: "corsair", Follow: "maximus"}, {Match: "maximus", Follow: "kraken"},
		{Match: "other pad", Follow: "other strip"}, {Match: "other strip", Follow: "other pad"},
	}}
	svc := servicetest.New(t, cfg, mat, board, pad, other, cooler)
	red := colour.MustParse("red")
	_, err := svc.Apply(t.Context(), service.Request{
		Colour: &red, Effects: map[string]scenes.Effect{"kraken": {Mode: "Spectrum Wave"}},
	})
	require.NoError(t, err)
	assert.Equal(t, "Spectrum Wave", viewOf(t, svc, "Corsair MM700").Device.ActiveMode, "the chain stopped")
	assert.NotEqual(t, "Spectrum Wave", viewOf(t, svc, "Other Pad").Device.ActiveMode, "a loop followed")
}

// A full restore writes the leader before its followers, so they start in
// step as they did when applied (spec 077 R3.5).
func TestARestoreWritesTheLeaderFirst(t *testing.T) {
	store := recorder(t)
	cfg := &config.Config{Devices: []config.DeviceRule{{Match: "maximus", Follow: "kraken"}}}
	first := servicetest.New(t, cfg,
		canvastest.NewBoard("ASUS ROG MAXIMUS Z790 HERO", canvastest.Zone("Addressable 1", 4)), leaderCooler())
	first.SetRecorder(store)
	red := colour.MustParse("red")
	_, err := first.Apply(t.Context(), service.Request{
		Colour: &red, Effects: map[string]scenes.Effect{"kraken": {Mode: "Tai Chi"}},
	})
	require.NoError(t, err)

	again := servicetest.New(t, cfg,
		canvastest.NewBoard("ASUS ROG MAXIMUS Z790 HERO", canvastest.Zone("Addressable 1", 4)), leaderCooler())
	again.SetRecorder(store)
	restore, err := again.Reconcile(t.Context(), nil)
	require.NoError(t, err)
	require.Len(t, restore.Results, 2)
	assert.Equal(t, "NZXT Kraken Elite V2", restore.Results[0].Device, "a follower was restored before its leader")
	assert.Equal(t, "Tai Chi", restore.Results[1].Mode)
}

// A scene that lights only the leader still lights its followers.
func TestAFollowerIsLitWhenOnlyTheLeaderIs(t *testing.T) {
	cooler := leaderCooler()
	board := canvastest.NewBoard("ASUS ROG MAXIMUS Z790 HERO", canvastest.Zone("Addressable 1", 4))
	cfg := &config.Config{Devices: []config.DeviceRule{{Match: "maximus", Follow: "kraken"}}}
	svc := servicetest.New(t, cfg, board, cooler)
	_, err := svc.Apply(t.Context(), service.Request{
		Assignments: []devices.Assignment{{Target: devices.Target{Device: "NZXT Kraken Elite V2"}, Colour: colour.MustParse("red")}},
		Effects:     map[string]scenes.Effect{"kraken": {Mode: "Tai Chi"}},
	})
	require.NoError(t, err)
	assert.Equal(t, "Tai Chi", viewOf(t, svc, "ASUS ROG MAXIMUS Z790 HERO").Device.ActiveMode)
}

// A one-colour effect stays one colour on a best fit that takes two (E4).
func TestABestFitOfOneColourFillsEverySlot(t *testing.T) {
	for _, effect := range []string{"Pulse", "Breathing"} {
		t.Run(effect, func(t *testing.T) { oneColour(t, effect) })
	}
}

func oneColour(t *testing.T, effect string) {
	gpu := msiCard()
	svc := servicetest.New(t, &config.Config{Devices: []config.DeviceRule{{Match: "msi", Follow: "kraken"}}},
		gpu, krakenWithBreathing())
	red := colour.MustParse("red")
	_, err := svc.Apply(t.Context(), service.Request{
		Colour: &red, Effects: map[string]scenes.Effect{"kraken": {Mode: effect, Colours: []string{"#ff0000"}}},
	})
	require.NoError(t, err)
	running, _ := gpu.Running()
	assert.Equal(t, "Breathing", running)
	colours, _ := gpu.Settings()
	require.Greater(t, len(colours), 1)
	for i, c := range colours {
		assert.Equal(t, lighting.Colour{R: 255}, c, "slot %d is not the colour given", i)
	}
}

// Colours given to a firmware effect that takes none are left out, not the
// effect (E4: the Kraken held red for a Rainbow Flow given two colours).
func TestColoursOnAColourlessEffectAreLeftOut(t *testing.T) {
	cooler := leaderCooler()
	svc := servicetest.New(t, nil, cooler)
	red := colour.MustParse("red")
	_, err := svc.Apply(t.Context(), service.Request{
		Colour: &red, Effects: map[string]scenes.Effect{"kraken": {Mode: "Spectrum Wave", Colours: []string{"#ff0000", "#0000ff"}}},
	})
	require.NoError(t, err)
	running, _ := cooler.Running()
	assert.Equal(t, "Spectrum Wave", running)
}

/*
An effect given no colours of its own takes the scene's colours on the
device, in order, and a follower takes the same: a red ring and blue fans are
Tai Chi in red and blue on the Kraken and on what follows it (the
maintainer's model of scenes and following).
*/
func TestAnEffectTakesTheScenesColoursAndSoDoItsFollowers(t *testing.T) {
	cooler := canvastest.NewFirmware(canvastest.NewBoard("NZXT Kraken Elite V2",
		canvastest.Ring("Ring", 24), canvastest.Ring("Fan", 8, 8, 8)), "Fading", "Tai Chi")
	cooler.MaxColours, cooler.Speeds = 3, []int{350, 300, 250, 150, 80}
	board := canvastest.NewBoard("ASUS ROG MAXIMUS Z790 HERO", canvastest.Zone("Addressable 1", 4))
	svc := servicetest.New(t, &config.Config{Devices: []config.DeviceRule{{Match: "maximus", Follow: "kraken"}}},
		board, cooler)

	red, blue := colour.MustParse("red"), colour.MustParse("blue")
	_, err := svc.Apply(t.Context(), service.Request{
		Assignments: []devices.Assignment{
			{Target: devices.Target{Device: "kraken", Part: "Ring"}, Colour: red},
			{Target: devices.Target{Device: "kraken", Part: "Fan"}, Colour: blue},
		},
		Effects: map[string]scenes.Effect{"kraken": {Mode: "Tai Chi"}},
	})
	require.NoError(t, err)

	running, _ := cooler.Running()
	require.Equal(t, "Tai Chi", running)
	colours, _ := cooler.Settings()
	assert.Equal(t, []lighting.Colour{{R: 255}, {B: 255}}, colours, "the Kraken's effect is not in the scene's colours")

	follower := viewOf(t, svc, "ASUS ROG MAXIMUS Z790 HERO").Device
	require.Equal(t, "Tai Chi", follower.ActiveMode)
	mode, _ := follower.Mode("Tai Chi")
	assert.Equal(t, []colour.Colour{red, blue}, mode.Colours, "the follower is not in the leader's colours")
}

// Rainbow Wave given no colours takes the scene's, as a gradient through
// them, rather than running the full rainbow (the live run).
func TestRainbowWaveTakesTheScenesColours(t *testing.T) {
	board := canvastest.NewBoard("ASUS ROG MAXIMUS Z790 HERO", canvastest.Zone("Addressable 1", 4))
	svc := servicetest.New(t, nil, board)
	teal, green := colour.MustParse("#4c8c88"), colour.MustParse("#788c60")
	_, err := svc.Apply(t.Context(), service.Request{
		Assignments: []devices.Assignment{
			{Target: devices.Target{Device: "maximus", Part: "Addressable 1", Picks: []config.LEDs{{First: 0, Last: 1}}}, Colour: teal},
			{Target: devices.Target{Device: "maximus", Part: "Addressable 1", Picks: []config.LEDs{{First: 2, Last: 3}}}, Colour: green},
		},
		Effects: map[string]scenes.Effect{"maximus": {Mode: "Rainbow Wave"}},
	})
	require.NoError(t, err)
	got := viewOf(t, svc, "ASUS ROG MAXIMUS Z790 HERO").Device
	require.Equal(t, "Rainbow Wave", got.ActiveMode)
	mode, _ := got.Mode("Rainbow Wave")
	assert.Equal(t, []colour.Colour{teal, green}, mode.Colours, "it ran the full rainbow")
}
