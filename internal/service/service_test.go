package service_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/ushineko/hotaru/internal/colour"
	"github.com/ushineko/hotaru/internal/config"
	"github.com/ushineko/hotaru/internal/devices"
	"github.com/ushineko/hotaru/internal/openrgb"
	"github.com/ushineko/hotaru/internal/service"
)

// A board that advertises Static and Direct. On the real Aura, Static is
// accepted and leaves the addressable headers dark.
func board() devices.Device {
	return devices.Device{
		Name:     "ASUS ROG MAXIMUS Z790 HERO",
		LEDCount: 4,
		Modes: []devices.Mode{
			{Name: "Static", PerLED: true},
			{Name: "Direct", PerLED: true},
			{Name: "Rainbow Wave"},
		},
		Zones:      []devices.Zone{{Name: "Addressable 1", First: 0, Count: 4}},
		ActiveMode: "Rainbow Wave",
	}
}

func keyboard() devices.Device {
	return devices.Device{
		Name:     "Keychron K4 HE",
		LEDCount: 2,
		Modes: []devices.Mode{
			{Name: "Direct", PerLED: true},
			{Name: "Solid Color"},
		},
		Zones:      []devices.Zone{{Name: "Keyboard", First: 0, Count: 2}},
		ActiveMode: "Solid Color",
	}
}

func strip() devices.Device {
	return devices.Device{
		Name:       "Generic Strip",
		LEDCount:   3,
		Modes:      []devices.Mode{{Name: "Static"}, {Name: "Off"}},
		Zones:      []devices.Zone{{Name: "Strip", First: 0, Count: 3}},
		ActiveMode: "Off",
	}
}

func solid(target, name string) []devices.Assignment {
	t, err := devices.ParseTarget(target)
	if err != nil {
		panic(err)
	}
	return []devices.Assignment{{Target: t, Colour: colour.MustParse(name)}}
}

func TestAMachineWithNoConfigurationGetsItsLightsSet(t *testing.T) {
	// The stranger's machine: no rules file, hardware nobody wrote a line
	// about, and it works.
	server := openrgb.NewFake(board(), keyboard(), strip())
	svc := service.New(nil, server, "")

	results, err := svc.Apply(t.Context(), service.Request{Assignments: append(
		solid("ASUS", "red"), append(solid("Keychron", "red"), solid("Generic", "red")...)...)})
	require.NoError(t, err)
	require.Len(t, results, 3)
	for _, r := range results {
		require.True(t, r.Applied, "%s: %s", r.Device, r.Skipped)
	}

	showing, _ := server.Showing("Generic Strip")
	require.Equal(t, colour.MustParse("red"), showing.Colours[0])
}

func TestADeviceThatAcceptsAModeWithoutHonouringItFallsThrough(t *testing.T) {
	// The Aura failure, discovered rather than configured: Static is taken,
	// the server says yes, and the device stays where it was. Only the
	// read-back notices, and the next candidate is tried.
	server := openrgb.NewFake(board())
	server.Lies["ASUS ROG MAXIMUS Z790 HERO"] = "Static"
	svc := service.New(nil, server, "")

	results, err := svc.Apply(t.Context(), service.Request{Assignments: solid("ASUS", "blue")})
	require.NoError(t, err)
	require.Len(t, results, 1)

	got := results[0]
	require.True(t, got.Applied)
	require.Equal(t, "Direct", got.Mode, "the mode that actually took")
	require.Len(t, got.Attempts, 2, "and the one that did not is on the record")
	require.Equal(t, "Static", got.Attempts[0].Mode)
	require.True(t, got.Attempts[0].Accepted, "the server said yes")
	require.Equal(t, "Rainbow Wave", got.Attempts[0].Active, "and the device disagreed")
}

