package daemon_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/ushineko/hotaru/internal/cooler"
	"github.com/ushineko/hotaru/internal/cooler/coolertest"
	"github.com/ushineko/hotaru/internal/daemon"
	"github.com/ushineko/hotaru/internal/openrgb"
	"github.com/ushineko/hotaru/internal/service"
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
		return cooler.New(coolertest.New()), nil
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

func TestACoolerThatGoesIsDetachedAndFoundAgain(t *testing.T) {
	/*
		Spec 059 R3. An unplugged cooler answers ErrGone. Before, the service
		held the dead handle until somebody restarted it; now it lets go,
		reports absence as it does on a machine with none, waits with the
		same backoff, and attaches whatever the next open finds -- with a
		dashboard pusher of its own, because the last one may have stopped
		for good on a panel it could not claim.
	*/
	first := coolertest.NewPanel()
	first.GoneOnce = true
	second := coolertest.NewPanel()
	second.Reading.Coolant = 41

	var mu sync.Mutex
	opens := 0
	open := func(context.Context) (*cooler.Cooler, error) {
		mu.Lock()
		defer mu.Unlock()
		opens++
		switch opens {
		case 1:
			return cooler.New(first), nil
		case 2:
			return nil, cooler.ErrNoCooler // unplugged: the wait is the same one
		default:
			return cooler.New(second), nil
		}
	}

	svc := service.New(nil, openrgb.NewFake(), "")
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan struct{})
	go func() {
		defer close(done)
		daemon.Attach(ctx, svc, open, quick, func(string, ...any) {})
	}()

	// The first dashboard frame reads the cooler, which has gone.
	require.Eventually(t, first.Closed, 2*time.Second, 5*time.Millisecond,
		"a cooler that had gone was not closed")

	require.Eventually(t, func() bool {
		status, _, err := svc.Cooling(t.Context())
		return err == nil && status.Coolant == 41
	}, 2*time.Second, 5*time.Millisecond, "the cooler that came back was not attached")

	// R3.2: the dashboard is drawn on the cooler that came back.
	require.Eventually(t, func() bool { return second.Images() > 0 }, 2*time.Second, 5*time.Millisecond,
		"the pusher did not start again on the re-attached cooler")

	mu.Lock()
	require.Equal(t, 3, opens, "it did not look for the cooler again through open")
	mu.Unlock()

	cancel()
	<-done
	require.True(t, second.Closed(), "stopping the service left the cooler open")
	_, _, err := svc.Cooling(t.Context())
	require.ErrorIs(t, err, cooler.ErrNoCooler, "a stopped attachment left the cooler set")
}
