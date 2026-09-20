package config_test

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/ushineko/hotaru/internal/config"
)

func TestTheFilesFollowXDGWhenItIsSet(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", "/tmp/cfg")
	t.Setenv("XDG_STATE_HOME", "/tmp/state")

	rules, err := config.RulesPath()
	require.NoError(t, err)
	require.Equal(t, "/tmp/cfg/hotaru/hotaru.yml", rules)

	scenes, err := config.ScenesPath()
	require.NoError(t, err)
	require.Equal(t, "/tmp/cfg/hotaru/scenes.yml", scenes)

	// State is not configuration: it changes on every scene and means nothing
	// on another machine, so it does not live among files people back up.
	state, err := config.StatePath()
	require.NoError(t, err)
	require.Equal(t, "/tmp/state/hotaru/state.yml", state)
}

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
