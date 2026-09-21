package dashboard_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/ushineko/hotaru/internal/dashboard"
	"github.com/ushineko/hotaru/internal/readings"
)

func store(t *testing.T) *dashboard.Store {
	t.Helper()
	s, err := dashboard.Open(filepath.Join(t.TempDir(), "dashboards.yml"))
	require.NoError(t, err)
	return s
}

func names(all []dashboard.Dashboard) []string {
	out := make([]string, len(all))
	for i, one := range all {
		out[i] = one.Name
	}
	return out
}

func TestAMachineThatHasSavedNothingStillHasDashboards(t *testing.T) {
	// The inert rule, for dashboards: a fresh install has these and has
	// written nothing.
	s := store(t)
	require.Equal(t, []string{"coolant", "cooling", "load", "quiet"}, names(s.All()))
	require.Equal(t, dashboard.Default, s.Active().Name)
	require.True(t, s.Active().Shipped)
}

func TestSavingOverAShippedNameReplacesItUntilItIsDeleted(t *testing.T) {
	s := store(t)

	require.NoError(t, s.Save(dashboard.Dashboard{
		Name: "coolant", Arrangement: dashboard.Big,
		Headline: dashboard.Slot{Source: readings.CPUTemp},
	}))
	mine, err := s.Get("coolant")
	require.NoError(t, err)
	require.False(t, mine.Shipped)
	require.Equal(t, dashboard.Big, mine.Arrangement)

	require.NoError(t, s.Delete("coolant"))
	back, err := s.Get("coolant")
	require.NoError(t, err)
	require.True(t, back.Shipped, "deleting the override did not bring the shipped one back")
	require.Equal(t, dashboard.Ring, back.Arrangement)
}

func TestTheActiveDashboardSurvivesARestart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "dashboards.yml")
	first, err := dashboard.Open(path)
	require.NoError(t, err)
	require.NoError(t, first.Use("quiet"))

	again, err := dashboard.Open(path)
	require.NoError(t, err)
	require.Equal(t, "quiet", again.Active().Name)
}

func TestADeletedActiveDashboardFallsBackRatherThanGoingBlank(t *testing.T) {
	// The panel is decorative and has to keep working through somebody's
	// editing: a screen that went blank because a name disappeared would be
	// a fault reported as a fault.
	s := store(t)
	require.NoError(t, s.Save(dashboard.Dashboard{Name: "mine", Arrangement: dashboard.Big}))
	require.NoError(t, s.Use("mine"))
	require.NoError(t, s.Delete("mine"))

	require.Equal(t, dashboard.Default, s.Active().Name)
}

func TestAPrefixIsEnough(t *testing.T) {
	s := store(t)
	one, err := s.Get("qui")
	require.NoError(t, err)
	require.Equal(t, "quiet", one.Name)

	_, err = s.Get("coo")
	require.ErrorContains(t, err, "coolant", "an ambiguous prefix should name the matches")
	require.ErrorContains(t, err, "cooling")
}

func TestUsingSomethingThatIsNotThereSaysSo(t *testing.T) {
	require.ErrorContains(t, store(t).Use("absent"), "absent")
}

func TestTheNameIsTheKeyAndNotAField(t *testing.T) {
	/*
		The settings codec encodes through JSON, so the yaml tags on the
		stored type are ignored and `json:"-"` is what keeps a field out of
		the file. The first version wrote `name: pluto` inside the entry
		already keyed by `pluto`, which is a second place for it to disagree.
	*/
	path := filepath.Join(t.TempDir(), "dashboards.yml")
	s, err := dashboard.Open(path)
	require.NoError(t, err)
	require.NoError(t, s.Save(dashboard.Dashboard{
		Name: "mine", Arrangement: dashboard.Big,
		Headline: dashboard.Slot{Source: readings.Coolant},
	}))

	body, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Contains(t, string(body), "mine:")
	require.NotContains(t, string(body), "name:")

	// And it comes back with its name, because the key is the name.
	again, err := dashboard.Open(path)
	require.NoError(t, err)
	one, err := again.Get("mine")
	require.NoError(t, err)
	require.Equal(t, "mine", one.Name)
}
