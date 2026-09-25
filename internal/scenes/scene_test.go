package scenes_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/ushineko/hotaru/internal/colour"
	"github.com/ushineko/hotaru/internal/scenes"
)

func evening() scenes.Scene {
	return scenes.Scene{
		Name: "evening",
		Assignments: []scenes.Assignment{
			{Target: "kraken", Colour: "#8000ff"},
			{Target: "kraken/fan-top", Colour: "red"},
		},
		Effects: map[string]scenes.Effect{"keychron": {Mode: "Solid Splash"}},
		Screen:  scenes.ScreenDashboard,
	}
}

func store(t *testing.T) *scenes.Store {
	t.Helper()
	s, err := scenes.Open(filepath.Join(t.TempDir(), "scenes.yml"))
	require.NoError(t, err)
	return s
}

func TestAnAssignmentKeepsItsOrder(t *testing.T) {
	// "Everything blue except the top fan" is two lines, and it only works
	// because the second one is applied after the first.
	got, problems := evening().Resolve()

	require.Empty(t, problems)
	require.Len(t, got, 2)
	require.Equal(t, "kraken", got[0].Target.String())
	require.Equal(t, "kraken/fan-top", got[1].Target.String())
	require.Equal(t, colour.MustParse("red"), got[1].Colour)
}

func TestOneBadLineCostsOneLine(t *testing.T) {
	/*
		A scene is not all-or-nothing. An unknown segment already costs its
		own assignment rather than the whole scene, and a colour nobody can
		read is the same kind of mistake.
	*/
	scene := scenes.Scene{Assignments: []scenes.Assignment{
		{Target: "kraken", Colour: "#8000ff"},
		{Target: "kraken", Colour: "puce"},
		{Target: "", Colour: "red"},
	}}

	got, problems := scene.Resolve()
	require.Len(t, got, 1)
	require.Len(t, problems, 2)
	require.Contains(t, problems[0].Error(), "assignment 2")
}

func TestAnEffectIsFoundByTheNameSomebodyWouldType(t *testing.T) {
	// The device's full name is the vendor's string. A scene says "keychron".
	require.Equal(t, "Solid Splash", evening().Effect("Keychron K4 HE").Mode)
	require.Empty(t, evening().Effect("NZXT Kraken Elite V2"))
}

func TestASceneKnowsWhatItTouches(t *testing.T) {
	// What a preview takes a lease on: devices named by an assignment or by
	// an effect, and nothing else.
	require.Equal(t, []string{"keychron", "kraken"}, evening().Devices())
}

func TestScenesRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "scenes.yml")
	first, err := scenes.Open(path)
	require.NoError(t, err)
	require.NoError(t, first.Save(evening()))

	again, err := scenes.Open(path)
	require.NoError(t, err)
	got, err := again.Get("evening")
	require.NoError(t, err)
	require.Equal(t, evening(), got)
}

func TestAMalformedFileIsReportedRatherThanReplaced(t *testing.T) {
	/*
		Machine state can be discarded on a bad read; somebody's named scenes
		cannot. A typo in a hand edit must not delete the file, least of all
		silently and at the moment they most want it back.
	*/
	path := filepath.Join(t.TempDir(), "scenes.yml")
	broken := "scenes:\n  evening:\n    assignments: [ unclosed\n"
	require.NoError(t, os.WriteFile(path, []byte(broken), 0o600))

	_, err := scenes.Open(path)
	require.Error(t, err)

	after, readErr := os.ReadFile(path) //nolint:gosec // the test's own temp file
	require.NoError(t, readErr)
	require.Equal(t, broken, string(after), "the unreadable file was overwritten")
}

func TestAnUnambiguousPrefixIsEnough(t *testing.T) {
	s := store(t)
	require.NoError(t, s.Save(evening()))
	require.NoError(t, s.Save(scenes.Scene{Name: "work"}))

	got, err := s.Get("even")
	require.NoError(t, err)
	require.Equal(t, "evening", got.Name)
}

func TestAnAmbiguousNameSaysWhatItMatched(t *testing.T) {
	s := store(t)
	require.NoError(t, s.Save(scenes.Scene{Name: "evening"}))
	require.NoError(t, s.Save(scenes.Scene{Name: "even-dimmer"}))

	_, err := s.Get("even")
	require.ErrorContains(t, err, "even-dimmer, evening")
}

func TestDeletingSomethingAbsentIsNotAFailure(t *testing.T) {
	// The caller wanted it gone and it is gone.
	require.NoError(t, store(t).Delete("never-existed"))
}

func TestASceneNeedsAName(t *testing.T) {
	require.ErrorContains(t, store(t).Save(scenes.Scene{}), "needs a name")
}

func TestListingIsSorted(t *testing.T) {
	s := store(t)
	require.NoError(t, s.Save(scenes.Scene{Name: "work"}))
	require.NoError(t, s.Save(scenes.Scene{Name: "aardvark"}))

	all := s.All()
	require.Equal(t, "aardvark", all[0].Name)
	require.Equal(t, "work", all[len(all)-1].Name)
}

func TestTheShippedBankIsThereBeforeAnybodySavesAnything(t *testing.T) {
	/*
		Nine scenes and no file. They are what this desk's numpad has meant
		for two years, read out of the program being replaced rather than
		invented, and a machine that has never run hotaru has them without
		hotaru having written anything.
	*/
	names := map[string]bool{}
	for _, scene := range store(t).All() {
		require.True(t, scene.Shipped)
		names[scene.Name] = true
	}
	require.Len(t, names, 9)
	require.True(t, names["red"])
	require.True(t, names["off"])
}