func TestARuleSparesTheFallThroughRatherThanBeingRequiredForIt(t *testing.T) {
	// With the correction written down, the first attempt is the right one.
	server := openrgb.NewFake(board())
	server.Lies["ASUS ROG MAXIMUS Z790 HERO"] = "Static"
	cfg := &config.Config{Devices: []config.DeviceRule{
		{Match: "maximus", SolidModes: []string{"direct", "static"}},
	}}
	svc := service.New(cfg, server, "")

	results, err := svc.Apply(t.Context(), service.Request{Assignments: solid("ASUS", "blue")})
	require.NoError(t, err)
	require.True(t, results[0].Applied)
	require.Len(t, results[0].Attempts, 1, "no wasted write")
}

func TestADeviceThatCannotBeBlankedIsSkippedWithItsReason(t *testing.T) {
	server := openrgb.NewFake(keyboard())
	cfg := &config.Config{Devices: []config.DeviceRule{{Match: "keychron", NeverBlank: true}}}
	svc := service.New(cfg, server, "")

	results, err := svc.Apply(t.Context(), service.Request{Off: true})
	require.NoError(t, err)
	require.Len(t, results, 1)
	require.False(t, results[0].Applied)
	require.Contains(t, results[0].Skipped, "backlight")
	require.Empty(t, server.Modes, "and nothing was written to it")
}

func TestOffUsesTheDevicesOwnOffModeWhereThereIsOne(t *testing.T) {
	server := openrgb.NewFake(strip(), keyboard())
	svc := service.New(nil, server, "")

	results, err := svc.Apply(t.Context(), service.Request{Off: true})
	require.NoError(t, err)
	require.Len(t, results, 2)

	byDevice := map[string]service.Result{}
	for _, r := range results {
		byDevice[r.Device] = r
	}
	require.Equal(t, "Off", byDevice["Generic Strip"].Mode)
	require.Equal(t, "Direct", byDevice["Keychron K4 HE"].Mode,
		"no Off mode, so direct with black -- which is why never_blank exists")

	showing, _ := server.Showing("Keychron K4 HE")
	require.Equal(t, colour.Black, showing.Colours[0])
}

func TestADeviceOutOfScopeIsLeftAloneEntirely(t *testing.T) {
	server := openrgb.NewFake(board(), keyboard())
	cfg := &config.Config{Scope: []string{"maximus"}}
	svc := service.New(cfg, server, "")

	results, err := svc.Apply(t.Context(), service.Request{Assignments: append(
		solid("ASUS", "red"), solid("Keychron", "red")...)})
	require.NoError(t, err)
	require.Len(t, results, 1)
	require.Equal(t, "ASUS ROG MAXIMUS Z790 HERO", results[0].Device)

	for _, write := range server.Modes {
		require.NotEqual(t, "Keychron K4 HE", write.Device)
	}
}

func TestNamingADeviceNarrowsToItWithoutChangingScope(t *testing.T) {
	server := openrgb.NewFake(board(), keyboard())
	svc := service.New(nil, server, "")

	results, err := svc.Apply(t.Context(), service.Request{
		Assignments: append(solid("ASUS", "red"), solid("Keychron", "red")...),
		Devices:     []string{"keychron"},
	})
	require.NoError(t, err)
	require.Len(t, results, 1)
	require.Equal(t, "Keychron K4 HE", results[0].Device)
}

func TestADeviceThatCannotShowTheFrameIsReportedNotApproximated(t *testing.T) {
	// The strip takes one colour for the whole of itself. A two-colour frame
	// is refused rather than reduced to whichever colour came first.
	server := openrgb.NewFake(strip())
	svc := service.New(nil, server, "")

	assignments := append(solid("Generic", "red"), devices.Assignment{
		Target: mustTarget(t, "Generic/Strip[0:0]"),
		Colour: colour.MustParse("blue"),
	})
	results, err := svc.Apply(t.Context(), service.Request{Assignments: assignments})
	require.NoError(t, err)
	require.Len(t, results, 1)
	require.False(t, results[0].Applied)
	require.Contains(t, results[0].Skipped, "per LED")
	require.Empty(t, server.Writes)
}

