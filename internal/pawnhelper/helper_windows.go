package pawnhelper

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"

	"github.com/ushineko/sanshoku"
	"github.com/ushineko/sanshoku/corsairdram"
	"github.com/ushineko/sanshoku/msi"
	"github.com/ushineko/sanshoku/pawnio"

	"github.com/ushineko/hotaru/internal/host"
)

// account is the one account besides SYSTEM and administrators that may
// open the pipe: the installed service's own.
const account = `NT SERVICE\hotaru`

/*
Main runs the helper until ctx ends. allow is the SID that may open the
pipe; empty is the installed service's. It runs as the service manager
starts it (fynstall.yaml), or from an administrator's terminal to try it.
*/
func Main(ctx context.Context, allow string) error {
	// PawnIO's library is loaded by its full path; what it loads in turn
	// comes from System32 alone, not the machine's PATH, as this runs as
	// LocalSystem.
	if err := windows.SetDefaultDllDirectories(windows.LOAD_LIBRARY_SEARCH_SYSTEM32); err != nil {
		return fmt.Errorf("limiting where libraries load from: %w", err)
	}
	if allow == "" {
		sid, _, _, err := windows.LookupSID("", account)
		if err != nil {
			return fmt.Errorf("finding %s, which the installer makes: %w", account, err)
		}
		allow = sid.String()
	}
	sid, err := windows.StringToSid(allow)
	if err != nil {
		return fmt.Errorf("%q is not an account's SID: %w", allow, err)
	}
	self, err := os.Executable()
	if err != nil {
		return fmt.Errorf("finding this program: %w", err)
	}
	dir := filepath.Dir(self)
	var epoch [4]byte
	_, _ = rand.Read(epoch[:])
	h := &handler{epoch: hex.EncodeToString(epoch[:]), scan: func(ctx context.Context) ([]sanshoku.Device, error) { return scan(ctx, dir) }}
	defer h.close()
	// The service's account reads and writes, and may not make an instance
	// of the pipe (FILE_CREATE_PIPE_INSTANCE, which GW would give): only
	// the helper serves it.
	return listen(ctx, "D:P(A;;GA;;;SY)(A;;GA;;;BA)(A;;0x12019b;;;"+sid.String()+")", h)
}

// scan finds the devices sanshoku reaches through PawnIO, after checking the
// modules it would load.
func scan(ctx context.Context, dir string) ([]sanshoku.Device, error) {
	for name, want := range modules {
		b, err := os.ReadFile(filepath.Join(dir, name)) //nolint:gosec // the program's own directory, in Program Files
		if err != nil {
			return nil, fmt.Errorf("the PawnIO module %s: %w", name, err)
		}
		sum := sha256.Sum256(b)
		if hex.EncodeToString(sum[:]) != want {
			return nil, fmt.Errorf("the PawnIO module %s is not the one this hotaru was built with: install hotaru again", name) //nolint:err113 // said plainly
		}
	}
	pawnio.ModuleDir = dir
	found, err := sanshoku.Scan(ctx, msi.Board{}, corsairdram.Driver{})
	var out []sanshoku.Device
	for _, c := range found {
		dev, oerr := c.Open(ctx)
		if oerr != nil {
			err = errors.Join(err, oerr)
			continue
		}
		out = append(out, dev)
	}
	if len(out) == 0 && errors.Is(err, pawnio.ErrNoPawnIO) {
		return nil, fmt.Errorf("PawnIO is not installed (winget install namazso.PawnIO): %w", err)
	}
	return out, err
}

const (
	// pipeInstances is room for a connection per device and the list's.
	pipeInstances = maxDevices + 2
	// retry is how long the helper waits before trying again for a pipe it
	// could not make: one another program holds, or every instance in use.
	retry = 5 * time.Second
)

/*
listen serves the pipe until ctx ends: each connection in turn of its own,
answered by h. The pipe is the helper's alone (the first instance), reads
and writes for the account sddl names, and refuses remote clients.

A pipe it cannot make is waited for, not given up on: a program that took
the name while the helper was stopped, or connections filling every
instance, would otherwise stop it, and the service manager would start it
again into the same refusal. Stopping does not wait for a connection's read
to end: the process is ending, and a synchronous read on a pipe ends with it.
*/
func listen(ctx context.Context, sddl string, h *handler) error {
	sd, err := windows.SecurityDescriptorFromString(sddl)
	if err != nil {
		return fmt.Errorf("the pipe's access list: %w", err)
	}
	next, err := instance(sd, true)
	for said := false; err != nil; next, err = instance(sd, true) {
		if !said {
			host.Report(Service, fmt.Sprintf("waiting for the pipe %s, which another program holds: %v", Pipe, err))
			said = true
		}
		select {
		case <-ctx.Done():
			return nil
		case <-time.After(retry):
		}
	}
	stop := context.AfterFunc(ctx, func() {
		// The accept below is blocked waiting for a client; be one, so it
		// sees it is done.
		if c, err := open(Pipe); err == nil {
			_ = windows.CloseHandle(c)
		}
	})
	defer stop()
	for {
		connected := windows.ConnectNamedPipe(next, nil)
		if ctx.Err() != nil {
			closePipe(next)
			return nil
		}
		conn := next
		for next, err = instance(sd, false); err != nil; next, err = instance(sd, false) {
			select {
			case <-ctx.Done():
				closePipe(conn)
				return nil
			case <-time.After(retry):
			}
		}
		if connected != nil && !errors.Is(connected, windows.ERROR_PIPE_CONNECTED) {
			closePipe(conn)
			continue
		}
		go func() {
			f := os.NewFile(uintptr(conn), Pipe)
			cctx, cancel := context.WithCancel(ctx)
			defer cancel()
			// A read in progress is synchronous; disconnecting the pipe from
			// here ends it.
			context.AfterFunc(cctx, func() { _ = windows.DisconnectNamedPipe(conn) })
			_ = h.serveConn(cctx, f)
			_ = windows.DisconnectNamedPipe(conn)
			_ = f.Close()
		}()
	}
}

