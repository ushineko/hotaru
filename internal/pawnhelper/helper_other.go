//go:build !windows

package pawnhelper

import (
	"context"
	"errors"

	"github.com/ushineko/sanshoku"
)

// errNotWindows is the helper anywhere but Windows, where sanshoku reaches
// these devices without PawnIO.
var errNotWindows = errors.New("hotaru's PawnIO helper is for Windows")

// Main is refused.
func Main(context.Context, string) error { return errNotWindows }

// Driver is nil: there is no helper here.
func Driver() sanshoku.Driver { return nil }
