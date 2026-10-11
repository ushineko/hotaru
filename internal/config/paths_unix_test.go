//go:build !windows

package config_test

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/ushineko/hotaru/internal/config"
)

func TestTheFilesFallBackToTheUsualPlacesWithoutXDG(t *testing.T) {
	home := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("XDG_STATE_HOME", "")
	t.Setenv("HOME", home)

	rules, err := config.RulesPath()
	require.NoError(t, err)
	require.Equal(t, filepath.Join(home, ".config", "hotaru", "hotaru.yml"), rules)

	state, err := config.StatePath()
	require.NoError(t, err)
	require.Equal(t, filepath.Join(home, ".local", "state", "hotaru", "state.yml"), state)
}

func TestARelativeXDGValueIsIgnoredAsTheSpecificationSays(t *testing.T) {
	home := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", "relative/path")
	t.Setenv("HOME", home)

	rules, err := config.RulesPath()
	require.NoError(t, err)
	require.Equal(t, filepath.Join(home, ".config", "hotaru", "hotaru.yml"), rules)
}

func TestWithNoRuntimeDirectoryTheSocketHasNowhereToGoAndSaysSo(t *testing.T) {
	// Inventing a path under $HOME would put a world-readable socket where a
	// user-only one was promised, and the socket's permissions are the whole
	// authentication story.
	t.Setenv("XDG_RUNTIME_DIR", "")
	_, err := config.SocketPath()
	require.ErrorContains(t, err, "XDG_RUNTIME_DIR")
}
