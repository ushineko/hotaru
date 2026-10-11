package openrgb

import (
	"net"
	"testing"

	"github.com/stretchr/testify/require"
)

// socketPair is the two ends of a loopback TCP connection: Windows has no
// socketpair, and TCP on loopback is what the client meets there in any case.
func socketPair(t *testing.T) (net.Conn, net.Conn) {
	t.Helper()
	var config net.ListenConfig
	listener, err := config.Listen(t.Context(), "tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer func() { _ = listener.Close() }()

	accepted := make(chan net.Conn, 1)
	go func() {
		conn, err := listener.Accept()
		if err != nil {
			close(accepted)
			return
		}
		accepted <- conn
	}()

	var dialer net.Dialer
	client, err := dialer.DialContext(t.Context(), "tcp", listener.Addr().String())
	require.NoError(t, err)
	server, ok := <-accepted
	require.True(t, ok, "the listener accepted nothing")

	t.Cleanup(func() { _ = client.Close(); _ = server.Close() })
	return client, server
}

/*
A server that is killed, or whose machine closes the connection under it,
does not hang up politely: the client's next read or write fails with a
Winsock error, not EOF. Those are not the errno values Unix reports, and a
connection that is not recognised as gone is never redialled.
*/
func TestAConnectionResetOnWindowsIsGone(t *testing.T) {
	client, server := socketPair(t)

	// Linger of zero turns the close into a reset.
	require.NoError(t, server.(*net.TCPConn).SetLinger(0))
	require.NoError(t, server.Close())

	_, err := client.Read(make([]byte, 1))
	require.Error(t, err)
	require.True(t, isGone(err), "a reset read as %v is not counted as gone", err)

	// And writing into the reset connection, which reports the abort.
	var writeErr error
	for range 4 {
		if _, writeErr = client.Write(make([]byte, 16)); writeErr != nil {
			break
		}
	}
	require.Error(t, writeErr)
	require.True(t, isGone(writeErr), "a write after a reset, %v, is not counted as gone", writeErr)
}
