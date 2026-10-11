/*
Package testsock gives a test somewhere to put a Unix socket.

A socket's address is a fixed field in a system structure: 108 bytes on Linux,
104 on some others, and the service refuses anything longer. t.TempDir() names
its directory after the test, and under a Windows profile's temporary
directory a long test name alone overruns the limit. This directory is named
for nothing.
*/
package testsock

import (
	"os"
	"path/filepath"
	"testing"
)

// Path is a socket path short enough to listen on, in a directory of its own
// that is removed when the test ends. Nothing exists at the path.
func Path(t testing.TB) string {
	t.Helper()
	dir, err := os.MkdirTemp("", "h")
	if err != nil {
		t.Fatalf("make a directory for the socket: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	return filepath.Join(dir, "s")
}
