//go:build !windows

package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// The directories used where an XDG variable is not set: the ones the XDG
// specification gives as its defaults.

func configFallback() (string, error) { return underHome(".config") }

func stateFallback() (string, error) { return underHome(filepath.Join(".local", "state")) }

func dataFallback() (string, error) { return underHome(filepath.Join(".local", "share")) }

func runtimeFallback() (string, error) {
	return "", errors.New("XDG_RUNTIME_DIR is not set, so there is nowhere session-scoped to put the socket")
}

func underHome(dir string) (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("find the home directory: %w", err)
	}
	return filepath.Join(home, dir, App), nil
}
