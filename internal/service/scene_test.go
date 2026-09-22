package service_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/ushineko/hotaru/internal/colour"
	hw "github.com/ushineko/hotaru/internal/cooler" // `cooler` is a fixture in this package
	"github.com/ushineko/hotaru/internal/openrgb"
	"github.com/ushineko/hotaru/internal/scenes"
	"github.com/ushineko/hotaru/internal/service"
	"github.com/ushineko/hotaru/internal/state"
)

// lit is a service over two fake devices with somewhere to keep scenes and
// somewhere to remember what was asked for.
func lit(t *testing.T, saved ...scenes.Scene) (*service.Service, *openrgb.Fake) {
	t.Helper()

	server := openrgb.NewFake(board(), keyboard())
	svc := service.New(nil, server, "")

	desired, err := state.Open(filepath.Join(t.TempDir(), "state.yml"))
	require.NoError(t, err)
	svc.SetRecorder(desired)

	store, err := scenes.Open(filepath.Join(t.TempDir(), "scenes.yml"))
	require.NoError(t, err)
	for _, scene := range saved {
		require.NoError(t, store.Save(scene))
	}
	svc.SetScenes(store)
	return svc, server
}

// showing is what a device is displaying on the fake server.
func showing(t *testing.T, server *openrgb.Fake, device string) []colour.Colour {
	t.Helper()
	frame, ok := server.Showing(device)
	require.True(t, ok, "%s is not on the server", device)
	return frame.Colours
}

func blue() scenes.Scene {
	return scenes.Scene{
		Name: "blue",
		Assignments: []scenes.Assignment{
			{Target: "ASUS", Colour: "blue"},
			{Target: "Keychron", Colour: "blue"},
		},
	}
}

func TestASceneLightsWhatItNames(t *testing.T) {
	svc, _ := lit(t, blue())

	done, err := svc.ApplyScene(t.Context(), "blue")
	require.NoError(t, err)
	require.Equal(t, "blue", done.Scene)
	require.Len(t, done.Results, 2)
	for _, result := range done.Results {
		require.True(t, result.Applied, "%s was not lit", result.Device)
	}
}

func TestALaterAssignmentWins(t *testing.T) {
	/*
		"Everything blue except the top fan" is two lines, and it only works
		because the second is composed over the first into one frame. A
		partial write would race with a reconcile and leave a device showing
		halves of two scenes.
	*/
	scene := blue()
	scene.Assignments = append(scene.Assignments,
		scenes.Assignment{Target: "ASUS/Addressable 1[0:0]", Colour: "red"})
	svc, server := lit(t, scene)

	_, err := svc.ApplyScene(t.Context(), "blue")
	require.NoError(t, err)

	frame := showing(t, server, "ASUS ROG MAXIMUS Z790 HERO")
	require.Equal(t, "#ff0000", frame[0].String())
	require.Equal(t, "#0000ff", frame[1].String())
}

func TestAnEffectIsPreferredForItsOwnDevice(t *testing.T) {
	// A scene wants the keyboard doing one thing and the board another, which
	// is why an effect is named per device rather than per request.
	scene := blue()
	scene.Effects = map[string]string{"Keychron": "Solid Color"}
	svc, _ := lit(t, scene)

	done, err := svc.ApplyScene(t.Context(), "blue")
	require.NoError(t, err)

	for _, result := range done.Results {
		if result.Device == "Keychron K4 HE" {
			require.Equal(t, "Solid Color", result.Mode)
		}
	}
}

func TestAnEffectADeviceDoesNotHaveCostsTheEffectNotTheScene(t *testing.T) {
	/*
		The case an effect hotaru renders itself will arrive into: a name that
		resolves to nothing. The colours are still what somebody asked for, so
		the scene applies -- and the name is reported, because silently
		ignoring it makes a scene that worked indistinguishable from one that
		did half of what it said.
	*/
	scene := blue()
	scene.Effects = map[string]string{"Keychron": "Storm"}
	svc, _ := lit(t, scene)

	done, err := svc.ApplyScene(t.Context(), "blue")
	require.NoError(t, err)

	var said string
	for _, result := range done.Results {
		if result.Device == "Keychron K4 HE" {
			require.True(t, result.Applied, "the colours were dropped with the effect")
			require.NotEmpty(t, result.Problems)
			said = result.Problems[0]
		}
	}
	require.Contains(t, said, "Storm")
}

