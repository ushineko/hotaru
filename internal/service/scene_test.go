package service_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/ushineko/hotaru/internal/canvas/canvastest"
	"github.com/ushineko/hotaru/internal/colour"
	hw "github.com/ushineko/hotaru/internal/cooler" // `cooler` is a fixture in this package
	"github.com/ushineko/hotaru/internal/openrgb"
	"github.com/ushineko/hotaru/internal/scenes"
	"github.com/ushineko/hotaru/internal/service"
	"github.com/ushineko/hotaru/internal/service/servicetest"
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

// sceneDesk is the board and keyboard litCanvas draws on.
type sceneDesk struct {
	board *canvastest.Board
	kb    *canvastest.Firmware
}

// litCanvas is lit on canvases (spec 076): the board and the keyboard as
// sanshoku gives them, with no OpenRGB server.
func litCanvas(t *testing.T, saved ...scenes.Scene) (*service.Service, *sceneDesk) {
	t.Helper()
	desk := &sceneDesk{board: auraBoard(), kb: k4()}
	svc := servicetest.New(t, nil, desk.board, desk.kb)

	desired, err := state.Open(filepath.Join(t.TempDir(), "state.yml"))
	require.NoError(t, err)
	svc.SetRecorder(desired)

	store, err := scenes.Open(filepath.Join(t.TempDir(), "scenes.yml"))
	require.NoError(t, err)
	for _, scene := range saved {
		require.NoError(t, store.Save(scene))
	}
	svc.SetScenes(store)
	return svc, desk
}

// firmwareGiven is the colour a canvas's firmware runs effect in, failing
// where it does not run it.
func firmwareGiven(t *testing.T, f *canvastest.Firmware, effect string) string {
	t.Helper()
	running, c := f.Running()
	require.Equal(t, effect, running, "the firmware was not put in the effect")
	return colour.Colour{R: c.R, G: c.G, B: c.B}.String()
}

// lightIs waits for a fake's light (by key id, from 1) to show hex.
func lightIs(t *testing.T, d *canvastest.Device, id byte, hex string, msg ...any) {
	t.Helper()
	want := colour.MustParse(hex)
	require.Eventually(t, func() bool {
		p, ok := d.Showing(id)
		return ok && p.R == want.R && p.G == want.G && p.B == want.B
	}, eventually, every, msg...)
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
	svc, _ := litCanvas(t, blue())

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
	svc, desk := litCanvas(t, scene)

	_, err := svc.ApplyScene(t.Context(), "blue")
	require.NoError(t, err)

	// Key 1 is the board's own light; the header's first is key 2.
	lightIs(t, desk.board.Device, 2, "#ff0000")
	lightIs(t, desk.board.Device, 3, "#0000ff")
}

func TestAnEffectIsPreferredForItsOwnDevice(t *testing.T) {
	// A scene wants the keyboard doing one thing and the board another, which
	// is why an effect is named per device rather than per request.
	scene := blue()
	scene.Effects = map[string]scenes.Effect{"Keychron": {Mode: "Solid Color"}}
	svc, _ := litCanvas(t, scene)

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
	scene.Effects = map[string]scenes.Effect{"Keychron": {Mode: "Storm"}}
	svc, _ := litCanvas(t, scene)

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
	svc, _ := litCanvas(t, blue())

	_, err := svc.ApplyScene(t.Context(), "blue")
	require.NoError(t, err)

	desired := svc.Desired()
	require.Contains(t, desired.Devices, "Keychron K4 HE")
}

func TestPreviewingASceneIsNotRemembered(t *testing.T) {
	// A preview is what somebody is looking at, never what they want.
	svc, _ := litCanvas(t, blue())

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
	svc, _ := litCanvas(t, blue())
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
	svc, desk := litCanvas(t, blue())
	_, err := svc.ApplyScene(t.Context(), "blue")
	require.NoError(t, err)

	draft := blue()
	draft.Name = "draft"
	draft.Assignments = []scenes.Assignment{{Target: "Keychron", Colour: "red"}}
	require.NoError(t, svc.SaveScene(t.Context(), draft))

	held, err := svc.PreviewScene(t.Context(), "draft", "a test", false)
	require.NoError(t, err)
	lightIs(t, desk.kb.Device, 1, "#ff0000")

	_, err = svc.Release(t.Context(), held.Lease.Token)
	require.NoError(t, err)
	lightIs(t, desk.kb.Device, 1, "#0000ff", "ending the preview left the draft on the hardware")
}

