package config

import (
	"fmt"
	"os"
	"path/filepath"
)

/*
The directories used where an XDG variable is not set.

Rules, scenes and dashboards are somebody's choices and go where Windows
roams a user's settings. State, added files and the socket belong to this
machine and go under the local profile, each in a directory of its own, so
that what is a cache and what is somebody's work stay apart as they do under
XDG.
*/

func configFallback() (string, error) { return underProfile("APPDATA") }

func stateFallback() (string, error) { return underProfile("LOCALAPPDATA", "state") }

func dataFallback() (string, error) { return underProfile("LOCALAPPDATA", "data") }

func runtimeFallback() (string, error) { return underProfile("LOCALAPPDATA", "run") }

func underProfile(env string, sub ...string) (string, error) {
	base := os.Getenv(env)
	if !filepath.IsAbs(base) {
		return "", fmt.Errorf("%s is not set, so there is nowhere to keep hotaru's files", env)
	}
	return filepath.Join(append([]string{base, App}, sub...)...), nil
}
