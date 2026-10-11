//go:build !windows

package openrgb

import (
	"errors"
	"syscall"
)

// reset is whether an error is the connection having been torn down under
// the client: a write into a closed socket, or a reset from the far end.
func reset(err error) bool {
	return errors.Is(err, syscall.EPIPE) || errors.Is(err, syscall.ECONNRESET)
}

// PeerGone is what this system reports on a socket whose peer has gone. It
// is for a test that makes the Fake vanish the way a real server does.
var PeerGone error = syscall.EPIPE
