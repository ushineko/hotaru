/*
Command hotaru-gui is hotaru's window.

	hotaru-gui    the control surface: service, system, cooling

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

	"github.com/ushineko/fynedesygn/shell"
	"github.com/ushineko/hotaru/internal/api"
	"github.com/ushineko/hotaru/internal/config"
	"github.com/ushineko/hotaru/internal/gui"
)

func main() { os.Exit(run()) }

func run() int {
	socket := flag.String("socket", "",
		"the service's socket (default: $XDG_RUNTIME_DIR/hotaru/hotaru.sock)")
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

	shell.Run(gui.New(api.NewClient(path)).Options(path))
	return 0
}