func instance(sd *windows.SECURITY_DESCRIPTOR, first bool) (windows.Handle, error) {
	name, err := windows.UTF16PtrFromString(Pipe)
	if err != nil {
		return 0, err //nolint:wrapcheck // a constant
	}
	flags := uint32(windows.PIPE_ACCESS_DUPLEX)
	if first {
		// Another program holding this name would be answered in our place.
		flags |= windows.FILE_FLAG_FIRST_PIPE_INSTANCE
	}
	sa := &windows.SecurityAttributes{Length: uint32(unsafe.Sizeof(windows.SecurityAttributes{})), SecurityDescriptor: sd} //nolint:gosec // the structure's own size
	h, err := windows.CreateNamedPipe(name, flags,
		windows.PIPE_TYPE_BYTE|windows.PIPE_READMODE_BYTE|windows.PIPE_WAIT|windows.PIPE_REJECT_REMOTE_CLIENTS,
		pipeInstances, maxLine, maxLine, 0, sa)
	if err != nil {
		return 0, fmt.Errorf("making the pipe %s: %w", Pipe, err)
	}
	return h, nil
}

func closePipe(h windows.Handle) {
	_ = windows.DisconnectNamedPipe(h)
	_ = windows.CloseHandle(h)
}

// open opens the pipe anonymously: its server may not act as the opener.
func open(pipe string) (windows.Handle, error) {
	name, err := windows.UTF16PtrFromString(pipe)
	if err != nil {
		return 0, err //nolint:wrapcheck // a constant
	}
	return windows.CreateFile(name, windows.GENERIC_READ|windows.GENERIC_WRITE, 0, nil, windows.OPEN_EXISTING, //nolint:wrapcheck // the caller says which
		windows.SECURITY_SQOS_PRESENT|windows.SECURITY_ANONYMOUS, 0)
}

var shared = &client{dial: dial}

// Driver is the helper's devices, for the service's canvas drivers.
func Driver() sanshoku.Driver { return driver{c: shared} }

/*
dial opens the pipe and believes it only if the process serving it is the
one the service manager started for hotaru-pawnio: a pipe's name is
anybody's to take while the helper is not running. Asking a service's state
is a right every service has.
*/
func dial(context.Context) (io.ReadWriteCloser, error) {
	pid, err := helperProcess()
	if errors.Is(err, windows.ERROR_SERVICE_DOES_NOT_EXIST) {
		return nil, errNoHelper
	}
	if err != nil {
		return nil, fmt.Errorf("asking after %s: %w", Service, err)
	}
	var h windows.Handle
	for range 3 {
		if h, err = open(Pipe); !errors.Is(err, windows.ERROR_PIPE_BUSY) {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if err != nil {
		return nil, fmt.Errorf("opening %s: %w", Pipe, err)
	}
	var server uint32
	err = windows.GetNamedPipeServerProcessId(h, &server)
	// Asked again after the open: the helper could have stopped, and its
	// process ID been taken, between the first question and the open.
	after, aerr := helperProcess()
	if err != nil || aerr != nil || server == 0 || server != pid || after != pid {
		_ = windows.CloseHandle(h)
		return nil, fmt.Errorf("the pipe %s is held by process %d, which is not %s", Pipe, server, Service) //nolint:err113 // the numbers are the message
	}
	return os.NewFile(uintptr(h), Pipe), nil
}

// helperProcess is the process the service manager started for the helper,
// or zero when it is not running.
func helperProcess() (uint32, error) {
	m, err := windows.OpenSCManager(nil, nil, windows.SC_MANAGER_CONNECT)
	if err != nil {
		return 0, err //nolint:wrapcheck // the caller says which
	}
	defer func() { _ = windows.CloseServiceHandle(m) }()
	name, _ := windows.UTF16PtrFromString(Service)
	s, err := windows.OpenService(m, name, windows.SERVICE_QUERY_STATUS)
	if err != nil {
		return 0, err //nolint:wrapcheck // the caller says which
	}
	defer func() { _ = windows.CloseServiceHandle(s) }()
	var status windows.SERVICE_STATUS_PROCESS
	var needed uint32
	if err := windows.QueryServiceStatusEx(s, windows.SC_STATUS_PROCESS_INFO,
		(*byte)(unsafe.Pointer(&status)), uint32(unsafe.Sizeof(status)), &needed); err != nil { //nolint:gosec // the structure Windows fills
		return 0, err //nolint:wrapcheck // the caller says which
	}
	return status.ProcessId, nil
}
