package services

import (
	"context"
	"errors"
	"fmt"
	"time"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/svc"
	"golang.org/x/sys/windows/svc/mgr"
)

/*
openRGBService is the service OpenRGB's Windows installer registers. Service
names are matched without regard to case, so one name covers the spellings.
*/
const openRGBService = "OpenRGB"

/*
look asks the Windows service manager about OpenRGB, with the rights of any
user: whether the service exists, and whether it runs. Lingering is a systemd
idea, and a Windows service starts with the computer whoever signs in.
*/
func look(context.Context) Facts {
	facts := Facts{Manager: SCM, Lingering: true}
	state, found := scmState(openRGBService)
	if !found {
		return facts
	}
	facts.Installed, facts.Unit, facts.Active = true, openRGBService, state == svc.Running
	return facts
}

func scmState(name string) (svc.State, bool) {
	m, err := windows.OpenSCManager(nil, nil, windows.SC_MANAGER_CONNECT)
	if err != nil {
		return 0, false
	}
	defer func() { _ = windows.CloseServiceHandle(m) }()
	p, err := windows.UTF16PtrFromString(name)
	if err != nil {
		return 0, false
	}
	h, err := windows.OpenService(m, p, windows.SERVICE_QUERY_STATUS)
	if err != nil {
		return 0, false
	}
	defer func() { _ = windows.CloseServiceHandle(h) }()
	var status windows.SERVICE_STATUS
	if err := windows.QueryServiceStatus(h, &status); err != nil {
		return 0, false
	}
	return svc.State(status.CurrentState), true
}

// bounceTimeout is how long a restart is given: the service manager's own
// limit for a stop, and as long again for the start.
const bounceTimeout = 60 * time.Second

/*
bounce restarts the OpenRGB service where this account may.

A Windows service's access list lets Administrators and SYSTEM stop and
start it, and not other accounts, so the service of the machine, which runs
as its own account, is refused unless somebody granted it the right. Refused
is ErrNotRestartable, and the caller offers the command instead.
*/
func bounce(ctx context.Context, f Facts) error {
	if !f.Installed || f.Manager != SCM {
		return ErrNotRestartable
	}
	m, err := windows.OpenSCManager(nil, nil, windows.SC_MANAGER_CONNECT)
	if err != nil {
		return ErrNotRestartable
	}
	manager := &mgr.Mgr{Handle: m}
	defer func() { _ = manager.Disconnect() }()
	p, err := windows.UTF16PtrFromString(f.Unit)
	if err != nil {
		return ErrNotRestartable
	}
	h, err := windows.OpenService(m, p, windows.SERVICE_STOP|windows.SERVICE_START|windows.SERVICE_QUERY_STATUS)
	if errors.Is(err, windows.ERROR_ACCESS_DENIED) {
		return ErrNotRestartable
	}
	if err != nil {
		return fmt.Errorf("open the %s service: %w", f.Unit, err)
	}
	s := &mgr.Service{Name: f.Unit, Handle: h}
	defer func() { _ = s.Close() }()

	ctx, cancel := context.WithTimeout(ctx, bounceTimeout)
	defer cancel()
	if _, err := s.Control(svc.Stop); err != nil && !errors.Is(err, windows.ERROR_SERVICE_NOT_ACTIVE) {
		return fmt.Errorf("stop the %s service: %w", f.Unit, err)
	}
	if err := waitFor(ctx, s, svc.Stopped); err != nil {
		return fmt.Errorf("stop the %s service: %w", f.Unit, err)
	}
	if err := s.Start(); err != nil {
		return fmt.Errorf("start the %s service: %w", f.Unit, err)
	}
	if err := waitFor(ctx, s, svc.Running); err != nil {
		return fmt.Errorf("start the %s service: %w", f.Unit, err)
	}
	return nil
}

func waitFor(ctx context.Context, s *mgr.Service, want svc.State) error {
	for {
		st, err := s.Query()
		if err != nil {
			return fmt.Errorf("ask its state: %w", err)
		}
		if st.State == want {
			return nil
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("it did not get there in time: %w", ctx.Err())
		case <-time.After(250 * time.Millisecond):
		}
	}
}
