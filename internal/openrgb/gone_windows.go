package openrgb

import (
	"errors"

	"golang.org/x/sys/windows"
)

/*
reset is whether an error is the connection having been torn down under the
client.

Windows does not report the errno values Unix does. A server that was killed
shows as WSAECONNRESET on the next read and WSAECONNABORTED on a write after
it, and neither matches syscall.ECONNRESET or syscall.EPIPE, which on Windows
are invented numbers no socket call returns.
*/
func reset(err error) bool {
	return errors.Is(err, windows.WSAECONNRESET) || errors.Is(err, windows.WSAECONNABORTED)
}

// PeerGone is what this system reports on a socket whose peer has gone. It
// is for a test that makes the Fake vanish the way a real server does.
var PeerGone error = windows.WSAECONNRESET
