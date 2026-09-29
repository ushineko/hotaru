package daemon_test

import (
	"context"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/ushineko/hotaru/internal/colour"
	"github.com/ushineko/hotaru/internal/daemon"
	"github.com/ushineko/hotaru/internal/openrgb"
	"github.com/ushineko/hotaru/internal/service"
)

/*
servers hands out one fake per dial, in order.

A reconnection reaches a different process from the one that went away -- that
is what makes it a reconnection rather than a retry -- so a test of it needs
the second dial to produce a server that has never been written to.
*/
type servers struct {
	mu   sync.Mutex
	next int
	all  []openrgb.Client
}

func (s *servers) dial(context.Context, string) (openrgb.Client, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	at := min(s.next, len(s.all)-1)
	s.next++
	return s.all[at], nil
}

func (s *servers) dials() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.next
}

// TestAServiceThatLosesTheServerReconnectsAndPutsTheLightsBack is spec 058 AC1.
func TestAServiceThatLosesTheServerReconnectsAndPutsTheLightsBack(t *testing.T) {
	first, second := openrgb.NewFake(strip()), openrgb.NewFake(strip())
	svc := service.New(nil, first, "")
	withState(t, svc)
	asked(t, svc, "Generic", "red")

	pool := &servers{all: []openrgb.Client{first, second}}
	said := &lines{}
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan struct{})
	go func() {
		defer close(done)
		daemon.Supervise(ctx, svc, "", said.report, pool.dial, 5*time.Millisecond)
	}()

	require.Eventually(t, func() bool { return pool.dials() >= 1 }, time.Second, 5*time.Millisecond,
		"the supervisor never connected in the first place")

	// The server exits. Every exchange on that socket now fails the way a
	// socket whose peer has gone fails, which is not how a missing device
	// fails.
	first.Vanish(syscall.EPIPE)

	// Nothing restarts the service, and nothing asks it to reconnect.
	require.Eventually(t, func() bool { return pool.dials() >= 2 }, 2*time.Second, 5*time.Millisecond,
		"the service kept holding a socket to a server that had gone")

	// R2: the server that came back enumerated afresh, with the strip at its
	// power-on colour. What was asked for is on it again without being asked
	// for again.
	require.Eventually(t, func() bool {
		showing, ok := second.Showing("Generic Strip")
		return ok && showing.Colours[0] == colour.MustParse("red")
	}, 2*time.Second, 5*time.Millisecond, "the lights were not put back on the server that came back")

	// A server that came back is news; a server that is still there is not.
	require.Contains(t, said.all(), "the OpenRGB server at %s came back (protocol %d); putting the lights back")

	cancel()
	<-done
}

// TestATransientErrorIsNotADepartedServer guards the other half: a device that
// is not there is an answer, and answering badly is not grounds to redial.
func TestATransientErrorIsNotADepartedServer(t *testing.T) {
	only := openrgb.NewFake(strip())
	svc := service.New(nil, only, "")
	withState(t, svc)
	asked(t, svc, "Generic", "red")

	pool := &servers{all: []openrgb.Client{only}}
	said := &lines{}
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan struct{})
	go func() {
		defer close(done)
		daemon.Supervise(ctx, svc, "", said.report, pool.dial, 5*time.Millisecond)
	}()

	require.Eventually(t, func() bool { return pool.dials() >= 1 }, time.Second, 5*time.Millisecond)

	// A server that is answering, and answering an error.
	only.Vanish(syscall.ENODEV)

	require.Never(t, func() bool { return pool.dials() > 1 }, 200*time.Millisecond, 10*time.Millisecond,
		"a failing call was mistaken for a server that had gone")
	require.False(t, svc.Gone(), "an error that is not the socket was recorded as one")

	cancel()
	<-done
}