func TestListSaysWhichDevicesHotaruWouldDriveAndWhy(t *testing.T) {
	server := openrgb.NewFake(board(), keyboard())
	cfg := &config.Config{
		Scope:   []string{"maximus"},
		Devices: []config.DeviceRule{{Match: "maximus", SolidModes: []string{"direct"}}},
	}
	svc := service.New(cfg, server, "")

	list, err := svc.List(t.Context())
	require.NoError(t, err)
	require.Len(t, list, 2, "everything present is listed")

	require.True(t, list[0].InScope)
	require.Equal(t, []string{"direct"}, list[0].Rule.SolidModes)
	require.False(t, list[1].InScope, "and the reason it will not change is visible")
}

func TestAStoppedServerIsAnOrdinaryStateWithSomethingToDoAboutIt(t *testing.T) {
	svc := service.New(nil, nil, "127.0.0.1:6742")

	_, err := svc.List(t.Context())
	var down *service.Unreachable
	require.ErrorAs(t, err, &down)
	require.Equal(t, "127.0.0.1:6742", down.Address)
	require.Contains(t, err.Error(), "start it")

	_, err = svc.Apply(t.Context(), service.Request{Off: true})
	require.ErrorAs(t, err, &down)
}

func TestAServerThatFailsMidWayReportsAgainstTheDeviceItFailedOn(t *testing.T) {
	server := openrgb.NewFake(board())
	server.Unreachable = errors.New("connection reset")
	svc := service.New(nil, server, "")

	_, err := svc.Apply(t.Context(), service.Request{Assignments: solid("ASUS", "red")})
	require.ErrorContains(t, err, "connection reset")
}

func mustTarget(t *testing.T, s string) devices.Target {
	t.Helper()
	target, err := devices.ParseTarget(s)
	require.NoError(t, err)
	return target
}

func TestABrightnessRuleIsAssertedOnEveryWrite(t *testing.T) {
	// Brightness is device state nobody owned: whatever was last written
	// stuck, so a value set once by hand became permanent and invisible.
	// Asserting it makes hotaru's picture of the device complete rather than
	// partial.
	server := openrgb.NewFake(keyboard())
	level := 100
	cfg := &config.Config{Devices: []config.DeviceRule{{Match: "keychron", Brightness: &level}}}
	svc := service.New(cfg, server, "")

	_, err := svc.Apply(t.Context(), service.Request{Assignments: solid("Keychron", "red")})
	require.NoError(t, err)

	require.NotEmpty(t, server.Modes)
	require.NotNil(t, server.Modes[0].Brightness, "the rule's brightness never reached the device")
	require.Equal(t, 100, *server.Modes[0].Brightness)

	// A device with no such rule is not sent one, because a write nobody asked
	// for is still a write.
	plain := openrgb.NewFake(board())
	svc = service.New(nil, plain, "")
	_, err = svc.Apply(t.Context(), service.Request{Assignments: solid("ASUS", "red")})
	require.NoError(t, err)
	require.Nil(t, plain.Modes[0].Brightness)
}

func TestAnAssignmentThatNamedNothingIsReportedEvenWhenTheWriteWorked(t *testing.T) {
	// "Everything blue, except the top fan" with the fan misspelled is a
	// device that went blue all over and reported success. Found on a machine
	// whose zone name contained a slash, where the typo was mine.
	server := openrgb.NewFake(board())
	svc := service.New(nil, server, "")

	typo, err := devices.ParseTarget("ASUS/Addressable 1 but spelled wrong")
	require.NoError(t, err)

	blue := colour.MustParse("blue")
	results, err := svc.Apply(t.Context(), service.Request{
		Colour:      &blue,
		Assignments: []devices.Assignment{{Target: typo, Colour: colour.MustParse("red")}},
	})
	require.NoError(t, err)
	require.Len(t, results, 1)

	require.True(t, results[0].Applied, "the rest of the request still applied")
	require.Len(t, results[0].Problems, 1)
	require.Contains(t, results[0].Problems[0], "spelled wrong")
	require.Contains(t, results[0].Problems[0], "Addressable 1", "and what does exist")
}

// ignores is a device that takes a mode and keeps showing what it liked.
type ignores struct {
	*openrgb.Fake
	device string
}

