package service_test

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/ushineko/hotaru/internal/colour"
	"github.com/ushineko/hotaru/internal/openrgb"
	"github.com/ushineko/hotaru/internal/service"
)

func TestProbeFindsTheModeThatIsAcceptedAndIgnored(t *testing.T) {
	// The whole point of probing rather than reading: the device advertises
	// Static, takes the write, reports success, and stays where it was.
	server := openrgb.NewFake(board())
	server.Lies["ASUS ROG MAXIMUS Z790 HERO"] = "Static"
	svc := service.New(nil, server, "")

	findings, err := svc.Probe(t.Context(), nil)
	require.NoError(t, err)
	require.Len(t, findings, 1)

	byMode := map[string]service.ModeFinding{}
	for _, mode := range findings[0].Modes {
		byMode[mode.Name] = mode
	}
	require.True(t, byMode["Static"].Tried)
	require.False(t, byMode["Static"].Took, "accepted, and not honoured")
	require.True(t, byMode["Direct"].Took)
	require.True(t, byMode["Direct"].PerLED, "and it can carry more than one colour")

	require.Contains(t, findings[0].Suggested, "solid_modes: [direct, static]",
		"the mode that worked first, and the one that did not kept as a fallback")
}

func TestProbeSuggestsNothingForADeviceThatNeedsNothing(t *testing.T) {
	// The common and correct answer. A probe that suggested a rule for every
	// device would turn a diagnostic into a configuration generator, and a
	// rules file full of lines nobody understands is one nobody can safely
	// delete from.
	svc := service.New(nil, openrgb.NewFake(strip()), "")

	findings, err := svc.Probe(t.Context(), nil)
	require.NoError(t, err)
	require.Empty(t, findings[0].Suggested)
}

func TestProbeReportsNoOffModeAsAQuestionNotAnInstruction(t *testing.T) {
	// Black is "off" for a strip and a dead backlight for a keyboard, and the
	// probe cannot tell which it is looking at. So it says what it found and
	// what that would mean, and leaves the decision where it belongs.
	svc := service.New(nil, openrgb.NewFake(keyboard()), "")

	findings, err := svc.Probe(t.Context(), nil)
	require.NoError(t, err)
	require.True(t, findings[0].NoOffMode)
	require.Contains(t, findings[0].Suggested, "no Off mode")
	require.Contains(t, findings[0].Suggested, "# never_blank: true",
		"suggested as a comment, because only a person knows if it is right")

	// A device with an Off mode raises nothing at all.
	svc = service.New(nil, openrgb.NewFake(strip()), "")
	findings, err = svc.Probe(t.Context(), nil)
	require.NoError(t, err)
	require.False(t, findings[0].NoOffMode)
	require.Empty(t, findings[0].Suggested)
}

func TestProbePutsEveryDeviceBackAsItFoundIt(t *testing.T) {
	// A diagnostic that leaves the lights different has cost more than it
	// explained.
	server := openrgb.NewFake(board())
	svc := service.New(nil, server, "")

	teal := colour.MustParse("teal")
	_, err := svc.Apply(t.Context(), service.Request{Assignments: solid("ASUS", "teal")})
	require.NoError(t, err)

	before, ok := server.Showing("ASUS ROG MAXIMUS Z790 HERO")
	require.True(t, ok)
	devicesBefore, err := server.Devices(t.Context())
	require.NoError(t, err)
	modeBefore := devicesBefore[0].ActiveMode

	_, err = svc.Probe(t.Context(), nil)
	require.NoError(t, err)

	after, _ := server.Showing("ASUS ROG MAXIMUS Z790 HERO")
	require.Equal(t, before.Colours, after.Colours, "the colours were left changed")
	require.Equal(t, teal, after.Colours[0])

	devicesAfter, err := server.Devices(t.Context())
	require.NoError(t, err)
	require.Equal(t, modeBefore, devicesAfter[0].ActiveMode, "the mode was left changed")
}

func TestProbeReportsZonesBecauseThatIsWhereNamingStarts(t *testing.T) {
	svc := service.New(nil, openrgb.NewFake(board()), "")

	findings, err := svc.Probe(t.Context(), nil)
	require.NoError(t, err)
	require.Equal(t, []service.ZoneFinding{{Name: "Addressable 1", First: 0, Count: 4}}, findings[0].Zones)
}

func TestProbeLeavesEffectModesAlone(t *testing.T) {
	// Setting Rainbow Wave to see whether it takes would be a light show
	// nobody asked for.
	server := openrgb.NewFake(board())
	svc := service.New(nil, server, "")

	findings, err := svc.Probe(t.Context(), nil)
	require.NoError(t, err)
	for _, mode := range findings[0].Modes {
		require.NotEqual(t, "Rainbow Wave", mode.Name)
	}
}