func TestApplyingASceneIsRemembered(t *testing.T) {
	// A scene is what somebody wants their machine to look like, so it
	// survives a device that forgets and a reboot.
	svc, _ := lit(t, blue())

	_, err := svc.ApplyScene(t.Context(), "blue")
	require.NoError(t, err)

	desired := svc.Desired()
	require.Contains(t, desired.Devices, "Keychron K4 HE")
}

func TestPreviewingASceneIsNotRemembered(t *testing.T) {
	// A preview is what somebody is looking at, never what they want.
	svc, _ := lit(t, blue())

	done, err := svc.PreviewScene(t.Context(), "blue", "a test", false)
	require.NoError(t, err)
	require.NotNil(t, done.Lease)
	require.True(t, svc.Desired().Empty(), "a draft was recorded as an intention")
}

func TestReassertionLeavesADraftAlone(t *testing.T) {
	/*
		The G502's re-assert timer restores desired state within sixty
		seconds. Without this, it does so underneath the person deciding
		whether they like what they are looking at.
	*/
	svc, _ := lit(t, blue())
	_, err := svc.ApplyScene(t.Context(), "blue")
	require.NoError(t, err)

	_, err = svc.PreviewScene(t.Context(), "blue", "a test", false)
	require.NoError(t, err)

	restore, err := svc.Reconcile(t.Context(), nil)
	require.NoError(t, err)
	require.ElementsMatch(t, []string{"ASUS ROG MAXIMUS Z790 HERO", "Keychron K4 HE"}, restore.Previewing)
	require.Zero(t, restore.Applied, "a device being looked at was written to anyway")
}

func TestReleasingAPreviewPutsTheLightsBack(t *testing.T) {
	svc, server := lit(t, blue())
	_, err := svc.ApplyScene(t.Context(), "blue")
	require.NoError(t, err)

	draft := blue()
	draft.Name = "draft"
	draft.Assignments = []scenes.Assignment{{Target: "Keychron", Colour: "red"}}
	require.NoError(t, svc.SaveScene(draft))

	held, err := svc.PreviewScene(t.Context(), "draft", "a test", false)
	require.NoError(t, err)
	require.Equal(t, "#ff0000", showing(t, server, "Keychron K4 HE")[0].String())

	_, err = svc.Release(t.Context(), held.Lease.Token)
	require.NoError(t, err)
	require.Equal(t, "#0000ff", showing(t, server, "Keychron K4 HE")[0].String(),
		"ending the preview left the draft on the hardware")
}

func TestALapsedPreviewIsEndedByTheLoopThatWouldHaveCorrectedIt(t *testing.T) {
	/*
		The holder stopped talking. Nothing else will put this back: the
		correction that would have is suspended on that holder's behalf, which
		is exactly why the lease has to lapse.
	*/
	svc, server := lit(t, blue())
	_, err := svc.ApplyScene(t.Context(), "blue")
	require.NoError(t, err)

	draft := scenes.Scene{Name: "draft", Assignments: []scenes.Assignment{{Target: "Keychron", Colour: "red"}}}
	require.NoError(t, svc.SaveScene(draft))
	held, err := svc.PreviewScene(t.Context(), "draft", "a client that died", false)
	require.NoError(t, err)

	// Nobody renews it. Reaching into the lease is the one thing a test can
	// do that waiting ten seconds also does.
	held.Lease.Expires = time.Now().Add(-time.Second)

	restore, err := svc.Expired(t.Context())
	require.NoError(t, err)
	require.Equal(t, 1, restore.Applied)
	require.Equal(t, "#0000ff", showing(t, server, "Keychron K4 HE")[0].String())
}

