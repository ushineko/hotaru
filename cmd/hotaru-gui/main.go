/*
Command hotaru-gui is hotaru's window.

	hotaru-gui                  the control surface
	HOTARU_PPROF=:6060 hotaru-gui    with a profiling endpoint on loopback

A client of the service and nothing else. It holds no device handle, which is
not a policy it follows but a property of what it imports: everything it shows
is a request over the same socket the CLI uses, so a second copy of this window
cannot contend with the first and neither can contend with a shell.

See specs/017-the-window-and-what-it-shows.md.
*/
package main

import (
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/ushineko/fynedesygn/profiling"
	"github.com/ushineko/fynedesygn/shell"
	fdtheme "github.com/ushineko/fynedesygn/theme"
	"github.com/ushineko/hotaru/internal/api"
	"github.com/ushineko/hotaru/internal/config"
	"github.com/ushineko/hotaru/internal/gui"
)

func main() { os.Exit(run()) }

func run() int {
	socket := flag.String("socket", "",
		"the service's socket (default: $XDG_RUNTIME_DIR/hotaru/hotaru.sock)")
	/*
		Which section to open on, and a colour scheme for this run.

		For a window somebody opens to look at one thing -- and for the
		screenshot script, which starts a window per image and cannot click.
		The scheme is not saved, so a capture in Breeze Dark does not
		overwrite whatever this desk had chosen.
	*/
	section := flag.String("section", "",
		"open on this section: "+strings.Join(gui.SectionNames(), ", "))
	scheme := flag.String("scheme", "",
		"colour scheme for this run, not saved: "+strings.Join(fdtheme.SchemeNames(), ", "))
	flag.Parse()

	path := *socket
	if path == "" {
		found, err := config.SocketPath()
		if err != nil {
			fmt.Fprintln(os.Stderr, "hotaru-gui: "+err.Error())
			return 1
		}
		path = found
	}

	/*
		Profiling, when asked for, and a soft memory ceiling.

		Off unless HOTARU_PPROF names an address, and loopback whatever it
		says. The ceiling is what stops the arena growing to fit a burst and
		keeping it; it defers to GOMEMLIMIT and HOTARU_MEMLIMIT can change or
		switch it off. See fynedesygn's docs/performance.md.

		The number is a starting point and not a measurement: this window has
		not been profiled yet, and 512 MiB is several times anything it is
		known to hold. It is the first thing the profile should correct.
	*/
	report := func(line string) { fmt.Fprintln(os.Stderr, "hotaru-gui: "+line) }
	defer profiling.FromEnv("HOTARU_PPROF", report)()
	profiling.Limit(memoryCeiling, "HOTARU_MEMLIMIT", report)

	opts := gui.New(api.NewClient(path)).Options(path)
	opts.Scheme = *scheme
	gui.OpenOn(&opts, *section)
	shell.Run(opts)
	return 0
}

// memoryCeiling is the soft limit the window runs under. Soft: Go collects
// harder rather than failing an allocation, so a machine doing something this
// number did not anticipate degrades into GC and not a crash.
const memoryCeiling = 512 << 20
