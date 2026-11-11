//go:build !windows

package config

// PrepareMachine does nothing: only Windows has directories of the machine
// for hotaru (spec 072 R3).
func PrepareMachine() error { return nil }

// SharedSocketAccess does nothing: only Windows has a shared socket.
func SharedSocketAccess(string) error { return nil }