func TestAConnectionBoundLeaseDoesNotLapse(t *testing.T) {
	// Its holder is sitting on an open request, and the socket closing is a
	// better signal than any clock. Nothing should expire it in the meantime.
	svc, _ := lit(t, blue())

	held, err := svc.PreviewScene(t.Context(), "blue", "a GUI", true)
	require.NoError(t, err)
	require.True(t, held.Lease.Expires.IsZero())

	restore, err := svc.Expired(t.Context())
	require.NoError(t, err)
	require.Zero(t, restore.Applied)
	_, still := svc.Previewing("Keychron K4 HE")
	require.True(t, still, "a lease bound to a connection was expired by a clock")
}

func TestTwoCallersCannotPreviewTheSameDevice(t *testing.T) {
	// Writing over a draft somebody else is looking at is the confusion this
	// whole mechanism exists to prevent.
	svc, _ := lit(t, blue())
	_, err := svc.PreviewScene(t.Context(), "blue", "the first client", false)
	require.NoError(t, err)

	_, err = svc.PreviewScene(t.Context(), "blue", "the second client", false)
	require.ErrorContains(t, err, "the first client")
}

func TestRenewingKeepsALeaseAlive(t *testing.T) {
	svc, _ := lit(t, blue())
	held, err := svc.PreviewScene(t.Context(), "blue", "a script", false)
	require.NoError(t, err)

	held.Lease.Expires = time.Now().Add(time.Millisecond)
	require.NoError(t, svc.Renew(held.Lease.Token))

	restore, err := svc.Expired(t.Context())
	require.NoError(t, err)
	require.Zero(t, restore.Applied)
}

func TestRenewingSomethingThatLapsedSaysSo(t *testing.T) {
	// The devices may belong to somebody else by now, so a lapsed lease is
	// not quietly revived.
	svc, _ := lit(t, blue())
	require.ErrorContains(t, svc.Renew("nothing"), "expired")
}

func TestASceneSaysWhatItDidToTheScreen(t *testing.T) {
	// A scene that says nothing about the screen changes nothing about it.
	svc, _ := lit(t, blue())

	done, err := svc.ApplyScene(t.Context(), "blue")
	require.NoError(t, err)
	require.Empty(t, done.Screen)
}

func TestAMissingImageCostsTheScreenAndNotTheColours(t *testing.T) {
	/*
		The animation bank this replaces pointed at files under one person's
		home directory. A scene whose GIF has moved should light the room and
		tell them which file to go and find.
	*/
	scene := blue()
	scene.Screen = filepath.Join(t.TempDir(), "gone.gif")
	svc, _ := lit(t, scene)
	svc.SetCooler(&panel{})

	done, err := svc.ApplyScene(t.Context(), "blue")
	require.NoError(t, err)
	require.Len(t, done.Results, 2)
	require.True(t, done.Results[0].Applied)
	require.Len(t, done.Problems, 1)
	require.Contains(t, done.Problems[0], "gone.gif")
}

func TestASceneCanTakeTheScreen(t *testing.T) {
	gif := filepath.Join(t.TempDir(), "rain.gif")
	require.NoError(t, os.WriteFile(gif, []byte("GIF89a"), 0o600))

	scene := blue()
	scene.Screen = gif
	svc, _ := lit(t, scene)
	screen := &panel{}
	svc.SetCooler(screen)
	svc.SetDashboard(&screenAuthor{})

	done, err := svc.ApplyScene(t.Context(), "blue")
	require.NoError(t, err)
	require.Equal(t, gif, done.Screen)
	require.Equal(t, 1, screen.shown)
}

func TestCapturingKeepsWhatIsShowingNow(t *testing.T) {
	// The shortcut somebody reaches for after fiddling until it looks right.
	svc, _ := lit(t, blue())
	_, err := svc.ApplyScene(t.Context(), "blue")
	require.NoError(t, err)

	captured, err := svc.SceneFrom("kept", scenes.ScreenDashboard)
	require.NoError(t, err)
	require.Len(t, captured.Assignments, 2)
	require.Equal(t, "#0000ff", captured.Assignments[0].Colour)
	require.Equal(t, scenes.ScreenDashboard, captured.Screen)
}