func (i ignores) SetFrame(ctx context.Context, device string, frame devices.Frame) error {
	if strings.EqualFold(device, i.device) {
		return nil // accepted, and nothing changes
	}
	return i.Fake.SetFrame(ctx, device, frame)
}

func TestADeviceThatTakesTheModeAndIgnoresTheColoursIsNotASuccess(t *testing.T) {
	/*
		Found on a machine with four identical sticks of RAM, three of which
		accept every write and change nothing -- inside OpenRGB, before any
		hardware is involved. Comparing only the mode calls that a success,
		which is how someone ends up with a scene reporting six devices lit
		and showing three.
	*/
	server := openrgb.NewFake(board())
	svc := service.New(nil, ignores{Fake: server, device: "ASUS ROG MAXIMUS Z790 HERO"}, "")

	results, err := svc.Apply(t.Context(), service.Request{Assignments: solid("ASUS", "red")})
	require.NoError(t, err)
	require.Len(t, results, 1)

	// Applied, and said out loud that nobody can confirm it. Sticks of DDR5
	// were seen reporting black while visibly red: the write had landed and
	// the buffer was stale. Refusing to call that applied marks working
	// hardware broken, and calling it silently applied hides a real failure --
	// so it is reported, and a person who can see the machine decides.
	require.True(t, results[0].Applied)
	require.Contains(t, results[0].Unconfirmed, "does not report the colours")
	require.Contains(t, results[0].Unconfirmed, "look at it")
}

func TestADeviceInAModeThatCannotReportColoursIsNotDoubted(t *testing.T) {
	// A device in Static reports its mode's colour rather than its buffer, so
	// holding the difference against it would fail every write to hardware
	// that works perfectly well.
	server := openrgb.NewFake(strip()) // Static, and not per-LED
	svc := service.New(nil, ignores{Fake: server, device: "Generic Strip"}, "")

	results, err := svc.Apply(t.Context(), service.Request{Assignments: solid("Generic", "red")})
	require.NoError(t, err)
	require.True(t, results[0].Applied, "a device that cannot answer was treated as failing")
	require.Empty(t, results[0].Unconfirmed,
		"a device in a mode that never reports colours was doubted anyway")
}

/*
cooler is a device whose Static mode carries its own colour.

Modelled on the NZXT Kraken that turned three radiator fans red when asked for
purple: Static holds a colour of its own, the per-LED buffer it also accepts is
not what it displays, and the colour already in the mode is the vendor's. See
spec 009.
*/
func cooler() devices.Device {
	return devices.Device{
		Name:     "NZXT Kraken",
		LEDCount: 2,
		Modes: []devices.Mode{
			{Name: "Static", ModeColour: true, Colour: colour.MustParse("red")},
			{Name: "Direct", PerLED: true},
		},
		Zones:      []devices.Zone{{Name: "Ring", Shape: devices.ShapeLine, First: 0, Count: 2}},
		ActiveMode: "Static",
	}
}

func TestAModeThatCarriesItsOwnColourIsGivenOne(t *testing.T) {
	/*
		The fans-went-red bug. hotaru set the mode and wrote the buffer, and
		the device showed the colour its vendor had left in Static. Asked for
		purple, it must end up purple -- by whichever half of the mode the
		hardware actually reads.
	*/
	server := openrgb.NewFake(cooler())
	svc := service.New(nil, server, "")

	results, err := svc.Apply(t.Context(), service.Request{
		Assignments: solid("Kraken/Ring", "purple"),
		Mode:        "Static",
		Exactly:     true,
	})
	require.NoError(t, err)
	require.Len(t, results, 1)
	require.True(t, results[0].Applied)
	require.Empty(t, results[0].Unconfirmed, "the colour was set and read back; nothing to doubt")

	after, err := server.Device(t.Context(), "NZXT Kraken")
	require.NoError(t, err)
	mode, ok := after.Mode("Static")
	require.True(t, ok)
	require.Equal(t, colour.MustParse("purple"), mode.Colour,
		"the mode kept the vendor's colour, which is what the device displays")
	for _, c := range after.Colours {
		require.Equal(t, colour.MustParse("purple"), c,
			"the device is showing a colour nobody asked for")
	}
}

