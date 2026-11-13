//go:build !linux && !windows

package services

import "context"

// bounce cannot restart what it cannot see. The caller offers the command.
func bounce(context.Context, Facts) error { return ErrNotRestartable }