func TestALapsedPreviewIsEndedByTheLoopThatWouldHaveCorrectedIt(t *testing.T) {
	/*
		The holder stopped talking. Nothing else will put this back: the
		correction that would have is suspended on that holder's behalf, which
		is exactly why the lease has to lapse.
	*/
	svc, desk := litCanvas(t, blue())
	_, err := svc.ApplyScene(t.Context(), "blue")
	require.NoError(t, err)

	draft := scenes.Scene{Name: "draft", Assignments: []scenes.Assignment{{Target: "Keychron", Colour: "red"}}}
	require.NoError(t, svc.SaveScene(t.Context(), draft))
	held, err := svc.PreviewScene(t.Context(), "draft", "a client that died", false)
	require.NoError(t, err)

	// Nobody renews it. Reaching into the lease is the one thing a test can
	// do that waiting ten seconds also does.
	held.Lease.Expires = time.Now().Add(-time.Second)

	restore, err := svc.Expired(t.Context())
	require.NoError(t, err)
	require.Equal(t, 1, restore.Applied)
	lightIs(t, desk.kb.Device, 1, "#0000ff")
}

func TestAConnectionBoundLeaseDoesNotLapse(t *testing.T) {
	// Its holder is sitting on an open request, and the socket closing is a
	// better signal than any clock. Nothing should expire it in the meantime.
	svc, _ := litCanvas(t, blue())

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
	svc, _ := litCanvas(t, blue())
	_, err := svc.PreviewScene(t.Context(), "blue", "the first client", false)
	require.NoError(t, err)

	_, err = svc.PreviewScene(t.Context(), "blue", "the second client", false)
	require.ErrorContains(t, err, "the first client")
}

func TestRenewingKeepsALeaseAlive(t *testing.T) {
	svc, _ := litCanvas(t, blue())
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
	svc, _ := litCanvas(t, blue())
	require.ErrorContains(t, svc.Renew("nothing"), "expired")
}

func TestASceneSaysWhatItDidToTheScreen(t *testing.T) {
	// A scene that says nothing about the screen changes nothing about it.
	svc, _ := litCanvas(t, blue())

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
	svc, _ := litCanvas(t, scene)
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
	svc, _ := litCanvas(t, scene)
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
	svc, _ := litCanvas(t, blue())
	_, err := svc.ApplyScene(t.Context(), "blue")
	require.NoError(t, err)

	captured, err := svc.SceneFrom("kept", scenes.ScreenDashboard)
	require.NoError(t, err)
	require.Len(t, captured.Assignments, 2)
	require.Equal(t, "#0000ff", captured.Assignments[0].Colour)
	require.Equal(t, scenes.ScreenDashboard, captured.Screen)
}

func TestCapturingNothingSaysSo(t *testing.T) {
	svc, _ := litCanvas(t)
	_, err := svc.SceneFrom("empty", "")
	require.ErrorContains(t, err, "nothing is recorded")
}

func TestAMachineWithNoScenesFileSaysSoRatherThanFailingQuietly(t *testing.T) {
	// An unreadable scenes file is somebody's saved work, so the service runs
	// without scenes rather than replacing them.
	svc := servicetest.New(t, nil, auraBoard())

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
	svc, _ := litCanvas(t)

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
	svc, desk := litCanvas(t)
	_, err := svc.ApplyScene(t.Context(), "red")
	require.NoError(t, err)
	lightIs(t, desk.kb.Device, 1, "#ff0000")

	done, err := svc.ApplyScene(t.Context(), "off")
	require.NoError(t, err)
	require.NotEmpty(t, done.Results)
	lightIs(t, desk.kb.Device, 1, "#000000")
}

func TestTurningOffIsNotRememberedAsAColour(t *testing.T) {
	// A device deliberately turned off has nothing to restore: putting black
	// back at boot is not what anybody meant by "off".
	svc, _ := litCanvas(t)
	_, err := svc.ApplyScene(t.Context(), "red")
	require.NoError(t, err)
	_, err = svc.ApplyScene(t.Context(), "off")
	require.NoError(t, err)

	require.True(t, svc.Desired().Empty(), "an off scene was recorded as a colour to restore")
}

