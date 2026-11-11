package host

import (
	"context"
	"errors"
	"time"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/svc"
	"golang.org/x/sys/windows/svc/eventlog"
)

// Managed reports whether this process is a service of the Windows service
// manager.
func Managed() bool {
	ok, err := svc.IsWindowsService()
	return err == nil && ok
}

/*
Installed reports whether Windows has a service of this name, so that a
client run by a person finds the installed service's files and socket
(spec 072 R3.2).

Asked with the rights of any user. A service this user may not even ask
about is there all the same.
*/
func Installed(name string) bool {
	m, err := windows.OpenSCManager(nil, nil, windows.SC_MANAGER_CONNECT)
	if err != nil {
		return false
	}
	defer func() { _ = windows.CloseServiceHandle(m) }()
	p, err := windows.UTF16PtrFromString(name)
	if err != nil {
		return false
	}
	s, err := windows.OpenService(m, p, windows.SERVICE_QUERY_STATUS)
	if err == nil {
		_ = windows.CloseServiceHandle(s)
		return true
	}
	return errors.Is(err, windows.ERROR_ACCESS_DENIED)
}

// stopWait is how long a stop waits for main to return. The Windows service
// manager gives a service 30 seconds; this leaves it room to say stopped.
const stopWait = 20 * time.Second

func runManaged(name string, main func(ctx context.Context) int) int {
	h := &handler{main: main, wait: stopWait}
	if err := svc.Run(name, h); err != nil {
		return 1
	}
	return h.code
}

// handler answers the service manager for main (spec 072 R1.4).
type handler struct {
	main func(ctx context.Context) int
	wait time.Duration
	code int
}

const accepts = svc.AcceptStop | svc.AcceptShutdown

/*
Execute is the service, as the service manager runs it.

It says the service runs as soon as main has started: hotaru serves its
socket at once and reaches devices as they appear, so there is nothing to
wait for. A stop or a shutdown cancels main's context, and main gets wait to
return. A main that ends by itself with a code other than 0 is a failure,
which is what makes Windows start it again.
*/
func (h *handler) Execute(_ []string, requests <-chan svc.ChangeRequest, status chan<- svc.Status) (bool, uint32) {
	status <- svc.Status{State: svc.StartPending}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan int, 1)
	go func() { done <- h.main(ctx) }()
	status <- svc.Status{State: svc.Running, Accepts: accepts}

	for {
		select {
		case code := <-done:
			h.code = code
			status <- svc.Status{State: svc.StopPending}
			return false, exitCode(code)
		case r := <-requests:
			switch r.Cmd {
			case svc.Interrogate:
				status <- r.CurrentStatus
			case svc.Stop, svc.Shutdown:
				status <- svc.Status{State: svc.StopPending}
				cancel()
				select {
				case h.code = <-done:
				case <-time.After(h.wait):
					h.code = 1
				}
				// Asked to stop, so stopping is not a failure.
				return false, 0
			default:
			}
		}
	}
}

func exitCode(code int) uint32 {
	if code <= 0 {
		return 0
	}
	return 1
}

/*
Report records msg where a person can find it when the service cannot even
open its own log: the Windows event log, under the service's name, as an
error. Event Viewer shows it under Windows Logs > Application.
*/
func Report(name, msg string) {
	l, err := eventlog.Open(name)
	if err != nil {
		return
	}
	defer func() { _ = l.Close() }()
	_ = l.Error(1, msg)
}
