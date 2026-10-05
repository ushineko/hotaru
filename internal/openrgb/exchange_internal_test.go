package openrgb

import (
	"context"
	"net"
	"os"
	"syscall"
	"testing"
	"time"

	sdk "github.com/csutorasa/go-openrgb-sdk"
	"github.com/stretchr/testify/require"
)

/*
A server that dies in the middle of an exchange (spec 063).

These run against a server on the other end of a socket pair that speaks just
enough of the protocol for a handshake, and then fails at the one moment that
matters: after it has read a request and before it answers. The SDK's
behaviour there is the bug, so a fake client cannot stand in for it.
*/

// brief is the deadline the tests give an exchange, so proving a timeout
// costs a fraction of a second rather than exchangeTimeout.
const brief = 200 * time.Millisecond

// listing is what a test server does when asked how many devices it has.
type listing func(conn net.Conn, reply func(count uint32))

/*
serve starts a server on one end of a socket pair and returns the other. The
first version request is the SDK's Initialize, the second the handshake's
own; agreeing says whether the second one is answered. asked is told each
time a listing is requested.

A socket pair rather than net.Pipe: a pipe blocks every write until the other
end reads, so a server that hangs up fails the client's write instead of
leaving it waiting for an answer, which is the failure in #181. Kernel
sockets buffer and report EOF as TCP does, and need no listener.
*/
func serve(t *testing.T, agreeing bool, list listing) (client net.Conn, asked <-chan struct{}) {
	t.Helper()
	client, server := socketPair(t)

	requests := make(chan struct{}, 8)
	go answer(server, agreeing, list, requests)
	return client, requests
}

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

func answer(conn net.Conn, agreeing bool, list listing, requests chan<- struct{}) {
	decoder := sdk.NewNetPacketDecoder(conn)
	encoder := sdk.NewNetPacketEncoder(conn)
	reply := func(id sdk.NetPacketId, value uint32) {
		b := &sdk.NetPacketDataBuilder{}
		b.WriteUint32(value)
		_ = encoder.Encode(sdk.NewNetPacket(id, 0, b.Bytes()))
	}

	versions := 0
	for {
		packet, err := decoder.Decode()
		if err != nil {
			return
		}
		switch packet.Header.PktId {
		case sdk.NetPacketIdRequestProtocolVersion:
			versions++
			if versions == 1 || agreeing {
				reply(sdk.NetPacketIdRequestProtocolVersion, 3)
			}
		case sdk.NetPacketIdRequestControllerCount:
			requests <- struct{}{}
			list(conn, func(count uint32) { reply(sdk.NetPacketIdRequestControllerCount, count) })
		}
	}
}

func silent(net.Conn, func(uint32)) {}

func hangsUp(conn net.Conn, _ func(uint32)) { _ = conn.Close() }

func connect(t *testing.T, socket net.Conn) *Conn {
	t.Helper()
	conn, err := handshake(t.Context(), socket, "a socket pair", brief)
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })
	return conn
}

// The server read the request and stopped answering, socket still open.
func TestAServerThatStopsAnsweringIsGoneWithinTheDeadline(t *testing.T) {
	socket, _ := serve(t, true, silent)
	conn := connect(t, socket)

	start := time.Now()
	_, err := conn.Devices(t.Context())
	require.Error(t, err)
	require.Less(t, time.Since(start), 10*brief, "the exchange outlived its deadline")
	require.True(t, conn.Gone(), "a server that stopped answering was not noticed")
}

// The crash in #181: the server read the request and its socket closed.
func TestAServerThatDiesMidExchangeIsGoneWithinTheDeadline(t *testing.T) {
	socket, _ := serve(t, true, hangsUp)
	conn := connect(t, socket)

	_, err := conn.Devices(t.Context())
	require.Error(t, err)
	require.True(t, conn.Gone(), "a server that died mid-exchange was not noticed")
}

/*
A caller that gives up does not abandon the exchange.

Abandoning one leaves its reply channel in the SDK, and the answer that still
arrives wedges the SDK's read loop: the next exchange then waits for nothing.
So the answer is waited for, the connection stays usable, and nothing marks a
healthy server gone because a client hung up.
*/
func TestACallerThatGivesUpDoesNotPoisonTheConnection(t *testing.T) {
	late := func(_ net.Conn, reply func(uint32)) {
		time.Sleep(brief / 4)
		reply(0)
	}
	socket, asked := serve(t, true, late)
	conn := connect(t, socket)

	ctx, cancel := context.WithCancel(t.Context())
	go func() { <-asked; cancel() }()
	_, err := conn.Devices(ctx)
	require.NoError(t, err, "the exchange ended with its caller rather than its answer")
	require.False(t, conn.Gone())

	// The read loop is still delivering, which an abandoned exchange stops.
	go func() { <-asked }()
	_, err = conn.Devices(t.Context())
	require.NoError(t, err, "the connection stopped answering after a caller gave up")
}

// Asking whether the server has gone does not wait behind an exchange stuck on it.
func TestGoneAnswersWhileAnExchangeIsStuck(t *testing.T) {
	socket, asked := serve(t, true, silent)
	conn := connect(t, socket)

	done := make(chan struct{})
	go func() { defer close(done); _, _ = conn.Devices(t.Context()) }()
	<-asked

	start := time.Now()
	require.False(t, conn.Gone())
	require.Less(t, time.Since(start), brief/2, "Gone waited for the exchange in flight")
	<-done
}

// The SDK's own Close blocks for good once an exchange has timed out.
func TestCloseReturnsAfterAnExchangeTimedOut(t *testing.T) {
	socket, _ := serve(t, true, silent)
	conn, err := handshake(t.Context(), socket, "a socket pair", brief)
	require.NoError(t, err)
	_, err = conn.Devices(t.Context())
	require.Error(t, err)

	closed := make(chan error, 1)
	go func() { closed <- conn.Close() }()
	select {
	case err := <-closed:
		require.NoError(t, err)
	case <-time.After(5 * brief):
		t.Fatal("Close did not return after an exchange timed out")
	}
}

// A server that accepts and never finishes the handshake cannot hold a redial.
func TestDialGivesUpOnAServerThatDoesNotAgree(t *testing.T) {
	socket, _ := serve(t, false, silent)

	_, err := handshake(t.Context(), socket, "a socket pair", brief)
	require.Error(t, err)
}
