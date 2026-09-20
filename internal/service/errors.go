package service

import (
	"errors"
	"fmt"

	"github.com/ushineko/hotaru/internal/devices"
)

/*
Unreachable is the OpenRGB server not being there.

Its own type because it is the one failure a caller should treat differently:
not a bug, not a bad request, just a daemon that is not running. The message
names the address and what to do, since "connection refused" tells a user
nothing they can act on.
*/
type Unreachable struct {
	Address string
}

func (e *Unreachable) Error() string {
	return fmt.Sprintf("no OpenRGB server at %s: start it, and lighting works from then on", e.Address)
}

func unreachable(address string) error { return &Unreachable{Address: address} }

func asUnsupported(err error, target **devices.Unsupported) bool {
	return errors.As(err, target)
}
