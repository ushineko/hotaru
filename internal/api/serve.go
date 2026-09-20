package api

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/ushineko/hotaru/internal/service"
)

// SocketMode is the socket's permissions: this user, nobody else.
//
// The API has no authentication, and this is why it does not need any. The
// boundary is the filesystem, the audience is one session, and a port would
// mean inventing an authentication scheme for a program that changes the colour
// of lights.
const SocketMode os.FileMode = 0o600

// maxSocketPath is the kernel's limit on a Unix socket address, including its
// terminator: sun_path is 108 bytes on Linux and less on some others.
const maxSocketPath = 104

/*
Listen opens the service's socket.

A socket left behind by a process that was killed is removed first — a stale
file is not a running service, and refusing to start because of one would mean a
crash costs a user their lighting until they know to delete a path they have
never heard of. A socket that something is actually listening on is a different
matter, and is reported rather than stolen.
*/
func Listen(ctx context.Context, socket string) (net.Listener, error) {
	// A Unix socket address is a fixed-size field in a kernel struct, and a
	// path that overruns it fails as "invalid argument" -- which sends someone
	// looking at their permissions, their directory and their sanity before
	// their path length. Say it plainly instead.
	if len(socket) >= maxSocketPath {
		return nil, fmt.Errorf("the socket path is %d characters and the limit is %d: %s",
			len(socket), maxSocketPath-1, socket)
	}
	if err := os.MkdirAll(filepath.Dir(socket), 0o700); err != nil {
		return nil, fmt.Errorf("make the runtime directory: %w", err)
	}

	if _, err := os.Stat(socket); err == nil {
		if live(socket) {
			return nil, fmt.Errorf("hotaru is already running on %s", socket)
		}
		if err := os.Remove(socket); err != nil {
			return nil, fmt.Errorf("remove the stale socket at %s: %w", socket, err)
		}
	}

	var config net.ListenConfig
	listener, err := config.Listen(ctx, "unix", socket)
	if err != nil {
		return nil, fmt.Errorf("listen on %s: %w", socket, err)
	}
	if err := os.Chmod(socket, SocketMode); err != nil {
		_ = listener.Close()
		return nil, fmt.Errorf("set permissions on %s: %w", socket, err)
	}
	return listener, nil
}

func live(socket string) bool {
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	var dialer net.Dialer
	conn, err := dialer.DialContext(ctx, "unix", socket)
	if err != nil {
		return false
	}
	_ = conn.Close()
	return true
}

/*
Serve runs the API until the context is cancelled.

The listener is closed on the way out, which removes the socket: a service that
has stopped should not leave something that looks like one behind.
*/
func Serve(ctx context.Context, listener net.Listener, svc *service.Service) error {
	server := &http.Server{
		Handler:           Handler(svc),
		ReadHeaderTimeout: ReadHeaderTimeout,
		IdleTimeout:       IdleTimeout,
	}

	done := make(chan error, 1)
	go func() { done <- server.Serve(listener) }()

	select {
	case <-ctx.Done():
		shutdown, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdown); err != nil {
			return fmt.Errorf("stop serving on %s: %w", listener.Addr(), err)
		}
		return nil
	case err := <-done:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return fmt.Errorf("serve on %s: %w", listener.Addr(), err)
	}
}
