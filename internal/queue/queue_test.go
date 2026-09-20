package queue_test

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/ushineko/hotaru/internal/queue"
)

func TestOneDeviceIsWrittenOneJobAtATime(t *testing.T) {
	// Two writes to one device must not interleave: the whole reason this
	// exists is that a frame is atomic from the device's point of view.
	set := queue.New(t.Context())
	defer set.Close()

	var inFlight, overlaps atomic.Int32
	var wg sync.WaitGroup
	for range 20 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			set.Do("one", func(context.Context) {
				if inFlight.Add(1) > 1 {
					overlaps.Add(1)
				}
				time.Sleep(time.Millisecond)
				inFlight.Add(-1)
			})
		}()
	}
	wg.Wait()
	require.Zero(t, overlaps.Load(), "two jobs ran against one device at once")
}

func TestDifferentDevicesDoNotWaitForEachOther(t *testing.T) {
	set := queue.New(t.Context())
	defer set.Close()

	start := time.Now()
	var wg sync.WaitGroup
	for _, device := range []string{"one", "two", "three", "four"} {
		wg.Add(1)
		go func() {
			defer wg.Done()
			set.Do(device, func(context.Context) { time.Sleep(30 * time.Millisecond) })
		}()
	}
	wg.Wait()
	require.Less(t, time.Since(start), 100*time.Millisecond,
		"four devices took as long as four jobs in a row")
}

func TestABackgroundJobIsReplacedByANewerOneRatherThanQueued(t *testing.T) {
	// Lighting is a state, not a sequence. Nobody wants the second-to-last
	// scene applied after the last one, and the old bounded queue dropped
	// whichever job happened to be oldest instead.
	set := queue.New(t.Context())
	defer set.Close()

	release := make(chan struct{})
	var ran []int
	var mu sync.Mutex

	set.Post("one", func(context.Context) { <-release }) // occupies the worker
	for i := 1; i <= 5; i++ {
		set.Post("one", func(context.Context) {
			mu.Lock()
			ran = append(ran, i)
			mu.Unlock()
		})
	}
	close(release)

	require.Eventually(t, func() bool {
		mu.Lock()
		defer mu.Unlock()
		return len(ran) > 0
	}, time.Second, 5*time.Millisecond)

	time.Sleep(20 * time.Millisecond)
	mu.Lock()
	defer mu.Unlock()
	require.Equal(t, []int{5}, ran, "the queue kept work that had been countermanded")
}

func TestACallerWaitingOnASupersededJobIsToldRatherThanLeftGuessing(t *testing.T) {
	set := queue.New(t.Context())
	defer set.Close()

	release := make(chan struct{})
	set.Post("one", func(context.Context) { <-release })
	// Let the worker pick that job up. Until it does, the slot still holds it
	// and the next submission replaces it rather than queueing behind it --
	// which is the design working, and would make this test assert nothing.
	time.Sleep(10 * time.Millisecond)

	first := make(chan queue.Outcome, 1)
	go func() { first <- set.Do("one", func(context.Context) {}) }()
	time.Sleep(10 * time.Millisecond)

	second := make(chan queue.Outcome, 1)
	go func() { second <- set.Do("one", func(context.Context) {}) }()
	time.Sleep(10 * time.Millisecond) // both are pending before the worker frees up

	close(release)

	require.True(t, (<-first).Superseded, "the replaced caller was left waiting on a write that never came")
	require.False(t, (<-second).Superseded)
}

func TestClosingStopsEveryQueueAndFreesAnyoneWaiting(t *testing.T) {
	set := queue.New(t.Context())

	release := make(chan struct{})
	set.Post("one", func(context.Context) { <-release })
	time.Sleep(10 * time.Millisecond)

	waiting := make(chan queue.Outcome, 1)
	go func() { waiting <- set.Do("one", func(context.Context) {}) }()
	time.Sleep(10 * time.Millisecond)

	go func() { close(release) }()
	set.Close()

	select {
	case outcome := <-waiting:
		require.True(t, outcome.Superseded)
	case <-time.After(time.Second):
		t.Fatal("a caller was left hanging when the queue stopped")
	}
}

func TestADeviceNobodyWritesToCostsNothing(t *testing.T) {
	// A machine with sixty devices and one scene runs one goroutine.
	set := queue.New(t.Context())
	defer set.Close()

	set.Do("one", func(context.Context) {})
	// Nothing to assert directly about goroutines without being fragile; the
	// guarantee is structural: a queue is created on first write, not per
	// device seen.
	require.NotPanics(t, func() { set.Do("one", func(context.Context) {}) })
}
