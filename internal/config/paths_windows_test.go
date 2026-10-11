package config_test

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/ushineko/hotaru/internal/config"
)

// profile is a made-up user profile with no XDG variable set, which is what
// Windows gives a program.
func profile(t *testing.T) (roaming, local string) {
	t.Helper()
	roaming, local = t.TempDir(), t.TempDir()
	for _, name := range []string{"XDG_CONFIG_HOME", "XDG_STATE_HOME", "XDG_DATA_HOME", "XDG_RUNTIME_DIR"} {
		t.Setenv(name, "")
	}
	t.Setenv("APPDATA", roaming)
	t.Setenv("LOCALAPPDATA", local)
	return roaming, local
}

func TestWithoutXDGTheFilesGoWhereWindowsKeepsAUsersThings(t *testing.T) {
	roaming, local := profile(t)

	rules, err := config.RulesPath()
	require.NoError(t, err)
	require.Equal(t, filepath.Join(roaming, "hotaru", "hotaru.yml"), rules)

	// State and added files are this machine's, so they do not roam, and they
	// are kept apart from each other as they are under XDG.
	state, err := config.StatePath()
	require.NoError(t, err)
	require.Equal(t, filepath.Join(local, "hotaru", "state", "state.yml"), state)

	data, err := config.DataDir()
	require.NoError(t, err)
	require.Equal(t, filepath.Join(local, "hotaru", "data"), data)

	socket, err := config.SocketPath()
	require.NoError(t, err)
	require.Equal(t, filepath.Join(local, "hotaru", "run", "hotaru.sock"), socket)
}

func TestARelativeXDGValueIsIgnoredOnWindowsToo(t *testing.T) {
	roaming, _ := profile(t)
	t.Setenv("XDG_CONFIG_HOME", `relative\path`)

	rules, err := config.RulesPath()
	require.NoError(t, err)
	require.Equal(t, filepath.Join(roaming, "hotaru", "hotaru.yml"), rules)
}

func TestWithNoProfileThereIsNowhereToKeepAnythingAndItSaysSo(t *testing.T) {
	profile(t)
	t.Setenv("LOCALAPPDATA", "")

	_, err := config.SocketPath()
	require.ErrorContains(t, err, "LOCALAPPDATA")
}
