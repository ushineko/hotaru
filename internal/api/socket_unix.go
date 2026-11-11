//go:build !windows

package api

import (
	"fmt"
	"os"
)

// restrict makes the socket this user's alone: SocketMode on the file.
func restrict(socket string) error {
	if err := os.Chmod(socket, SocketMode); err != nil {
		return fmt.Errorf("change its mode: %w", err)
	}
	return nil
}
