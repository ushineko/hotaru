package daemon_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/ushineko/hotaru/internal/cooler"
	"github.com/ushineko/hotaru/internal/daemon"
)

// quick is a backoff a test can wait on.
var quick = []time.Duration{time.Millisecond}

func TestTheCoolerIsWaitedForRatherThanOpenedOnce(t *testing.T) {
	/*
		The cold boot this exists for. A `uaccess` udev rule grants its ACL to
		an active seat session, and `enable-linger` starts the service with
		the machine -- sixteen seconds before the login that makes the device
		openable, measured on the development machine. Opened once, that
		failure was permanent: the lights came back and the panel did not
		(#136).
	*/
	denied := errors.New("open /dev/hidraw6: permission denied")
	attempts := 0
	open := func(context.Context) (*cooler.Cooler, error) {
		attempts++
		if attempts < 4 {
			return nil, denied
		}
		return cooler.NewWithFake(cooler.NewFake()), nil
	}

	var said []string
	found := daemon.WaitForCooler(t.Context(), open, quick,
		func(format string, _ ...any) { said = append(said, format) })

	require.NotNil(t, found, "the cooler never opened")
	require.Equal(t, 4, attempts, "it did not keep trying")

	// Once, not every tick: a machine with no cooler must not have its
	// journal filled with a fact that is not changing.
	require.Len(t, said, 1, "it said the same thing on every attempt")
}

func TestWaitingForTheCoolerEndsWithTheService(t *testing.T) {
	// A machine with no cooler waits for one until it stops, and stopping is
	// not a failure to report.
	ctx, stop := context.WithCancel(t.Context())

	attempts := 0
	open := func(context.Context) (*cooler.Cooler, error) {
		attempts++
		if attempts == 2 {
			stop()
		}
		return nil, errors.New("no cooler on this machine")
	}

	done := make(chan *cooler.Cooler, 1)
	go func() { done <- daemon.WaitForCooler(ctx, open, quick, func(string, ...any) {}) }()

	select {
	case found := <-done:
		require.Nil(t, found, "a stopped service found a cooler")
	case <-time.After(3 * time.Second):
		t.Fatal("waiting for the cooler did not end with the service")
	}
}
