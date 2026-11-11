//go:build !windows

package openrgb

import (
	"net"
	"os"
	"syscall"
	"testing"

	"github.com/stretchr/testify/require"
)

func socketPair(t *testing.T) (net.Conn, net.Conn) {
	t.Helper()
	fds, err := syscall.Socketpair(syscall.AF_UNIX, syscall.SOCK_STREAM|syscall.SOCK_CLOEXEC, 0)
	require.NoError(t, err)
	ends := make([]net.Conn, 2)
	for i, fd := range fds {
		file := os.NewFile(uintptr(fd), "socket pair")
		ends[i], err = net.FileConn(file)
		_ = file.Close()
		require.NoError(t, err)
		t.Cleanup(func() { _ = ends[i].Close() })
	}
	return ends[0], ends[1]
}
