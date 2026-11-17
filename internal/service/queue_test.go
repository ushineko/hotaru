package service_test

import (
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/ushineko/hotaru/internal/colour"
	"github.com/ushineko/hotaru/internal/queue"
	"github.com/ushineko/hotaru/internal/scenes"
	"github.com/ushineko/hotaru/internal/service"
	"github.com/ushineko/hotaru/internal/service/servicetest"
)

/*
The write queue on a device whose writes take time: a cooler's firmware
effect, which is an exchange with the device each time (spec 077 R6.1). The
newest write replaces a waiting one, and a replaced write is not a failure.
*/
func slowCooler(t *testing.T) (*service.Service, func(string) []service.Result) {
	t.Helper()
	cooler := tunedCooler()
	cooler.Delay = 20 * time.Millisecond
	q := queue.New(t.Context())
	t.Cleanup(q.Close)
	svc := servicetest.New(t, nil, cooler)
	svc.SetRecorder(recorder(t))
	svc.SetQueue(q)
	apply := func(hex string) []service.Result {
		red := colour.MustParse("red")
		out, err := svc.Apply(t.Context(), service.Request{
			Colour:  &red,
			Effects: map[string]scenes.Effect{"tuned": {Mode: "Fading", Colours: []string{hex, "#000001"}}},
		})
		require.NoError(t, err)
		return out
	}
	return svc, apply
}

func TestRapidWritesToOneDeviceConvergeOnTheLast(t *testing.T) {
	// Spec 026's failure: three scenes in a second must not queue behind
	// each other. The newest replaces the pending one, and the device ends up
	// showing what was last asked for.
	svc, apply := slowCooler(t)
	shades := []string{"#ff0000", "#00ff00", "#0000ff", "#800080", "#008080"}
	results := make([][]service.Result, len(shades))
	var wg sync.WaitGroup
	for i, hex := range shades {
		wg.Add(1)
		go func() { defer wg.Done(); results[i] = apply(hex) }()
		time.Sleep(2 * time.Millisecond) // pressed in quick succession, not at once
	}
	wg.Wait()

	superseded := 0
	for _, out := range results {
		if len(out) > 0 && out[0].Superseded {
			superseded++
		}
	}
	require.Positive(t, superseded, "every write waited its turn rather than being replaced")
	mode := modeNamed(t, viewOf(t, svc, "Tuned Cooler"), "Fading")
	require.Equal(t, "#008080", mode.Colours[0].String(), "the device is not showing the last write")
}

func TestASupersededWriteIsNotAFailure(t *testing.T) {
	_, apply := slowCooler(t)
	go func() { _ = apply("#ff0000") }() // in flight
	time.Sleep(5 * time.Millisecond)
	first := make(chan []service.Result, 1)
	go func() { first <- apply("#00ff00") }() // waiting
	time.Sleep(5 * time.Millisecond)

	out := apply("#0000ff") // replacing the waiter
	require.True(t, out[0].Applied)
	replaced := <-first
	require.True(t, replaced[0].Superseded)
	require.False(t, replaced[0].Applied)
	require.Empty(t, replaced[0].Skipped, "superseded is not a device declining to do something")
	require.NoError(t, replaced[0].Err, "nor an error")
}