func TestCapturingNothingSaysSo(t *testing.T) {
	svc, _ := lit(t)
	_, err := svc.SceneFrom("empty", "")
	require.ErrorContains(t, err, "nothing is recorded")
}

func TestAMachineWithNoScenesFileSaysSoRatherThanFailingQuietly(t *testing.T) {
	// An unreadable scenes file is somebody's saved work, so the service runs
	// without scenes rather than replacing them.
	svc := service.New(nil, openrgb.NewFake(board()), "")

	_, err := svc.Scenes()
	require.ErrorIs(t, err, service.ErrNoScenes)

	_, err = svc.ApplyScene(context.Background(), "anything")
	require.ErrorIs(t, err, service.ErrNoScenes)
}

func TestTheShippedKeysApplyTheShippedScenes(t *testing.T) {
	/*
		The bank this desk's hands already know: Ctrl+Alt+Num1 has been red
		for two years, and a rearchitecture that changes what a key does has
		broken something no test would otherwise catch.
	*/
	svc, _ := lit(t)

	keys, err := svc.Keys()
	require.NoError(t, err)
	require.Len(t, keys.Bindings, 9)

	bound := map[string]string{}
	for _, binding := range keys.Bindings {
		require.False(t, binding.Missing, "%s applies a scene that does not exist", binding.Key)
		bound[binding.Key] = binding.Scene
	}
	require.Equal(t, "red", bound["Ctrl+Alt+Num+1"])
	require.Equal(t, "off", bound["Ctrl+Alt+Num+9"])
	require.NotContains(t, bound, "Ctrl+Alt+Shift+Num+1",
		"hotaru bound a key it reserves for somebody else's scenes")
}

func TestApplyingTheNinthSceneTurnsTheLightsOff(t *testing.T) {
	// "off" is not a colour. It resolves the device's own Off mode and
	// honours the keyboard correction, which no assignment can ask for.
	svc, server := lit(t)
	_, err := svc.ApplyScene(t.Context(), "red")
	require.NoError(t, err)
	require.Equal(t, "#ff0000", showing(t, server, "Keychron K4 HE")[0].String())

	done, err := svc.ApplyScene(t.Context(), "off")
	require.NoError(t, err)
	require.NotEmpty(t, done.Results)
	require.Equal(t, "#000000", showing(t, server, "Keychron K4 HE")[0].String())
}

func TestTurningOffIsNotRememberedAsAColour(t *testing.T) {
	// A device deliberately turned off has nothing to restore: putting black
	// back at boot is not what anybody meant by "off".
	svc, _ := lit(t)
	_, err := svc.ApplyScene(t.Context(), "red")
	require.NoError(t, err)
	_, err = svc.ApplyScene(t.Context(), "off")
	require.NoError(t, err)

	require.True(t, svc.Desired().Empty(), "an off scene was recorded as a colour to restore")
}

func TestABindingToASceneThatIsNotThereSaysSoBeforeItIsPressed(t *testing.T) {
	// Otherwise the first anybody hears of it is a key that does nothing,
	// which is indistinguishable from the key not being registered at all.
	svc, _ := lit(t)
	require.NoError(t, svc.SaveScene(scenes.Scene{Name: "evening", Colour: "blue"}))
	require.NoError(t, svc.Bind("Ctrl+Alt+Shift+Num+1", "evening"))
	require.NoError(t, svc.DeleteScene("evening"))

	keys, err := svc.Keys()
	require.NoError(t, err)
	var missing bool
	for _, binding := range keys.Bindings {
		if binding.Key == "Ctrl+Alt+Shift+Num+1" {
			missing = binding.Missing
		}
	}
	require.True(t, missing)
}

func TestBindingAKeyToANonexistentSceneIsRefused(t *testing.T) {
	svc, _ := lit(t)
	require.ErrorContains(t, svc.Bind("Ctrl+Alt+Shift+Num+2", "nothing-like-this"), "no scene")
}

