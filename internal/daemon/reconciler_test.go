package daemon_test

import (
	"context"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/ushineko/hotaru/internal/colour"
	"github.com/ushineko/hotaru/internal/config"
	"github.com/ushineko/hotaru/internal/daemon"
	"github.com/ushineko/hotaru/internal/devices"
	"github.com/ushineko/hotaru/internal/openrgb"
	"github.com/ushineko/hotaru/internal/service"
	"github.com/ushineko/hotaru/internal/state"
)

func mouse() devices.Device {
	return devices.Device{
		Name:       "G502 X PLUS",
		LEDCount:   2,
		Modes:      []devices.Mode{{Name: "Direct", PerLED: true}, {Name: "Static"}},
		Zones:      []devices.Zone{{Name: "Mouse", First: 0, Count: 2}},
		ActiveMode: "Direct",
	}
}

func strip() devices.Device {
	return devices.Device{
		Name:       "Generic Strip",
		LEDCount:   2,
		Modes:      []devices.Mode{{Name: "Static", PerLED: true}},
		Zones:      []devices.Zone{{Name: "Strip", First: 0, Count: 2}},
		ActiveMode: "Static",
	}
}

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
	server := openrgb.NewFake(strip())
	svc := service.New(nil, server, "")
	withState(t, svc)

	ctx, cancel := context.WithTimeout(t.Context(), 120*time.Millisecond)
	defer cancel()
	(&daemon.Reconciler{Service: svc, Every: 10 * time.Millisecond}).Run(ctx)

	require.Empty(t, server.Writes, "a machine that has been asked for nothing was written to")
	require.Empty(t, server.Modes)
}

func TestARestoreWaitsForDevicesThatHaveNotEnumeratedYet(t *testing.T) {
	// A cold boot with OpenRGB running before the hardware settled: the
	// recorded devices are not all there, and the restore is unfinished
	// rather than successful.
	full := openrgb.NewFake(strip(), mouse())
	svc := service.New(nil, full, "")
	withState(t, svc)
	asked(t, svc, "Generic", "red")
	asked(t, svc, "G502", "blue")

	partial := openrgb.NewFake(strip())
	svc.SetClient(partial)
	require.NoError(t, partial.SetFrame(t.Context(), "Generic Strip", devices.Frame{
		Device: "Generic Strip", Colours: make([]colour.Colour, 2),
	}))

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
	require.Eventually(t, func() bool {
		showing, _ := partial.Showing("Generic Strip")
		return showing.Colours[0] == colour.MustParse("red")
	}, time.Second, 5*time.Millisecond, "the device that was there was not restored")

	// It keeps trying, and finishes when the mouse turns up, with nobody
	// asking it to.
	partial.Add(mouse())
	require.Eventually(t, func() bool {
		showing, ok := partial.Showing("G502 X PLUS")
		return ok && showing.Colours[0] == colour.MustParse("blue")
	}, time.Second, 5*time.Millisecond, "the late device was never picked up")

	cancel()
	<-done
}

func TestWaitingIsSaidOncePerChangeNotOncePerAttempt(t *testing.T) {
	// A machine waiting on one device should say so once, not fill its journal
	// with a fact that is not changing.
	full := openrgb.NewFake(strip(), mouse())
	svc := service.New(nil, full, "")
	withState(t, svc)
	asked(t, svc, "Generic", "red")
	asked(t, svc, "G502", "blue")
	svc.SetClient(openrgb.NewFake(strip()))

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

func TestADeviceWithAReassertRuleIsSentItsColourAgain(t *testing.T) {
	// The wireless mouse: it restores its onboard colour when it wakes, and
	// nothing but a repeat puts the scene colour back.
	cfg := &config.Config{Devices: []config.DeviceRule{
		{Match: "g502", Reassert: config.Duration(10 * time.Millisecond)},
	}}
	server := openrgb.NewFake(strip(), mouse())
	svc := service.New(cfg, server, "")
	withState(t, svc)
	asked(t, svc, "G502", "blue")
	asked(t, svc, "Generic", "red")

	writesBefore := len(server.Writes)

	// The mouse wakes up having forgotten.
	require.NoError(t, server.SetFrame(t.Context(), "G502 X PLUS", devices.Frame{
		Device: "G502 X PLUS", Colours: make([]colour.Colour, 2),
	}))

	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan struct{})
	go func() {
		(&daemon.Reconciler{Service: svc, Every: 5 * time.Millisecond}).Run(ctx)
		close(done)
	}()

	require.Eventually(t, func() bool {
		showing, _ := server.Showing("G502 X PLUS")
		return showing.Colours[0] == colour.MustParse("blue")
	}, time.Second, 5*time.Millisecond, "the mouse was never re-asserted")

	// Let a few more intervals pass, so repeated re-asserts are on the record.
	time.Sleep(40 * time.Millisecond)
	cancel()
	<-done

	// The restore at start puts every recorded device back, once. After that
	// only the device with a rule is written again: re-asserting everything
	// would write constantly to hardware that has no such problem.
	counts := map[string]int{}
	for _, write := range server.Writes[writesBefore+1:] {
		counts[write.Device]++
	}
	require.LessOrEqual(t, counts["Generic Strip"], 1,
		"a device with no reassert rule was written more than the one restore")
	require.Greater(t, counts["G502 X PLUS"], 1,
		"the device with a rule was not re-asserted repeatedly")
}