func TestSavingOverAShippedNameReplacesItUntilItIsDeleted(t *testing.T) {
	// Somebody who wants a different red should get their red, and should be
	// able to change their mind without hotaru having lost the original.
	s := store(t)
	require.NoError(t, s.Save(scenes.Scene{Name: "red", Colour: "#400000"}))

	mine, err := s.Get("red")
	require.NoError(t, err)
	require.Equal(t, "#400000", mine.Colour)
	require.False(t, mine.Shipped)

	require.NoError(t, s.Delete("red"))
	back, err := s.Get("red")
	require.NoError(t, err)
	require.Equal(t, "red", back.Colour)
	require.True(t, back.Shipped)
}

func TestTheShippedKeysAreTheMonitorsKeys(t *testing.T) {
	// The bank somebody's hands already know. Changing what Ctrl+Alt+Num4
	// does is breaking something no test would otherwise catch.
	keys := store(t).Bindings()
	require.Len(t, keys, 9)
	require.Equal(t, "red", keys["Ctrl+Alt+Num+1"])
	require.Equal(t, "purple", keys["Ctrl+Alt+Num+4"])
	require.Equal(t, "off", keys["Ctrl+Alt+Num+9"])

	for _, reserved := range scenes.Reserved() {
		require.NotContains(t, keys, reserved,
			"hotaru bound a key it reserves for somebody else's scenes")
	}
}

func TestRebindingOneKeyLeavesTheOthersAlone(t *testing.T) {
	s := store(t)
	require.NoError(t, s.Bind("Ctrl+Alt+Num+4", "evening"))

	keys := s.Bindings()
	require.Equal(t, "evening", keys["Ctrl+Alt+Num+4"])
	require.Equal(t, "red", keys["Ctrl+Alt+Num+1"])
}

func TestAShippedKeyCanBeUnbound(t *testing.T) {
	/*
		Recorded rather than forgotten: an unbinding has to survive a restart,
		and the shipped bank is merged in on every read, so "this key is
		deliberately nothing" is a thing the file must be able to say.
	*/
	s := store(t)
	require.NoError(t, s.Bind("Ctrl+Alt+Num+9", ""))
	require.NotContains(t, s.Bindings(), "Ctrl+Alt+Num+9")

	again, err := scenes.Open(s.Path())
	require.NoError(t, err)
	require.NotContains(t, again.Bindings(), "Ctrl+Alt+Num+9")
}

func TestABindingNeedsAKey(t *testing.T) {
	require.ErrorContains(t, store(t).Bind("", "red"), "needs a key")
}

func TestASceneWrittenBeforeEffectsHadSettingsStillReads(t *testing.T) {
	/*
		Every scene on disk names its effects as a mode and nothing else. A
		file hotaru cannot read back is a file somebody loses work to, which
		is why the store reports a bad parse rather than replacing it -- and
		why the short form is read first here.
	*/
	var effect scenes.Effect
	require.NoError(t, json.Unmarshal([]byte(`"Solid Reactive Multinexus"`), &effect))
	require.Equal(t, "Solid Reactive Multinexus", effect.Mode)
	require.Empty(t, effect.Colour)
	require.Nil(t, effect.Speed)
}

func TestAnEffectWithSettingsReadsAsAMapping(t *testing.T) {
	var effect scenes.Effect
	require.NoError(t, json.Unmarshal(
		[]byte(`{"mode":"Solid Reactive","colour":"#0000ff","speed":127}`), &effect))
	require.Equal(t, "Solid Reactive", effect.Mode)
	require.Equal(t, "#0000ff", effect.Colour)
	require.NotNil(t, effect.Speed)
	require.Equal(t, 127, *effect.Speed)
}

func TestAModeOnItsOwnIsWrittenOnItsOwn(t *testing.T) {
	// The file is read by people. A mode's name is a line; a mapping of one
	// key is three, for a scene that had nothing more to say.
	written, err := json.Marshal(scenes.Effect{Mode: "Direct"})
	require.NoError(t, err)
	require.JSONEq(t, `"Direct"`, string(written))
}

func TestAnEffectWithAColourIsWrittenInFull(t *testing.T) {
	speed := 40
	written, err := json.Marshal(scenes.Effect{Mode: "Splash", Colour: "#ff0000", Speed: &speed})
	require.NoError(t, err)
	require.JSONEq(t, `{"mode":"Splash","colour":"#ff0000","speed":40}`, string(written))
}

func TestASpeedOfZeroSurvivesTheRoundTrip(t *testing.T) {
	// Zero is the slowest speed, not the absence of one. A plain int would
	// make "as the device has it" and "as slow as it goes" the same scene.
	slowest := 0
	written, err := json.Marshal(scenes.Effect{Mode: "Splash", Speed: &slowest})
	require.NoError(t, err)

	var back scenes.Effect
	require.NoError(t, json.Unmarshal(written, &back))
	require.NotNil(t, back.Speed, "the slowest speed was read as no speed")
	require.Equal(t, 0, *back.Speed)
}

func TestSomethingThatIsNeitherFormSaysWhatAnEffectIs(t *testing.T) {
	var effect scenes.Effect
	err := json.Unmarshal([]byte(`[1,2,3]`), &effect)
	require.ErrorContains(t, err, "a mode's name")
}