func TestAPerLEDModeIsPreferredOverOneThatHoldsItsOwnColour(t *testing.T) {
	// Both can show a solid colour. Only one can be checked afterwards, and
	// the default order picks that one -- spec 009 R3.
	server := openrgb.NewFake(cooler())
	svc := service.New(nil, server, "")

	results, err := svc.Apply(t.Context(), service.Request{Assignments: solid("Kraken", "green")})
	require.NoError(t, err)
	require.Len(t, results, 1)
	require.True(t, results[0].Applied)
	require.Equal(t, "Direct", results[0].Mode)

	// And the buffer is what it shows, with the other mode's stored colour
	// left exactly as the vendor had it: hotaru changes what it was asked to
	// change and no more.
	after, err := server.Device(t.Context(), "NZXT Kraken")
	require.NoError(t, err)
	require.Equal(t, colour.MustParse("green"), after.Colours[0])
	static, ok := after.Mode("Static")
	require.True(t, ok)
	require.Equal(t, colour.MustParse("red"), static.Colour,
		"a mode nobody selected had its colour rewritten")
}

func TestARuleNamingItsModesStillWins(t *testing.T) {
	// A person who has looked at their machine outranks the default. The
	// development machine needed exactly this for its board.
	cfg := &config.Config{Devices: []config.DeviceRule{
		{Match: "kraken", SolidModes: []string{"static", "direct"}},
	}}
	server := openrgb.NewFake(cooler())
	svc := service.New(cfg, server, "")

	results, err := svc.Apply(t.Context(), service.Request{Assignments: solid("Kraken", "green")})
	require.NoError(t, err)
	require.Equal(t, "Static", results[0].Mode)
}

func TestAModeColourThatReadsBackWrongIsReported(t *testing.T) {
	// The device took the mode and kept a different colour. Applied, because
	// the mode landed; unconfirmed, because what it displays is not what was
	// asked for.
	device := cooler()
	server := openrgb.NewFake(device)
	svc := service.New(nil, stuckColour{Fake: server, device: "NZXT Kraken"}, "")

	results, err := svc.Apply(t.Context(), service.Request{
		Assignments: solid("Kraken", "purple"),
		Mode:        "Static",
		Exactly:     true,
	})
	require.NoError(t, err)
	require.True(t, results[0].Applied)
	require.Contains(t, results[0].Unconfirmed, "look at it")
}

func TestANonUniformFrameNeverLandsInAModeThatCannotShowIt(t *testing.T) {
	// Two colours cannot be shown by a mode that holds one. Spec 009 R2: the
	// fix for the red fans must not make this worse.
	server := openrgb.NewFake(cooler())
	svc := service.New(nil, server, "")

	results, err := svc.Apply(t.Context(), service.Request{Assignments: []devices.Assignment{
		{Target: mustTarget(t, "Kraken/Ring[0:0]"), Colour: colour.MustParse("red")},
		{Target: mustTarget(t, "Kraken/Ring[1:1]"), Colour: colour.MustParse("blue")},
	}})
	require.NoError(t, err)
	require.True(t, results[0].Applied)
	require.Equal(t, "Direct", results[0].Mode, "a two-colour frame went to a one-colour mode")
}

/*
stuckColour is a device that takes a mode and keeps the colour it already had.

The read-back half of the fans-went-red bug: the mode landed, and what the
device displays is still the vendor's. Nothing but reading the mode's colour
back can tell.
*/
type stuckColour struct {
	*openrgb.Fake
	device string
}

func (s stuckColour) SetMode(ctx context.Context, device, mode string, brightness *int, c *colour.Colour) error {
	if strings.EqualFold(device, s.device) {
		return s.Fake.SetMode(ctx, device, mode, brightness, nil)
	}
	return s.Fake.SetMode(ctx, device, mode, brightness, c)
}
