// Package version carries the build's version string.
//
// Stamped by the Makefile from .tag through -ldflags. A build that skips the
// Makefile reports "dev", which is the truth rather than a guess at a number.
package version

// Version is the release this binary was built from, or "dev".
var Version = "dev"
