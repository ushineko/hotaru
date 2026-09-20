package service_test

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/ushineko/hotaru/internal/colour"
	"github.com/ushineko/hotaru/internal/config"
	"github.com/ushineko/hotaru/internal/devices"
	"github.com/ushineko/hotaru/internal/openrgb"
	"github.com/ushineko/hotaru/internal/service"
	"github.com/ushineko/hotaru/internal/state"
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
	server := openrgb.NewFake(board(), keyboard())
	svc := service.New(nil, server, "")
	svc.SetRecorder(recorder(t))

	restore, err := svc.Reconcile(t.Context(), nil)
	require.NoError(t, err)
	require.True(t, restore.Complete())
	require.Zero(t, restore.Applied)
	require.Empty(t, server.Writes, "a fresh machine was written to")
	require.Empty(t, server.Modes)
}

func TestWhatWasAppliedIsWhatIsPutBack(t *testing.T) {
	server := openrgb.NewFake(board())
	svc := service.New(nil, server, "")
	svc.SetRecorder(recorder(t))

	_, err := svc.Apply(t.Context(), service.Request{Assignments: solid("ASUS", "teal")})
	require.NoError(t, err)

	// Something else moves the device, as a sleeping mouse or a restarted
	// server would.
	require.NoError(t, server.SetFrame(t.Context(), "ASUS ROG MAXIMUS Z790 HERO", devices.Frame{
		Device:  "ASUS ROG MAXIMUS Z790 HERO",
		Colours: make([]colour.Colour, 4),
	}))
	showing, _ := server.Showing("ASUS ROG MAXIMUS Z790 HERO")
	require.Equal(t, colour.Black, showing.Colours[0])

	restore, err := svc.Reconcile(t.Context(), nil)
	require.NoError(t, err)
	require.True(t, restore.Complete())
	require.Equal(t, 1, restore.Applied)

	showing, _ = server.Showing("ASUS ROG MAXIMUS Z790 HERO")
	require.Equal(t, colour.MustParse("teal"), showing.Colours[0])
}

func TestAReconcileIsNotANewUserChoice(t *testing.T) {
	// Desired state must not move underneath a reconcile: if it did, a
	// re-assert would be indistinguishable from an instruction, and the thing
	// being restored would slowly become whatever was last restored.
	server := openrgb.NewFake(board())
	svc := service.New(nil, server, "")
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
	// OpenRGB enumerates once at server start, and a cold boot has been seen
	// finding two devices out of six. Calling that a success is how a machine
	// ends up half-lit with everything claiming to be fine.
	server := openrgb.NewFake(board(), keyboard())
	svc := service.New(nil, server, "")
	svc.SetRecorder(recorder(t))

	_, err := svc.Apply(t.Context(), service.Request{Assignments: append(
		solid("ASUS", "red"), solid("Keychron", "red")...)})
	require.NoError(t, err)

	// The keyboard has not enumerated this time round.
	partial := openrgb.NewFake(board())
	svc.SetClient(partial)

	restore, err := svc.Reconcile(t.Context(), nil)
	require.NoError(t, err)
	require.False(t, restore.Complete(), "two devices were recorded and one was reached")
	require.Equal(t, []string{"Keychron K4 HE"}, restore.Missing)
	require.Equal(t, 1, restore.Applied)

	// And when it turns up, the restore finishes with no further instruction.
	partial.Add(keyboard())
	restore, err = svc.Reconcile(t.Context(), nil)
	require.NoError(t, err)
	require.True(t, restore.Complete())
	require.Equal(t, 2, restore.Applied)
}

func TestTurningADeviceOffIsNotSomethingToRestore(t *testing.T) {
	// Putting black back at boot is not what anybody meant by "off".
	server := openrgb.NewFake(strip())
	svc := service.New(nil, server, "")
	svc.SetRecorder(recorder(t))

	_, err := svc.Apply(t.Context(), service.Request{Assignments: solid("Generic", "red")})
	require.NoError(t, err)
	require.Contains(t, svc.Desired().Devices, "Generic Strip")

	_, err = svc.Apply(t.Context(), service.Request{Off: true})
	require.NoError(t, err)
	require.NotContains(t, svc.Desired().Devices, "Generic Strip")
}

func TestOnlyDevicesWithARuleAreReasserted(t *testing.T) {
	// Re-asserting everything would write constantly to hardware that has no
	// such problem, and a write nobody needs can still collide with something.
	cfg := &config.Config{Devices: []config.DeviceRule{
		{Match: "keychron", Reassert: config.Duration(time.Minute)},
	}}
	svc := service.New(cfg, openrgb.NewFake(board(), keyboard()), "")

	rules, err := svc.ReassertRules(t.Context())
	require.NoError(t, err)
	require.Equal(t, map[string]time.Duration{"Keychron K4 HE": time.Minute}, rules)
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

func TestReconcilingWithNoServerSaysSoRatherThanForgetting(t *testing.T) {
	svc := service.New(nil, nil, "127.0.0.1:6742")
	svc.SetRecorder(recorder(t))

	server := openrgb.NewFake(board())
	svc.SetClient(server)
	_, err := svc.Apply(t.Context(), service.Request{Assignments: solid("ASUS", "red")})
	require.NoError(t, err)

	svc.SetClient(nil)
	_, err = svc.Reconcile(t.Context(), nil)
	var down *service.Unreachable
	require.ErrorAs(t, err, &down)
	require.Contains(t, svc.Desired().Devices, "ASUS ROG MAXIMUS Z790 HERO",
		"a server that went away is not a reason to forget what the lights should be")
}

func TestAPartialSceneBuildsOnWhatHotaruWroteNotOnWhatTheDeviceClaims(t *testing.T) {
	// A device's reported colours are its LED buffer only while it is in a
	// per-LED mode. Observed on real hardware: a board in Static reports its
	// mode colour, and a cooler in Static reports black while visibly lit.
	// Composing an exception onto either would blank the rest of the scene.
	server := openrgb.NewFake(board())
	svc := service.New(nil, server, "")
	svc.SetRecorder(recorder(t))

	_, err := svc.Apply(t.Context(), service.Request{Assignments: solid("ASUS", "teal")})
	require.NoError(t, err)

	// The device now misreports what it is showing, as real hardware does.
	lying := openrgb.NewFake(board())
	require.NoError(t, lying.SetFrame(t.Context(), "ASUS ROG MAXIMUS Z790 HERO", devices.Frame{
		Device: "ASUS ROG MAXIMUS Z790 HERO", Colours: make([]colour.Colour, 4),
	}))
	svc.SetClient(lying)

	// One LED is changed; the rest should keep the teal hotaru last wrote.
	target, err := devices.ParseTarget("ASUS/Addressable 1[0:0]")
	require.NoError(t, err)
	_, err = svc.Apply(t.Context(), service.Request{Assignments: []devices.Assignment{
		{Target: target, Colour: colour.MustParse("red")},
	}})
	require.NoError(t, err)

	showing, _ := lying.Showing("ASUS ROG MAXIMUS Z790 HERO")
	require.Equal(t, colour.MustParse("red"), showing.Colours[0], "the exception")
	require.Equal(t, colour.MustParse("teal"), showing.Colours[1],
		"the rest of the scene was blanked by trusting the device's report")
}
