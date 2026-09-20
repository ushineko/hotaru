package config

import (
	"os"
	"path/filepath"
)

// App is the directory name and the D-Bus/application identifier stem.
const App = "hotaru"

// File names. Their owners differ, which is the point of there being three:
// see specs/001 "Files on disk".
const (
	RulesFile  = "hotaru.yml" // the user's. hotaru reads it and never writes it
	ScenesFile = "scenes.yml" // the service's, written when asked
	StateFile  = "state.yml"  // the service's, never hand-edited
)

/*
ConfigDir is where the user's files live: $XDG_CONFIG_HOME/hotaru, or
~/.config/hotaru.

XDG_CONFIG_HOME is read directly rather than through os.UserConfigDir because
that function ignores a relative value where the specification says to ignore
it, and a test setting the variable to a temporary directory is the case that
matters here.
*/
func ConfigDir() (string, error) {
	return xdgDir("XDG_CONFIG_HOME", ".config")
}

// StateDir is $XDG_STATE_HOME/hotaru, or ~/.local/state/hotaru.
//
// Desired state is not configuration: it changes on every scene, it means
// nothing on another machine, and nobody wants it in a backed-up config file.
func StateDir() (string, error) {
	return xdgDir("XDG_STATE_HOME", filepath.Join(".local", "state"))
}

// RulesPath, ScenesPath and StatePath are the three files, fully resolved.
func RulesPath() (string, error)  { return inDir(ConfigDir, RulesFile) }
func ScenesPath() (string, error) { return inDir(ConfigDir, ScenesFile) }
func StatePath() (string, error)  { return inDir(StateDir, StateFile) }

func inDir(dir func() (string, error), name string) (string, error) {
	d, err := dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(d, name), nil
}

func xdgDir(env, fallback string) (string, error) {
	if v := os.Getenv(env); filepath.IsAbs(v) {
		return filepath.Join(v, App), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, fallback, App), nil
}
