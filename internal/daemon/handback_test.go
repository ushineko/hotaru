package daemon_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/ushineko/hotaru/internal/cooler"
	"github.com/ushineko/hotaru/internal/cooler/coolertest"
	"github.com/ushineko/hotaru/internal/daemon"
	"github.com/ushineko/hotaru/internal/devices"
	"github.com/ushineko/hotaru/internal/openrgb"
	"github.com/ushineko/hotaru/internal/service"
)

// retouches is how many times the cooler's lights were sent again.
func (l *lines) retouches() int {
	n := 0
	for _, said := range l.all() {
		if strings.HasPrefix(said, "sent the cooler's lights") {
			n++
		}
	}
	return n
}

func TestTheCoolersLightsAreSentAgainAfterTheScreenChangesHands(t *testing.T) {
	/*
		AC5 (#24). Once after the service takes the cooler's screen, and once
		after a switch to the readout, each after the wait; never before it.
	*/
	daemon.Handback(t, 40*time.Millisecond)
	ring := devices.Device{
		Name: "NZXT Kraken Ring", Location: "HID: /dev/hidraw-fake", LEDCount: 2,
		Modes:      []devices.Mode{{Name: "Direct", PerLED: true}},
		Zones:      []devices.Zone{{Name: "Ring", First: 0, Count: 2}},
		ActiveMode: "Direct",
	}
	svc := service.New(nil, openrgb.NewFake(ring), "")
	withState(t, svc)
	asked(t, svc, ring.Name, "teal")

	open := func(context.Context) (*cooler.Cooler, error) { return cooler.New(coolertest.NewPanel()), nil }
	said := &lines{}
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan struct{})
	go func() { defer close(done); daemon.Attach(ctx, svc, open, quick, said.report) }()
	defer func() { cancel(); <-done }()

	require.Eventually(t, func() bool { return said.retouches() == 1 }, 2*time.Second, 5*time.Millisecond,
		"the cooler's lights were not sent again after the service took the screen")

	require.Eventually(t, func() bool {
		return svc.Draw(t.Context(), service.Screen{Readout: true}) == nil
	}, 2*time.Second, 5*time.Millisecond)
	require.Equal(t, 1, said.retouches(), "sent again before the wait was over")
	require.Eventually(t, func() bool { return said.retouches() == 2 }, 2*time.Second, 5*time.Millisecond,
		"the cooler's lights were not sent again after the switch to the readout")
}
