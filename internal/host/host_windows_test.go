package host

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"golang.org/x/sys/windows/svc"
)

// running starts the handler as the service manager would, and returns what
// it reports and how it ends.
func running(t *testing.T, main func(ctx context.Context) int) (chan<- svc.ChangeRequest, <-chan svc.Status, <-chan uint32, *handler) {
	t.Helper()
	requests := make(chan svc.ChangeRequest)
	status := make(chan svc.Status, 8)
	ended := make(chan uint32, 1)
	h := &handler{main: main, wait: time.Second}
	go func() {
		_, code := h.Execute(nil, requests, status)
		ended <- code
	}()
	require.Equal(t, svc.StartPending, (<-status).State)
	up := <-status
	require.Equal(t, svc.Running, up.State)
	require.Equal(t, svc.AcceptStop|svc.AcceptShutdown, up.Accepts)
	return requests, status, ended, h
}

// waitsForCancel is a main that serves until it is told to stop.
func waitsForCancel(ctx context.Context) int {
	<-ctx.Done()
	return 0
}

func TestAStopCancelsMainAndIsNotAFailure(t *testing.T) {
	for _, cmd := range []svc.Cmd{svc.Stop, svc.Shutdown} {
		requests, status, ended, h := running(t, waitsForCancel)
		now := svc.Status{State: svc.Running, Accepts: accepts}
		requests <- svc.ChangeRequest{Cmd: svc.Interrogate, CurrentStatus: now}
		require.Equal(t, now, <-status, "an interrogation is answered with the state")
		requests <- svc.ChangeRequest{Cmd: cmd}
		require.Equal(t, svc.StopPending, (<-status).State)
		require.Equal(t, uint32(0), <-ended)
		require.Equal(t, 0, h.code)
	}
}

func TestAMainThatFailsByItselfIsAFailure(t *testing.T) {
	_, status, ended, h := running(t, func(context.Context) int { return 1 })
	require.Equal(t, svc.StopPending, (<-status).State)
	require.Equal(t, uint32(1), <-ended, "so Windows' recovery starts it again")
	require.Equal(t, 1, h.code)
}

func TestAMainThatWillNotStopIsLeftAfterTheWait(t *testing.T) {
	stuck := make(chan struct{})
	t.Cleanup(func() { close(stuck) })
	requests, status, ended, h := running(t, func(context.Context) int { <-stuck; return 0 })
	requests <- svc.ChangeRequest{Cmd: svc.Stop}
	require.Equal(t, svc.StopPending, (<-status).State)
	require.Equal(t, uint32(0), <-ended)
	require.Equal(t, 1, h.code, "the process exit says it did not end cleanly")
}

func TestAServiceThatDoesNotExistIsNotInstalled(t *testing.T) {
	require.False(t, Installed("hotaru-test-no-such-service"))
	require.True(t, Installed("EventLog"), "a service every Windows has")
}
