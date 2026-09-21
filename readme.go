/*
Package hotaru carries the front page, so the window can show it.

The documentation lives in one place. A program that restates its README in an
About pane has two descriptions of itself and only one of them is read by
whoever changes it -- and it is not the one in the window.

This package exists at the repository root because that is where README.md is,
and `go:embed` cannot reach above the directory it is written in.
*/
package hotaru

import (
	"embed"
)

//go:generate go run github.com/ushineko/fynedesygn/cmd/fynedesygn-mermaid -root .

/*
Diagrams are the repository's mermaid fences, rendered at development time.

Never at runtime: a program that showed a diagram would otherwise need node,
a browser and network access to draw a box with an arrow in it. `make
generate` renders them, keyed by a hash of the diagram's own text, so an
edited diagram produces a different file and a stale one is detected rather
than shown.
*/
//go:embed diagrams
var Diagrams embed.FS

// README is the front page, embedded.
//
//go:embed README.md
var README string
