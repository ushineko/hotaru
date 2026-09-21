package config

import (
	"errors"
	"fmt"
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
	SocketFile = "hotaru.sock"
)

/*
Dir is where the user's files live: $XDG_CONFIG_HOME/hotaru, or
~/.config/hotaru.

XDG_CONFIG_HOME is read directly rather than through os.UserConfigDir because
that function ignores a relative value where the specification says to ignore
it, and a test setting the variable to a temporary directory is the case that
matters here.
*/
func Dir() (string, error) {
	return xdgDir("XDG_CONFIG_HOME", ".config")
}

// StateDir is $XDG_STATE_HOME/hotaru, or ~/.local/state/hotaru.
//
// Desired state is not configuration: it changes on every scene, it means
// nothing on another machine, and nobody wants it in a backed-up config file.
func StateDir() (string, error) {
	return xdgDir("XDG_STATE_HOME", filepath.Join(".local", "state"))
}

/*
DataDir is $XDG_DATA_HOME/hotaru, or ~/.local/share/hotaru.

Where files somebody added live, as opposed to settings they chose or state
hotaru keeps for itself. The distinction matters at the moment something is
lost: configuration can be written again from memory, state is a cache, and
this is work.
*/
func DataDir() (string, error) {
	return xdgDir("XDG_DATA_HOME", filepath.Join(".local", "share"))
}

/*
RuntimeDir is $XDG_RUNTIME_DIR/hotaru: where the socket lives.

Runtime rather than config or state, because the socket is meaningless once the
process is gone and the directory is cleared at logout. Under lingering it
exists from boot, which is what lets the service be reachable before anyone has
logged in.

There is no fallback to a home directory. A machine with no XDG_RUNTIME_DIR has
no session-scoped place to put a socket, and inventing one in $HOME would put a
world-readable path where a user-only one was promised.
*/
func RuntimeDir() (string, error) {
	dir := os.Getenv("XDG_RUNTIME_DIR")
	if !filepath.IsAbs(dir) {
		return "", errors.New("XDG_RUNTIME_DIR is not set, so there is nowhere session-scoped to put the socket")
	}
	return filepath.Join(dir, App), nil
}

// SocketPath is the Unix socket the service listens on.
func SocketPath() (string, error) { return inDir(RuntimeDir, SocketFile) }

// RulesPath is the user's rules file, which hotaru reads and never writes.
func RulesPath() (string, error) { return inDir(Dir, RulesFile) }

// ScenesPath is the scenes file, which the service writes when asked.
func ScenesPath() (string, error) { return inDir(Dir, ScenesFile) }

// StatePath is the desired-state file, outside the configuration directory
// because it is not configuration.
func StatePath() (string, error) { return inDir(StateDir, StateFile) }

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
		return "", fmt.Errorf("find the home directory: %w", err)
	}
	return filepath.Join(home, fallback, App), nil
}
