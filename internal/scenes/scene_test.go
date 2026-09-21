package scenes_test

import (
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
		Effects: map[string]string{"keychron": "Solid Splash"},
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
	require.Equal(t, "Solid Splash", evening().Effect("Keychron K4 HE"))
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
	require.NoError(t, s.Save(scenes.Scene{Name: "evening"}))

	all := s.All()
	require.Len(t, all, 2)
	require.Equal(t, "evening", all[0].Name)
}
