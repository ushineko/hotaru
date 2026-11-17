package daemon_test

import (
	"context"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/ushineko/hotaru/internal/colour"
	"github.com/ushineko/hotaru/internal/daemon"
	"github.com/ushineko/hotaru/internal/devices"
	"github.com/ushineko/hotaru/internal/service"
	"github.com/ushineko/hotaru/internal/service/servicetest"
	"github.com/ushineko/hotaru/internal/state"
)

func asked(t *testing.T, svc *service.Service, device, name string) {
	t.Helper()
	target, err := devices.ParseTarget(device)
	require.NoError(t, err)
	results, err := svc.Apply(t.Context(), service.Request{
		Assignments: []devices.Assignment{{Target: target, Colour: colour.MustParse(name)}},
	})
	require.NoError(t, err)
	require.True(t, results[0].Applied)
}

// lines collects what the reconciler reported, for asserting it is not chatty.
type lines struct {
	mu   sync.Mutex
	said []string
}

func (l *lines) report(format string, _ ...any) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.said = append(l.said, format)
}

func (l *lines) count() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return len(l.said)
}

func withState(t *testing.T, svc *service.Service) *state.Store {
	t.Helper()
	store, err := state.Open(filepath.Join(t.TempDir(), "state.yml"))
	require.NoError(t, err)
	svc.SetRecorder(store)
	return store
}

func TestAFreshInstallRunsEveryMechanismAndWritesToNothing(t *testing.T) {
	strip := stripCanvas()
	svc := servicetest.New(t, nil, strip)
	withState(t, svc)

	ctx, cancel := context.WithTimeout(t.Context(), 120*time.Millisecond)
	defer cancel()
	(&daemon.Reconciler{Service: svc, Every: 10 * time.Millisecond}).Run(ctx)

	require.Zero(t, strip.Frames(), "a machine that has been asked for nothing was written to")
}

func TestARestoreWaitsForDevicesThatHaveNotEnumeratedYet(t *testing.T) {
	// A cold boot before the hardware settled: the recorded devices are not
	// all there, and the restore is unfinished rather than successful.
	svc := service.New(nil)
	withState(t, svc)
	strip := servicetest.Attach(t, svc, stripCanvas())
	mouse := servicetest.Attach(t, svc, mouseCanvas())
	asked(t, svc, "Generic", "red")
	asked(t, svc, "G502", "blue")

	// Both go; the strip comes back on its power-on colours, the mouse not
	// yet.
	svc.DetachCanvas(strip.Attach)
	svc.DetachCanvas(mouse.Attach)
	partial := stripCanvas()
	servicetest.Attach(t, svc, partial)

	said := &lines{}
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan struct{})
	go func() {
		(&daemon.Reconciler{
			Service: svc,
			Backoff: []time.Duration{5 * time.Millisecond},
			Every:   time.Hour,
			Report:  said.report,
		}).Run(ctx)
		close(done)
	}()

	// What is present is put back immediately, without waiting for the rest.
	require.Eventually(t, func() bool { return first(partial.Device, colour.MustParse("red")) },
		time.Second, 5*time.Millisecond, "the device that was there was not restored")

	// It keeps trying, and finishes when the mouse turns up, with nobody
	// asking it to.
	late := mouseCanvas()
	servicetest.Attach(t, svc, late)
	require.Eventually(t, func() bool { return first(late.Device, colour.MustParse("blue")) },
		time.Second, 5*time.Millisecond, "the late device was never picked up")

	cancel()
	<-done
}

func TestWaitingIsSaidOncePerChangeNotOncePerAttempt(t *testing.T) {
	// A machine waiting on one device should say so once, not fill its journal
	// with a fact that is not changing.
	svc := service.New(nil)
	withState(t, svc)
	servicetest.Attach(t, svc, stripCanvas())
	mouse := servicetest.Attach(t, svc, mouseCanvas())
	asked(t, svc, "Generic", "red")
	asked(t, svc, "G502", "blue")
	svc.DetachCanvas(mouse.Attach)

	said := &lines{}
	ctx, cancel := context.WithTimeout(t.Context(), 200*time.Millisecond)
	defer cancel()
	(&daemon.Reconciler{
		Service: svc,
		Backoff: []time.Duration{5 * time.Millisecond},
		Every:   time.Hour,
		Report:  said.report,
	}).Run(ctx)

	require.LessOrEqual(t, said.count(), 2,
		"the same missing device was reported on every attempt")
}
