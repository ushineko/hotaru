package config

import (
	"os"
	"path/filepath"
	"sync/atomic"
)

// App is the directory name and the D-Bus/application identifier stem.
const App = "hotaru"

// File names. Their owners differ, which is the point of there being three:
// see specs/001 "Files on disk".
const (
	RulesFile  = "hotaru.yml" // the user's. hotaru reads it and never writes it
	ScenesFile = "scenes.yml" // the service's, written when asked
	// DashboardsFile is what the cooler's screen can be asked to draw:
	// configuration, because it is somebody's choices rather than machine
	// state, and beside the scenes for the same reason.
	DashboardsFile = "dashboards.yml"
	StateFile      = "state.yml" // the service's, never hand-edited
	SocketFile     = "hotaru.sock"
)

/*
machine is whether hotaru's files are the machine's rather than one user's:
true for the Windows service, and for a client of an installed one (spec 072
R3). Set once at start, by cmd/, before anything asks for a path.
*/
var machine atomic.Bool //nolint:gochecknoglobals // set once by main, read everywhere after

/*
SetMachine says whether hotaru's files are the machine's.

Only Windows has such a place, %ProgramData%\hotaru. Elsewhere the setting is
kept and changes nothing: a systemd user service and its clients share the
user's directories already.
*/
func SetMachine(on bool) { machine.Store(on) }

// Machine reports whether hotaru's files are the machine's on this system.
func Machine() bool { return machineCapable && machine.Load() }

/*
UserDir is the user's own configuration directory whatever the mode: where
the window keeps its view state, which is a person's and never the service's.
*/
func UserDir() (string, error) {
	return xdgDir("XDG_CONFIG_HOME", userConfigFallback)
}

/*
Dir is where the user's files live: $XDG_CONFIG_HOME/hotaru, or
~/.config/hotaru, or %APPDATA%\hotaru on Windows.

An XDG variable that is set wins on every system. Windows does not set them,
so there they are only ever somebody's choice, and a test's.

XDG_CONFIG_HOME is read directly rather than through os.UserConfigDir because
that function ignores a relative value where the specification says to ignore
it, and a test setting the variable to a temporary directory is the case that
matters here.
*/
func Dir() (string, error) {
	return xdgDir("XDG_CONFIG_HOME", configFallback)
}

// StateDir is $XDG_STATE_HOME/hotaru, or ~/.local/state/hotaru, or
// %LOCALAPPDATA%\hotaru\state on Windows.
//
// Desired state is not configuration: it changes on every scene, it means
// nothing on another machine, and nobody wants it in a backed-up config file.
func StateDir() (string, error) {
	return xdgDir("XDG_STATE_HOME", stateFallback)
}

/*
DataDir is $XDG_DATA_HOME/hotaru, or ~/.local/share/hotaru, or
%LOCALAPPDATA%\hotaru\data on Windows.

Where files somebody added live, as opposed to settings they chose or state
hotaru keeps for itself. The distinction matters at the moment something is
lost: configuration can be written again from memory, state is a cache, and
this is work.
*/
func DataDir() (string, error) {
	return xdgDir("XDG_DATA_HOME", dataFallback)
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

Windows has no such directory and no such variable, so there it is
%LOCALAPPDATA%\hotaru\run: a directory of this user's profile, which other
users cannot open, and which the listener narrows to this user alone.
*/
func RuntimeDir() (string, error) {
	return xdgDir("XDG_RUNTIME_DIR", runtimeFallback)
}

// SocketPath is the Unix socket the service listens on.
func SocketPath() (string, error) { return inDir(RuntimeDir, SocketFile) }

// RulesPath is the user's rules file, which hotaru reads and never writes.
func RulesPath() (string, error) { return inDir(Dir, RulesFile) }

// ScenesPath is the scenes file, which the service writes when asked.
func ScenesPath() (string, error) { return inDir(Dir, ScenesFile) }

// DashboardsPath is the dashboards file.
func DashboardsPath() (string, error) { return inDir(Dir, DashboardsFile) }

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

// xdgDir is the variable's directory where it names one, and otherwise what
// this system uses in its place. fallback returns the whole path.
func xdgDir(env string, fallback func() (string, error)) (string, error) {
	if v := os.Getenv(env); filepath.IsAbs(v) {
		return filepath.Join(v, App), nil
	}
	return fallback()
}