func TestAKeypressTakesTheSamePathAsTheApiCall(t *testing.T) {
	// One flow, two doors. A keypress that went another way would be a
	// second implementation of applying a scene.
	svc, server := lit(t)

	said, err := svc.ApplyByName(t.Context(), "green")
	require.NoError(t, err)
	require.Contains(t, said, "green")
	require.Equal(t, "#00ff00", showing(t, server, "Keychron K4 HE")[0].String())
	require.Contains(t, svc.Desired().Devices, "Keychron K4 HE")
}

func TestAKeypressForASceneThatIsGoneReportsRatherThanCrashing(t *testing.T) {
	svc, _ := lit(t)
	_, err := svc.ApplyByName(t.Context(), "never-existed")
	require.Error(t, err)
}

func TestASceneAppliesOnAMachineWhoseScreenCannotBeDrawnOn(t *testing.T) {
	/*
		A cooler whose panel will not open -- a model without one, a usbfs
		node this user may not claim, another program holding the interface
		-- is a machine with working lights and nothing drawn, which is the
		degradation rule in spec 012. Reporting it as a failed scene would
		put a permanent complaint on every scene on that machine, and the
		shipped scenes all name a screen state.
	*/
	gif := filepath.Join(t.TempDir(), "rain.gif")
	require.NoError(t, os.WriteFile(gif, []byte("GIF89a"), 0o600))

	scene := blue()
	scene.Screen = gif
	svc, _ := lit(t, scene)
	svc.SetCooler(&panel{wrong: hw.ErrNoScreen, refuse: hw.ErrNoScreen})
	svc.SetDashboard(&screenAuthor{})

	done, err := svc.ApplyScene(t.Context(), "blue")
	require.NoError(t, err)
	require.Empty(t, done.Problems, "a screen that cannot be drawn on was reported as a fault")
	require.True(t, done.Results[0].Applied, "the lights did not change")
}

func TestAStyleIsGivenToOtherScenesWithoutTheirColours(t *testing.T) {
	/*
		A style and a colour are different things: a scene names a colour per
		light and a mode per device. Somebody who decides their keyboard
		should be reactive has decided that about the keyboard rather than
		about one scene, and saying so across a bank of nine meant editing
		nine scenes.
	*/
	svc, _ := lit(t,
		scenes.Scene{
			Name: "evening", Colour: "blue",
			Effects: map[string]string{"Keychron": "Typing Heatmap"},
		},
		scenes.Scene{Name: "red", Colour: "red"},
		scenes.Scene{
			Name: "green", Colour: "green",
			Effects: map[string]string{"Keychron": "Direct", "Kraken": "Breathing"},
		},
	)

	changed, err := svc.CopyEffects("evening", []string{"red", "green"})
	require.NoError(t, err)
	require.ElementsMatch(t, []string{"red", "green"}, changed)

	for _, name := range []string{"red", "green"} {
		scene, err := svc.Scene(name)
		require.NoError(t, err)
		require.Equal(t, map[string]string{"Keychron": "Typing Heatmap"}, scene.Effects,
			"%s did not take the style", name)
	}

	// The colours are untouched, which is the whole point.
	red, err := svc.Scene("red")
	require.NoError(t, err)
	require.Equal(t, "red", red.Colour)

	// And it replaces rather than merges: green's second effect is gone,
	// because "these scenes now look like that one" has to be true of every
	// device rather than of some of them.
	green, err := svc.Scene("green")
	require.NoError(t, err)
	require.NotContains(t, green.Effects, "Kraken")
}

func TestAStyleGoesNowhereWhenAScenesNameIsWrong(t *testing.T) {
	// Every target is read before any is written: a name that is not there
	// costs nothing rather than leaving half a bank restyled.
	svc, _ := lit(t,
		scenes.Scene{Name: "evening", Effects: map[string]string{"Keychron": "Splash"}},
		scenes.Scene{Name: "red", Colour: "red"},
	)

	_, err := svc.CopyEffects("evening", []string{"red", "nothing-called-this"})
	require.Error(t, err)

	red, err := svc.Scene("red")
	require.NoError(t, err)
	require.Empty(t, red.Effects, "a scene was restyled before the run failed")
}
