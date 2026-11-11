package config

import (
	"fmt"
	"os"
	"path/filepath"
)

// machineCapable is true: Windows has %ProgramData% (spec 072 R3).
const machineCapable = true

/*
The directories used where an XDG variable is not set.

Run by a person, rules, scenes and dashboards are somebody's choices and go
where Windows roams a user's settings. State, added files and the socket
belong to this machine and go under the local profile, each in a directory of
its own, so that what is a cache and what is somebody's work stay apart as
they do under XDG.

As a service, and for the clients of one, everything is the machine's and
goes under %ProgramData%\hotaru, with the same split below it.
*/

func configFallback() (string, error) {
	if Machine() {
		return underProfile("ProgramData")
	}
	return userConfigFallback()
}

func userConfigFallback() (string, error) { return underProfile("APPDATA") }

func stateFallback() (string, error) { return local("state") }

func dataFallback() (string, error) { return local("data") }

func runtimeFallback() (string, error) { return local("run") }

// MachineDir is %ProgramData%\hotaru, whatever the mode: the root that the
// service prepares (see PrepareMachine).
func MachineDir() (string, error) { return underProfile("ProgramData") }

func local(sub string) (string, error) {
	if Machine() {
		return underProfile("ProgramData", sub)
	}
	return underProfile("LOCALAPPDATA", sub)
}

func underProfile(env string, sub ...string) (string, error) {
	base := os.Getenv(env)
	if !filepath.IsAbs(base) {
		return "", fmt.Errorf("%s is not set, so there is nowhere to keep hotaru's files", env)
	}
	return filepath.Join(append([]string{base, App}, sub...)...), nil
}
