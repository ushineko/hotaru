/*
Package daemon runs the service.

The resident half: it owns the socket, the connection to OpenRGB, and -- later
-- desired state and the reconcilers. Separated from the CLI so that the client
commands can be proved unable to reach a device, and so that the service's
dependencies are not carried by a process that only wanted to print a table.
*/
package daemon

import (
	"context"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"
	"github.com/ushineko/hotaru/internal/api"
	"github.com/ushineko/hotaru/internal/config"
	"github.com/ushineko/hotaru/internal/cooler"
	"github.com/ushineko/hotaru/internal/dashboard"
	"github.com/ushineko/hotaru/internal/desktop"
	"github.com/ushineko/hotaru/internal/images"
	"github.com/ushineko/hotaru/internal/openrgb"
	"github.com/ushineko/hotaru/internal/queue"
	"github.com/ushineko/hotaru/internal/scenes"
	"github.com/ushineko/hotaru/internal/service"
	"github.com/ushineko/hotaru/internal/state"
	"github.com/ushineko/hotaru/internal/systemd"
	"github.com/ushineko/hotaru/internal/version"
)

// Command is `hotaru serve`.
func Command() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "serve",
		Short: "Run the service",
		Long: `Run the service.

Starts whatever else is or is not running. An OpenRGB server that is absent, a
machine with no configuration, hardware that has not enumerated yet: each is
reported by ` + "`hotaru light health`" + ` and none of them stop the service
serving, because a program that refuses to start has nothing to tell you with.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return run(cmd)
		},
	}
	cmd.Flags().String("openrgb", openrgb.DefaultAddress, "the OpenRGB server to drive")
	return cmd
}

func run(cmd *cobra.Command) error {
	ctx := cmd.Context()

	socket, _ := cmd.Flags().GetString("socket")
	if socket == "" {
		got, err := config.SocketPath()
		if err != nil {
			return err
		}
		socket = got
	}

	rules, err := config.RulesPath()
	if err != nil {
		return err
	}
	cfg, problems, err := config.Load(rules)
	if err != nil {
		// An unreadable rules file is worth saying loudly and is not fatal:
		// the defaults drive every device present, which is what a machine
		// with no file at all gets.
		cmd.PrintErrf("hotaru: %v\n", err)
		cfg = nil
	}
	for _, problem := range problems {
		cmd.PrintErrf("hotaru: %s\n", problem.Error())
	}

	address, _ := cmd.Flags().GetString("openrgb")
	svc := service.New(cfg, nil, address)
	svc.SetRulesPath(rules)

	// Desired state: what the lights were last asked to show. An absent file
	// is the normal starting condition, and means there is nothing to restore.
	statePath, err := config.StatePath()
	if err != nil {
		return err
	}
	desired, err := state.Open(statePath)
	if err != nil {
		return err
	}
	svc.SetRecorder(desired)
	defer func() { _ = desired.Flush() }()

	/*
		Named scenes, if the file can be read.

		An unreadable scenes file is reported and the service carries on
		without scenes: it is somebody's saved work and is never rewritten, so
		the recovery is a person fixing their YAML rather than hotaru
		discarding it. Everything else -- lighting, the cooler, the API --
		works meanwhile.
	*/
	scenesPath, err := config.ScenesPath()
	if err != nil {
		return err
	}
	if saved, err := scenes.Open(scenesPath); err != nil {
		cmd.PrintErrf("hotaru: %v\n", err)
	} else {
		svc.SetScenes(saved)
	}

	/*
		The dashboards, on the same terms as the scenes: somebody's saved
		work, reported and never rewritten when it will not parse. A machine
		without them draws the shipped one, which is what it drew before
		there was a file at all.
	*/
	if path, err := config.DashboardsPath(); err != nil {
		cmd.PrintErrf("hotaru: no dashboards: %v\n", err)
	} else if boards, err := dashboard.Open(path); err != nil {
		cmd.PrintErrf("hotaru: %v\n", err)
	} else {
		svc.SetDashboards(boards)
	}

	/*
		The picture library, if its directory can be made.

		Under $XDG_DATA_HOME: files somebody added rather than settings they
		chose. A machine where it cannot be opened loses pictures and nothing
		else, which is why this is reported and not fatal.
	*/
	if dir, err := config.DataDir(); err != nil {
		cmd.PrintErrf("hotaru: no image library: %v\n", err)
		svc.NoImages(err)
	} else if library, err := images.Open(filepath.Join(dir, "images")); err != nil {
		// Told to the service as well as to the log. Somebody adding a
		// picture an hour later is not reading the startup output, and
		// "unavailable on this machine" without the reason is a dead end.
		cmd.PrintErrf("hotaru: no image library: %v\n", err)
		svc.NoImages(err)
	} else {
		svc.SetImages(library)
	}

	// One goroutine per device, created on first write. A reconcile and a
	// user's scene cannot interleave on the same device, and a write that is
	// superseded before it runs is replaced rather than queued behind the
	// thing that countermanded it.
	writes := queue.New(ctx)
	defer writes.Close()
	svc.SetQueue(writes)
	svc.SetEnvironment(environment{})

	/*
		The cooler, if this machine has one.

		Opened once and owned for the life of the service: it is a handle on a
		HID endpoint, and two callers on one endpoint interleave control
		transfers. Absence is ordinary -- most machines have no liquid cooler,
		and a machine that does may have one hotaru does not recognise -- so
		it is reported once and everything else carries on.
	*/
	report := func(format string, args ...any) { cmd.PrintErrf(format+"\n", args...) }

	if found, err := cooler.Open(ctx); err != nil {
		cmd.Printf("no cooler telemetry: %v\n", err)
	} else {
		owner := cooler.Own(found)
		svc.SetCooler(owner)
		cmd.Printf("reading %s at %s\n", found.Device().Name, found.Device().HID)

		/*
			The dashboard, if the cooler has a screen.

			Stopped before the cooler is closed, and deliberately in that
			order: closing hands the panel back to the firmware's own
			readout, and a push that arrived afterwards would leave hotaru's
			last frame on somebody's cooler for as long as the machine stayed
			off. The defers run bottom-up, so this one is registered after
			the close it must precede.
		*/
		panel := dashboard.NewPusher(owner, svc.Readings)
		panel.Trails = svc.Trails
		panel.Look = svc.Look
		panel.Report = report
		svc.SetDashboard(panel)

		drawn := make(chan struct{})
		go func() { defer close(drawn); panel.Run(ctx) }()
		defer func() { <-drawn; _ = owner.Close() }()
	}

	/*
		The desktop, if this machine has one.

		An attachment, never a dependency. hotaru starts with the machine and
		a session arrives later or not at all, so the D-Bus door goes up when
		there is a bus to put it on and the KWin script is installed on every
		appearance of KWin -- at login, and again whenever it restarts, because
		the shortcuts live exactly as long as the loaded script.

		Everything here failing is a machine without hotkeys and with
		everything else working, which is what a desktop that is not Plasma
		gets as well.
	*/
	keys(ctx, cmd, svc, report)

	listener, err := api.Listen(ctx, socket)
	if err != nil {
		return err
	}
	cmd.Printf("hotaru %s listening on %s\n", version.Version, socket)

	// The OpenRGB server is a resource that appears, not a dependency that is
	// satisfied: it may start after this does, or never. Connecting happens in
	// the background and keeps trying, so the API is up either way.
	go func() {
		connect(ctx, svc, address, report)
		// Restoring waits for a server rather than being ordered after one:
		// there is no unit to order against, and "started" is not "ready".
		(&Reconciler{Service: svc, Report: report}).Run(ctx)
	}()

	return api.Serve(ctx, listener, svc)
}

/*
environment answers "what could I do about it" from systemd.

Here rather than in the service because it shells out, and because a machine
without systemd should lose the suggestions and nothing else.
*/
type environment struct{}

func (environment) Remedies(ctx context.Context) []string {
	return systemd.Look(ctx).Remedies()
}

// connectBackoff is how long to wait between attempts to reach OpenRGB, and
// how long to keep waiting. Short enough to catch a server that starts moments
// later at boot; long enough not to be a busy loop for a machine that has none.
var connectBackoff = []time.Duration{
	time.Second, 2 * time.Second, 5 * time.Second, 15 * time.Second, 30 * time.Second,
}

func connect(ctx context.Context, svc *service.Service, address string, report func(string, ...any)) {
	var announced bool
	for attempt := 0; ; attempt++ {
		conn, err := openrgb.Dial(ctx, address)
		if err == nil {
			svc.SetClient(conn)
			report("connected to the OpenRGB server at %s (protocol %d)", address, conn.ProtocolVersion())
			return
		}
		if ctx.Err() != nil {
			return
		}
		if !announced {
			// Once, not every tick: a machine without OpenRGB should not have
			// its journal filled with a fact that is not changing.
			report("no OpenRGB server at %s yet; lighting waits for one", address)
			announced = true
		}

		wait := connectBackoff[min(attempt, len(connectBackoff)-1)]
		select {
		case <-ctx.Done():
			return
		case <-time.After(wait):
		}
	}
}

/*
keys puts hotaru's scenes on the desktop's shortcuts.

Two halves, and each fails on its own terms. The **door** is a D-Bus object
with one method, and it exists because a KWin script can reach the outside
world through `callDBus` and nothing else. The **script** is installed on every
appearance of KWin rather than once at start-up: the shortcuts live exactly as
long as the loaded script, so a KWin restart takes them, and the implementation
this replaces spent the rest of each session believing it still had them.

A machine with no session bus, no KWin, or another hotaru already holding the
name gets no hotkeys and everything else. The reason is recorded so that
`hotaru keys` can say it rather than showing an empty table.
*/
func keys(ctx context.Context, cmd *cobra.Command, svc *service.Service, report func(string, ...any)) {
	conn, err := desktop.Session()
	if err != nil {
		svc.SetDesktop(err.Error())
		cmd.Printf("no hotkeys: %v\n", err)
		return
	}
	go func() {
		<-ctx.Done()
		_ = conn.Close()
	}()

	if err := desktop.Export(ctx, conn, applier{svc}, report); err != nil {
		svc.SetDesktop(err.Error())
		cmd.Printf("no hotkeys: %v\n", err)
		return
	}

	runtime, err := config.RuntimeDir()
	if err != nil {
		svc.SetDesktop(err.Error())
		return
	}
	install := &desktop.Installer{
		KWin:     desktop.NewKWin(conn),
		Dir:      filepath.Join(runtime, "keys"),
		Bindings: func() map[string]string { return bindings(svc) },
		Report:   report,
	}
	/*
		The installer is also how a rebinding reaches the desktop.

		KWin's script carries the scene name in the call it makes rather than
		the key, so changing a binding in the file changes nothing until the
		script is written again. The service says when the file changed; this
		is what it calls.
	*/
	svc.SetShortcuts(install, report)
	svc.SetDesktop("waiting for the desktop")

	go desktop.Attach(ctx, &desktop.BusWatcher{Conn: conn, Name: desktop.KWinName},
		func() (int, error) {
			count, err := install.Install()
			if err != nil {
				svc.SetDesktop(err.Error())
				return 0, err
			}
			svc.SetDesktop("")
			return count, nil
		}, report)
}

// bindings are the keys as the service has them, or none if scenes are
// unavailable -- in which case there is nothing to bind them to anyway.
func bindings(svc *service.Service) map[string]string {
	keys, err := svc.Keys()
	if err != nil {
		return nil
	}
	out := make(map[string]string, len(keys.Bindings))
	for _, binding := range keys.Bindings {
		out[binding.Key] = binding.Scene
	}
	return out
}

// applier narrows the service to the one method a keypress needs, so the
// D-Bus door cannot grow into a second API by accident.
type applier struct{ svc *service.Service }

func (a applier) ApplyScene(ctx context.Context, name string) (string, error) {
	return a.svc.ApplyByName(ctx, name)
}