func TestABindingToASceneThatIsNotThereSaysSoBeforeItIsPressed(t *testing.T) {
	// Otherwise the first anybody hears of it is a key that does nothing,
	// which is indistinguishable from the key not being registered at all.
	svc, _ := litCanvas(t)
	require.NoError(t, svc.SaveScene(t.Context(), scenes.Scene{Name: "evening", Colour: "blue"}))
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
	svc, _ := litCanvas(t)
	require.ErrorContains(t, svc.Bind("Ctrl+Alt+Shift+Num+2", "nothing-like-this"), "no scene")
}

func TestAKeypressTakesTheSamePathAsTheApiCall(t *testing.T) {
	// One flow, two doors. A keypress that went another way would be a
	// second implementation of applying a scene.
	svc, desk := litCanvas(t)

	said, err := svc.ApplyByName(t.Context(), "green")
	require.NoError(t, err)
	require.Contains(t, said, "green")
	lightIs(t, desk.kb.Device, 1, "#00ff00")
	require.Contains(t, svc.Desired().Devices, "Keychron K4 HE")
}

func TestAKeypressForASceneThatIsGoneReportsRatherThanCrashing(t *testing.T) {
	svc, _ := litCanvas(t)
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
	svc, _ := litCanvas(t, scene)
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
	svc, _ := litCanvas(t,
		scenes.Scene{
			Name: "evening", Colour: "blue",
			Effects: map[string]scenes.Effect{"Keychron": {Mode: "Typing Heatmap"}},
		},
		scenes.Scene{Name: "red", Colour: "red"},
		scenes.Scene{
			Name: "green", Colour: "green",
			Effects: map[string]scenes.Effect{"Keychron": {Mode: "Direct"}, "Kraken": {Mode: "Breathing"}},
		},
	)

	changed, err := svc.CopyEffects("evening", []string{"red", "green"})
	require.NoError(t, err)
	require.ElementsMatch(t, []string{"red", "green"}, changed)

	for _, name := range []string{"red", "green"} {
		scene, err := svc.Scene(name)
		require.NoError(t, err)
		require.Equal(t, map[string]scenes.Effect{"Keychron": {Mode: "Typing Heatmap"}}, scene.Effects,
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
	svc, _ := litCanvas(t,
		scenes.Scene{Name: "evening", Effects: map[string]scenes.Effect{"Keychron": {Mode: "Splash"}}},
		scenes.Scene{Name: "red", Colour: "red"},
	)

	_, err := svc.CopyEffects("evening", []string{"red", "nothing-called-this"})
	require.Error(t, err)

	red, err := svc.Scene("red")
	require.NoError(t, err)
	require.Empty(t, red.Effects, "a scene was restyled before the run failed")
}

func TestSavingTheSceneOnTheMachineLightsIt(t *testing.T) {
	/*
		Editing the scene that is showing, saving it, and watching the lights
		stay as they were. Desired state was written by the apply, the file
		by the save, and nothing joined the two -- so the remedy was to apply
		another scene and come back, which every user found for themselves.

		Asserted on the device's own frame rather than on the outcome: what
		is being fixed is what the devices are showing, and an outcome that
		said "applied" while the board stayed blue is exactly the report this
		bug hid behind.
	*/
	svc, desk := litCanvas(t, blue())

	_, err := svc.ApplyScene(t.Context(), "blue")
	require.NoError(t, err)
	lightIs(t, desk.board.Device, 1, "#0000ff")

	edited := blue()
	edited.Assignments[0].Colour = "red"
	require.NoError(t, svc.SaveScene(t.Context(), edited))

	lightIs(t, desk.board.Device, 1, "#ff0000", "saving over the scene on the machine did not light it")
}

func TestSavingTheSceneOnTheMachineLightsItAfterARestart(t *testing.T) {
	/*
		The same edit, after the service restarted in between (#183).

		Which scene was applied lived in memory only. After a restart --
		every boot -- saving the scene on the machine wrote the file and
		left the lights as they were, so an edit took a save and then an
		apply. The name is kept with the desired state, in the same file,
		and a new service reads it back.
	*/
	statePath := filepath.Join(t.TempDir(), "state.yml")
	saved, err := scenes.Open(filepath.Join(t.TempDir(), "scenes.yml"))
	require.NoError(t, err)
	require.NoError(t, saved.Save(blue()))

	start := func() (*service.Service, *state.Store, *canvastest.Board) {
		desired, err := state.Open(statePath)
		require.NoError(t, err)
		board := auraBoard()
		svc := servicetest.New(t, nil, board, k4())
		svc.SetRecorder(desired)
		svc.SetScenes(saved)
		return svc, desired, board
	}

	before, desired, _ := start()
	_, err = before.ApplyScene(t.Context(), "blue")
	require.NoError(t, err)
	// As stopping the service does: what the next one reads is the file.
	require.NoError(t, desired.Flush())

	after, _, board := start()
	require.Equal(t, "blue", after.Applied(), "the restarted service forgot which scene was applied")

	edited := blue()
	edited.Assignments[0].Colour = "red"
	require.NoError(t, after.SaveScene(t.Context(), edited))
	lightIs(t, board.Device, 1, "#ff0000", "saving the scene on the machine after a restart did not light it")
}

func TestSavingASceneNobodyIsShowingLeavesTheMachineAlone(t *testing.T) {
	// A draft saved for later must not take the machine: the scene showing
	// is the one somebody asked for, and this is not it.
	svc, desk := litCanvas(t, blue())

	_, err := svc.ApplyScene(t.Context(), "blue")
	require.NoError(t, err)

	later := scenes.Scene{Name: "for later", Assignments: []scenes.Assignment{
		{Target: "ASUS", Colour: "red"},
	}}
	require.NoError(t, svc.SaveScene(t.Context(), later))

	lightIs(t, desk.board.Device, 1, "#0000ff")
	time.Sleep(30 * time.Millisecond)
	lightIs(t, desk.board.Device, 1, "#0000ff", "saving a scene nobody asked for took the machine")

	// And nothing has been applied but the one that was.
	require.Equal(t, "blue", svc.Applied())
}

func TestAnEffectSurvivesAFrameItCannotShow(t *testing.T) {
	/*
		The bug this is here for: a scene with a colour per key and a reactive
		effect on the keyboard lit every key, reported the device as applied,
		and left the keyboard in Direct with the effect nowhere -- because
		resolution read the frame first and dropped any mode that could not
		carry every colour in it.

		An effect is a decision about the device, not a preference about how
		to show the colours, so the frame is what gives way.
	*/
	scene := blue()
	scene.Assignments = append(scene.Assignments,
		scenes.Assignment{Target: "Keychron/Keyboard[1]", Colour: "red"})
	scene.Effects = map[string]scenes.Effect{"Keychron": {Mode: "Solid Reactive"}}
	svc, _ := litCanvas(t, scene)

	done, err := svc.ApplyScene(t.Context(), "blue")
	require.NoError(t, err)

	var found bool
	for _, result := range done.Results {
		if result.Device != "Keychron K4 HE" {
			continue
		}
		found = true
		require.True(t, result.Applied)
		require.Equal(t, "Solid Reactive", result.Mode,
			"the effect was dropped for a frame of more than one colour")
	}
	require.True(t, found, "the keyboard was not in the outcome at all")
}

func TestAnEffectIsLitInTheColourTheSceneMostlyIs(t *testing.T) {
	/*
		A mode that keeps its colour in the mode cannot show a frame of many,
		and the alternative to reducing the frame is the colour the vendor
		left behind -- which is neither what the scene says nor anything
		anybody picked.
	*/
	scene := blue()
	scene.Assignments = append(scene.Assignments,
		scenes.Assignment{Target: "Keychron/Keyboard[1]", Colour: "red"})
	scene.Effects = map[string]scenes.Effect{"Keychron": {Mode: "Solid Reactive"}}
	svc, desk := litCanvas(t, scene)

	_, err := svc.ApplyScene(t.Context(), "blue")
	require.NoError(t, err)

	require.Equal(t, "#0000ff", firmwareGiven(t, desk.kb, "Solid Reactive"),
		"the effect was not lit in the colour most of the scene is")
}

func TestAModeIsStillNotForcedOverAFrameItCannotShow(t *testing.T) {
	// The rule an effect is an exception to, held: --mode is a preference
	// about how to show the colours, so a mode that cannot show them is no
	// use and resolution carries on past it.
	svc, _ := lit(t, blue())

	results, err := svc.Apply(t.Context(), service.Request{
		Assignments: solid("Keychron/Keyboard[0]", "blue"),
		Devices:     []string{"Keychron"},
		Mode:        "Solid Reactive",
	})
	require.NoError(t, err)
	require.Len(t, results, 1)
	require.Equal(t, "Direct", results[0].Mode)
}

func TestAnEffectIsLitInTheColourTheSceneNames(t *testing.T) {
	// The reduction is the fallback, never the answer: somebody who has
	// decided their keyboard ripples red has said so, and the colour most of
	// the frame is has nothing to do with it.
	scene := blue()
	scene.Effects = map[string]scenes.Effect{
		"Keychron": {Mode: "Solid Reactive", Colour: "#ff0000"},
	}
	svc, desk := litCanvas(t, scene)

	_, err := svc.ApplyScene(t.Context(), "blue")
	require.NoError(t, err)

	require.Equal(t, "#ff0000", firmwareGiven(t, desk.kb, "Solid Reactive"),
		"the scene's own colour lost to the frame's")
}

func TestAnEffectWithNoColourStillTakesTheFrames(t *testing.T) {
	scene := blue()
	scene.Effects = map[string]scenes.Effect{"Keychron": {Mode: "Solid Reactive"}}
	svc, desk := litCanvas(t, scene)

	_, err := svc.ApplyScene(t.Context(), "blue")
	require.NoError(t, err)

	require.Equal(t, "#0000ff", firmwareGiven(t, desk.kb, "Solid Reactive"))
}

func TestAnEffectsSpeedIsWritten(t *testing.T) {
	speed := 40
	scene := blue()
	scene.Effects = map[string]scenes.Effect{"Keychron": {Mode: "Solid Reactive", Speed: &speed}}
	svc, server := lit(t, scene)

	_, err := svc.ApplyScene(t.Context(), "blue")
	require.NoError(t, err)

	written := lastMode(t, server, "Keychron K4 HE", "Solid Reactive").Speed
	require.NotNil(t, written, "the speed was not sent")
	require.Equal(t, 40, *written)
}

func TestAnEffectsColourIsUnreadableIsSaidRatherThanIgnored(t *testing.T) {
	// An effect quietly falling back to the frame because of a typo looks
	// exactly like one that was never given a colour.
	scene := blue()
	scene.Effects = map[string]scenes.Effect{
		"Keychron": {Mode: "Solid Reactive", Colour: "puce"},
	}
	svc, _ := litCanvas(t, scene)

	done, err := svc.ApplyScene(t.Context(), "blue")
	require.NoError(t, err)

	var said []string
	for _, result := range done.Results {
		if result.Device == "Keychron K4 HE" {
			require.True(t, result.Applied, "the scene was dropped with the colour")
			said = result.Problems
		}
	}
	require.NotEmpty(t, said)
	require.Contains(t, said[0], "puce")
}

// lastMode is the most recent mode write to a device, for asserting what was
// put into the mode rather than into the buffer.
func lastMode(t *testing.T, server *openrgb.Fake, device, mode string) openrgb.ModeWrite {
	t.Helper()
	var found *openrgb.ModeWrite
	for i := range server.Modes {
		if server.Modes[i].Device == device && server.Modes[i].Mode == mode {
			found = &server.Modes[i]
		}
	}
	require.NotNil(t, found, "%s was never put into %s", device, mode)
	return *found
}

func TestAModeThatShowsOneColourIsAssertedAgainAfterTheFrame(t *testing.T) {
	/*
		Coming from a per-LED mode, the first mode packet does not stick on
		real hardware: a keyboard in Direct sent Solid Reactive, its colour and
		then a hundred per-key colours displayed the keys, while every
		read-back said the mode was active and held the colour it was given.
		Going from one such mode to another was always right, which is what
		says it is the transition rather than the buffer.
	*/
	scene := blue()
	scene.Effects = map[string]scenes.Effect{"Keychron": {Mode: "Solid Reactive"}}
	svc, server := lit(t, scene)

	// From a per-LED mode, which is the transition that loses it.
	require.NoError(t, server.SetMode(t.Context(), "Keychron K4 HE", "Direct", openrgb.Style{}))
	before := len(server.Modes)

	_, err := svc.ApplyScene(t.Context(), "blue")
	require.NoError(t, err)

	var asserted int
	for _, write := range server.Modes[before:] {
		if write.Device == "Keychron K4 HE" && write.Mode == "Solid Reactive" {
			asserted++
		}
	}
	require.Equal(t, 2, asserted,
		"the mode was not asserted again after the frame, so the transition loses its colour")
}

func TestAPerLEDModeIsWrittenOnce(t *testing.T) {
	// The extra packet is for the modes that cannot show a frame. A device in
	// Direct is showing the frame, and sending its mode twice is two writes
	// where one says everything.
	svc, server := lit(t, blue())

	_, err := svc.ApplyScene(t.Context(), "blue")
	require.NoError(t, err)

	var written int
	for _, write := range server.Modes {
		if write.Device == "Keychron K4 HE" && write.Mode == "Direct" {
			written++
		}
	}
	require.Equal(t, 1, written)
}

/*
And the scene's own colours are still what is remembered.

The buffer is what the device is shown while a mode that cannot display those
colours is running; desired state is what somebody asked for, and is what the
lights go back to when the effect comes off.
*/
func TestTheSceneKeepsItsColoursWhileTheEffectShowsItsOwn(t *testing.T) {
	svc, _ := litCanvas(t, scenes.Scene{
		Name:        "evening",
		Assignments: []scenes.Assignment{{Target: "Keychron", Colour: "#8c6f6a"}},
		Effects: map[string]scenes.Effect{
			"Keychron": {Mode: "Solid Reactive", Colour: "#67798c"},
		},
	})

	_, err := svc.ApplyScene(context.Background(), "evening")
	require.NoError(t, err)

	kept := svc.Desired().Devices["Keychron K4 HE"]
	require.NotEmpty(t, kept.Colours)
	require.Equal(t, "#8c6f6a", kept.Colours[0].String(),
		"the scene's colours were overwritten by the effect's")
	require.Equal(t, "#67798c", kept.ModeColour, "and the effect's colour is kept too")
}

/*
A per-LED mode is untouched by any of this: the buffer is what it shows, and
a scene's colours are what go in it.
*/
func TestTheBufferHoldsTheScenesOwnColours(t *testing.T) {
	svc, desk := litCanvas(t, scenes.Scene{
		Name:        "evening",
		Assignments: []scenes.Assignment{{Target: "Keychron", Colour: "#8c6f6a"}},
	})

	_, err := svc.ApplyScene(context.Background(), "evening")
	require.NoError(t, err)

	lightIs(t, desk.kb.Device, 1, "#8c6f6a")
}

/*
The mode goes again after the frame, once the device has entered it.

The packet carries the mode and its colour together and the device takes them
as two operations, so the second one is the already-in-the-mode case and is
what makes the colour stick. It only works with a gap before it, which is what
spec 057 measured; the frame's position between them turned out not to matter
at all.
*/
func TestAModeWithAColourOfItsOwnIsWrittenAgainAfterTheFrame(t *testing.T) {
	svc, server := lit(t, scenes.Scene{
		Name:        "evening",
		Assignments: []scenes.Assignment{{Target: "Keychron", Colour: "#8c6f6a"}},
		Effects: map[string]scenes.Effect{
			"Keychron": {Mode: "Solid Reactive", Colour: "#67798c"},
		},
	})

	_, err := svc.ApplyScene(context.Background(), "evening")
	require.NoError(t, err)

	require.Equal(t,
		[]string{"mode:Solid Reactive", "frame", "mode:Solid Reactive"},
		server.Sequence,
		"the mode goes again after the frame, once the device has entered it")
}

/*
A per-LED mode keeps the order it has always had: the mode, then the frame.

A device arriving in the mode takes the packet first (spec 068). Nothing
measured about the keyboard changes that, and this is here so the reordering
cannot quietly spread to it.
*/
func TestAPerLEDModeTakesTheModeBeforeTheFrame(t *testing.T) {
	svc, server := lit(t, scenes.Scene{
		Name:        "evening",
		Assignments: []scenes.Assignment{{Target: "Keychron", Colour: "#8c6f6a"}},
	})

	_, err := svc.ApplyScene(context.Background(), "evening")
	require.NoError(t, err)
	require.Equal(t, []string{"mode:Direct", "frame"}, server.Sequence)
}
