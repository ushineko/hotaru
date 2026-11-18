package gui_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/ushineko/hotaru/internal/gui"
)

/*
In the tests a worker's hand-back to the screen waits for Settle, and is drawn
by whoever settles, on their goroutine: as the window's driver draws on its
one thread, where Fyne's test driver drew in place, beside the test (spec 079,
#219).
*/
func TestAHandBackDrawsOnlyWhenSettledOnTheSettlersGoroutine(t *testing.T) {
	gui.Settle() // anything an earlier test left
	drawn := make(chan bool, 1)
	settling := false
	done := make(chan struct{})
	go func() {
		defer close(done)
		gui.OnScreen(func() { drawn <- settling })
	}()
	<-done
	require.Equal(t, 1, gui.Queued(), "the hand-back was not queued")
	select {
	case <-drawn:
		t.Fatal("the hand-back drew before anything settled")
	default:
	}

	settling = true
	gui.Settle()
	select {
	case during := <-drawn:
		require.True(t, during, "the hand-back did not draw inside Settle, on its caller")
	default:
		t.Fatal("Settle did not draw the hand-back")
	}
	require.Zero(t, gui.Queued())
}
