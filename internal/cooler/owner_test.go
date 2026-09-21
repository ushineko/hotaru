package cooler

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestConcurrentReadersShareOneExchange(t *testing.T) {
	/*
		A dashboard, a status endpoint and a telemetry consumer asking at the
		same moment want the same number, and the device should be written to
		once. Superseding -- lighting's rule -- is wrong here: a reader whose
		request was dropped still wants an answer.
	*/
	fake := NewFake()
	o := Own(NewWithFake(fake))

	var wg sync.WaitGroup
	for range 20 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := o.Status(context.Background())
			require.NoError(t, err)
		}()
	}
	wg.Wait()

	require.Len(t, fake.Told, 1, "the cooler was asked more than once for one moment's reading")
}

func TestAStaleReadingIsTakenAgain(t *testing.T) {
	// Freshness is a coalescing window, not a cache: a dashboard showing a
	// number from a minute ago is lying.
	fake := NewFake()
	o := Own(NewWithFake(fake))
	o.fresh = time.Millisecond

	_, err := o.Status(context.Background())
	require.NoError(t, err)
	time.Sleep(5 * time.Millisecond)
	_, err = o.Status(context.Background())
	require.NoError(t, err)

	require.Len(t, fake.Told, 2)
}

func TestAFailedReadingIsNotRemembered(t *testing.T) {
	/*
		A device that failed once is asked again rather than written off for a
		quarter of a second: the next caller may be somebody who has just
		plugged it back in.
	*/
	fake := NewFake()
	fake.Silent = true
	o := Own(NewWithFake(fake))

	_, err := o.Status(context.Background())
	require.Error(t, err)

	fake.Silent = false
	status, err := o.Status(context.Background())
	require.NoError(t, err, "a transient failure was cached")
	require.InDelta(t, 37.5, status.Coolant, 0.05)
}

func TestReadsAreSerialised(t *testing.T) {
	// Two callers writing to one interrupt endpoint interleave control
	// transfers, which corrupts rather than merely delaying.
	fake := NewFake()
	o := Own(NewWithFake(fake))
	o.fresh = 0 // every call reaches the device

	var wg sync.WaitGroup
	for range 50 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _ = o.Status(context.Background())
		}()
	}
	wg.Wait()
	require.Len(t, fake.Told, 50, "some exchanges were lost, which means they overlapped")
}
