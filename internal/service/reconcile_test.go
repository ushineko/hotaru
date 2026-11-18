package service_test

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/ushineko/hotaru/internal/colour"
	"github.com/ushineko/hotaru/internal/devices"
	"github.com/ushineko/hotaru/internal/service"
	"github.com/ushineko/hotaru/internal/service/servicetest"
	"github.com/ushineko/hotaru/internal/state"
	"github.com/ushineko/sanshoku/lighting"
)

func recorder(t *testing.T) *state.Store {
	t.Helper()
	store, err := state.Open(filepath.Join(t.TempDir(), "state.yml"))
	require.NoError(t, err)
	return store
}

func TestAFreshInstallReconcilesByTouchingNothing(t *testing.T) {
	// The inert rule, end to end. A service that has never been asked for
	// anything has nothing to put back, so a reconcile at boot is a no-op --
	// which is what stops hotaru stamping over lighting someone configured
	// with another tool before they ever used it.
	board, kb := auraBoard(), k4()
	svc := servicetest.New(t, nil, board, kb)
	svc.SetRecorder(recorder(t))

	restore, err := svc.Reconcile(t.Context(), nil)
	require.NoError(t, err)
	require.True(t, restore.Complete())
	require.Zero(t, restore.Applied)
	time.Sleep(30 * time.Millisecond)
	require.Zero(t, board.Frames(), "a fresh machine was written to")
	require.Zero(t, kb.Frames())
	require.Zero(t, kb.Writes())
}

func TestWhatWasAppliedIsWhatIsPutBack(t *testing.T) {
	board := auraBoard()
	svc := servicetest.New(t, nil, board)
	svc.SetRecorder(recorder(t))

	_, err := svc.Apply(t.Context(), service.Request{Assignments: solid("ASUS", "teal")})
	require.NoError(t, err)
	require.Eventually(t, func() bool { return shows(t, board.Device, colour.MustParse("teal")) }, eventually, every)

	// Something else moves the device, as a sleeping mouse or a rebooted
	// keyboard would.
	black := make([]lighting.Pixel, board.Lights)
	for i := range black {
		black[i] = lighting.Pixel{ID: byte(i + 1)}
	}
	require.NoError(t, board.Frame(t.Context(), black))
	require.True(t, shows(t, board.Device, colour.Black))

	restore, err := svc.Reconcile(t.Context(), nil)
	require.NoError(t, err)
	require.True(t, restore.Complete())
	require.Equal(t, 1, restore.Applied)
	require.Eventually(t, func() bool { return shows(t, board.Device, colour.MustParse("teal")) }, eventually, every)
}

func TestAReconcileIsNotANewUserChoice(t *testing.T) {
	// Desired state must not move underneath a reconcile: if it did, a
	// re-assert would be indistinguishable from an instruction, and the thing
	// being restored would slowly become whatever was last restored.
	svc := servicetest.New(t, nil, auraBoard())
	svc.SetRecorder(recorder(t))

	_, err := svc.Apply(t.Context(), service.Request{Assignments: solid("ASUS", "red")})
	require.NoError(t, err)
	before := svc.Desired().Devices["ASUS ROG MAXIMUS Z790 HERO"]

	time.Sleep(2 * time.Millisecond)
	_, err = svc.Reconcile(t.Context(), nil)
	require.NoError(t, err)

	after := svc.Desired().Devices["ASUS ROG MAXIMUS Z790 HERO"]
	require.Equal(t, before.Applied, after.Applied, "the timestamp moved, so a reconcile looked like a request")
	require.Equal(t, before.Colours, after.Colours)
}

func TestARestoreThatReachedFewerDevicesIsUnfinished(t *testing.T) {
	// A device can come up later than the service: a wireless keyboard asleep,
	// a receiver enumerating late. Calling that a success is how a machine
	// ends up half-lit with everything claiming to be fine.
	svc := service.New(nil)
	svc.SetRecorder(recorder(t))
	servicetest.Attach(t, svc, auraBoard())
	kb := servicetest.Attach(t, svc, k4())

	_, err := svc.Apply(t.Context(), service.Request{Assignments: append(
		solid("ASUS", "red"), solid("Keychron", "red")...)})
	require.NoError(t, err)

	// The keyboard has gone this time round.
	svc.DetachCanvas(kb.Attach)

	restore, err := svc.Reconcile(t.Context(), nil)
	require.NoError(t, err)
	require.False(t, restore.Complete(), "two devices were recorded and one was reached")
	require.Equal(t, []string{"Keychron K4 HE"}, restore.Missing)
	require.Equal(t, 1, restore.Applied)

	// And when it turns up, the restore finishes with no further instruction.
	servicetest.Attach(t, svc, k4())
	restore, err = svc.Reconcile(t.Context(), nil)
	require.NoError(t, err)
	require.True(t, restore.Complete())
	require.Equal(t, 2, restore.Applied)
}

func TestTurningADeviceOffIsNotSomethingToRestore(t *testing.T) {
	// Putting black back at boot is not what anybody meant by "off".
	svc := servicetest.New(t, nil, genericStrip())
	svc.SetRecorder(recorder(t))

	_, err := svc.Apply(t.Context(), service.Request{Assignments: solid("Generic", "red")})
	require.NoError(t, err)
	require.Contains(t, svc.Desired().Devices, "Generic Strip")

	_, err = svc.Apply(t.Context(), service.Request{Off: true})
	require.NoError(t, err)
	require.NotContains(t, svc.Desired().Devices, "Generic Strip")
}

func TestADeviceIsDueTheFirstTimeAndThenOnItsInterval(t *testing.T) {
	// Due immediately on first sight: hotaru has no idea what happened while
	// it was not running, and the cheapest way to find out is to send what
	// should be there.
	reassert := service.NewReasserter()
	rules := map[string]time.Duration{"Mouse": time.Minute}
	now := time.Now()

	require.Equal(t, []string{"Mouse"}, reassert.Due(now, rules))
	require.Empty(t, reassert.Due(now.Add(30*time.Second), rules), "not yet")
	require.Equal(t, []string{"Mouse"}, reassert.Due(now.Add(61*time.Second), rules))

	// A device that went away is due again when it comes back, because what
	// happened while it was gone is exactly what is not known.
	reassert.Forget("Mouse")
	require.Equal(t, []string{"Mouse"}, reassert.Due(now.Add(62*time.Second), rules))
}

// The half of the test above a canvas keeps: an exception composes onto the
// scene hotaru last wrote (spec 076 moved it off OpenRGB).
func TestAPartialSceneOnACanvasBuildsOnWhatHotaruWrote(t *testing.T) {
	board := auraBoard()
	svc := servicetest.New(t, nil, board)
	svc.SetRecorder(recorder(t))

	_, err := svc.Apply(t.Context(), service.Request{Assignments: solid("ASUS", "teal")})
	require.NoError(t, err)
	target, err := devices.ParseTarget("ASUS/Addressable 1[0:0]")
	require.NoError(t, err)
	_, err = svc.Apply(t.Context(), service.Request{Assignments: []devices.Assignment{
		{Target: target, Colour: colour.MustParse("red")},
	}})
	require.NoError(t, err)

	// Key 1 is the board's own light; the header's first is key 2.
	require.Eventually(t, func() bool {
		first, _ := board.Showing(2)
		second, _ := board.Showing(3)
		return first == lighting.Pixel{ID: 2, R: 255} && second == lighting.Pixel{ID: 3, G: 255, B: 128}
	}, eventually, every, "the rest of the scene was not kept")
}
