//go:build !windows

package api_test

import (
	"net"
	"os"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/ushineko/hotaru/internal/api"
	"github.com/ushineko/hotaru/internal/testsock"
)

func TestTheSocketIsThisUsersAlone(t *testing.T) {
	// The socket's permissions are the whole authentication story: there is no
	// port and no token, and this is what stands in for both.
	socket := testsock.Path(t)
	listener, err := api.Listen(t.Context(), socket)
	require.NoError(t, err)
	defer func() { _ = listener.Close() }()

	require.Equal(t, socket, listener.Addr().(*net.UnixAddr).Name)

	stat, err := os.Stat(socket)
	require.NoError(t, err)
	require.Equal(t, api.SocketMode, stat.Mode().Perm())
}
