/*
Package daemon runs the service.

The resident half: it owns the socket, the devices hotaru draws, desired state
and the reconcilers. Separated from the CLI so that the client
commands can be proved unable to reach a device, and so that the service's
dependencies are not carried by a process that only wanted to print a table.
*/
package daemon

import (
	"context"
	"path/filepath"
	"runtime"
	"time"

	"github.com/spf13/cobra"
	"github.com/ushineko/hotaru/internal/api"
	"github.com/ushineko/hotaru/internal/canvas"
	"github.com/ushineko/hotaru/internal/config"
	"github.com/ushineko/hotaru/internal/cooler"
	"github.com/ushineko/hotaru/internal/dashboard"
	"github.com/ushineko/hotaru/internal/desktop"
	"github.com/ushineko/hotaru/internal/images"
	"github.com/ushineko/hotaru/internal/queue"
	"github.com/ushineko/hotaru/internal/scenes"
	"github.com/ushineko/hotaru/internal/service"
	"github.com/ushineko/hotaru/internal/state"
	"github.com/ushineko/hotaru/internal/version"
)

// Command is `hotaru serve`.
func Command() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "serve",
		Short: "Run the service",
		Long: `Run the service.

Starts whatever else is or is not running. A machine with no configuration,
hardware that has not enumerated yet, a cooler that cannot be opened: each is
reported by ` + "`hotaru light health`" + ` and none of them stop the service
serving, because a program that refuses to start has nothing to tell you with.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return run(cmd)
		},
	}
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

	svc := service.New(cfg)
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

	report := func(format string, args ...any) { cmd.PrintErrf(format+"\n", args...) }

	/*
		The cooler, when it can be opened.

		**A resource that appears**, like the devices below, and for a
		sharper reason than "it might be plugged in later". A `uaccess` udev
		rule grants its ACL to an active seat session, and `enable-linger`
		starts this service with the machine -- sixteen seconds before the
		login that makes the device openable, measured on a cold boot. Opened
		once, that one failure was permanent: the lights came back and the
		panel did not, until somebody restarted the service (#136).

		Held while it is there: sanshoku's driver owns the handle and
		serialises every exchange on it. A cooler that goes away is let go
		and waited for again (spec 059).
	*/
	cooled := make(chan struct{})
	go func() { defer close(cooled); attach(ctx, svc, cooler.Open, connectBackoff, report) }()
	defer func() { <-cooled }()

	/*
		The devices hotaru draws itself, on the same terms as the cooler: a
		resource that appears, attached when it can be opened and looked for
		again when it goes (spec 060). Most machines have none, and pay a
		sysfs scan every thirty seconds for it.
	*/
	drew := make(chan struct{})
	go func() {
		defer close(drew)
		attachCanvases(ctx, svc, canvas.Open, connectBackoff, report)
	}()
	defer func() { <-drew }()

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

	// Restoring waits for devices rather than being ordered after them: each
	// is a resource that appears, and "started" is not "found".
	go (&Reconciler{Service: svc, Report: report, Every: DefaultEvery}).Run(ctx)

	return api.Serve(ctx, listener, svc)
}

/*
attach waits for the cooler, then draws on it until the service stops, and
looks for it again if it goes away.

Absence is ordinary -- most machines have no liquid cooler, and a machine that
does may have one hotaru does not recognise -- so this keeps looking and says
nothing more after the first time. A machine with none pays a sysfs scan every
thirty seconds and nothing else.

A cooler that goes away (sanshoku.ErrGone) is detached, as absence, and the
same wait finds it again: before spec 059 the service held a dead handle until
somebody restarted it.
*/
func attach(ctx context.Context, svc *service.Service, open func(context.Context) (*cooler.Cooler, error),
	backoff []time.Duration, report func(string, ...any),
) {
	for {
		found := waitForCooler(ctx, open, backoff, report)
		if found == nil {
			return
		}
		if !serveCooler(ctx, svc, found, report) {
			return
		}
	}
}

/*
serveCooler gives the service one open cooler and draws the dashboard on it,
until the service stops or the cooler goes. It reports whether it went.

The panel stops before the cooler closes, and deliberately in that order:
closing hands the screen back to the firmware's own readout, and a push that
arrived afterwards would leave hotaru's last frame on somebody's cooler for as
long as the machine stayed off. The pusher is made afresh for each cooler,
because one that stopped on a panel it could not claim stays stopped.
*/
func serveCooler(ctx context.Context, svc *service.Service, found *cooler.Cooler, report func(string, ...any)) bool {
	drawing, stop := context.WithCancel(ctx)
	defer stop()

	svc.SetCooler(found)
	report("reading %s at %s", found.Device().Name, found.Device().HID)

	panel := dashboard.NewPusher(found, svc.Readings)
	panel.Trails = svc.Trails
	panel.Look = svc.Look
	panel.Report = report
	svc.SetDashboard(panel)

	drawn := make(chan struct{})
	go func() { defer close(drawn); panel.Run(drawing) }()

	// What the panel was last asked to show, as the lights are put back
	// (#184). After the dashboard has started, because a picture holds it.
	if err := svc.RestoreScreen(ctx); err != nil {
		report("could not put the screen back: %v", err)
	}
	// The cooler's own lights, drawn on its handle (spec 073).
	lit := make(chan struct{})
	go func() { defer close(lit); lightCooler(drawing, svc, found, connectBackoff, report) }()

	gone := false
	select {
	case <-ctx.Done():
	case <-found.Gone():
		gone = true
	}
	stop()
	<-drawn
	<-lit

	svc.SetDashboard(nil)
	svc.SetCooler(nil)
	_ = found.Close()
	return gone
}

/*
lightCooler draws on the cooler's lights as a canvas on the cooler's own handle
(spec 073), until the cooler goes or the service stops. A cooler whose
accessory list has not answered is asked again on the backoff, and a cooler
that has no lights is asked nothing: Borrow finds no canvas without a write.

The canvas is detached before this returns, so the cooler closes a handle
nothing else is using.
*/
func lightCooler(ctx context.Context, svc *service.Service, found *cooler.Cooler,
	backoff []time.Duration, report func(string, ...any),
) {
	for attempt := 0; ; attempt++ {
		if device, ok := canvas.Borrow(found.Shared()); ok {
			serveCanvas(ctx, svc, device, report)
			attempt = 0
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(backoff[min(attempt, len(backoff)-1)]):
		}
	}
}

/*
waitForCooler opens the cooler when it can be opened, or gives up when the
service stops.

`open` and `backoff` are arguments so that a test can drive this without a
cooler and without waiting seconds for it. The backoff attach hands it is the
one every device is looked for with: short enough to catch a
device that becomes readable moments later at boot, long enough not to be a
busy loop on a machine that has none.
*/
func waitForCooler(ctx context.Context, open func(context.Context) (*cooler.Cooler, error),
	backoff []time.Duration, report func(string, ...any),
) *cooler.Cooler {
	var announced bool
	for attempt := 0; ; attempt++ {
		found, err := open(ctx)
		if err == nil {
			return found
		}
		if ctx.Err() != nil {
			return nil
		}
		if !announced {
			// Once, not every tick: a machine with no cooler should not have
			// its journal filled with a fact that is not changing.
			report("no cooler telemetry yet: %v", err)
			announced = true
		}

		select {
		case <-ctx.Done():
			return nil
		case <-time.After(backoff[min(attempt, len(backoff)-1)]):
		}
	}
}

// connectBackoff is how long to wait between looks for a device, and how long
// to keep waiting. Short enough to catch one that appears moments later at
// boot; long enough not to be a busy loop for a machine that has none.
var connectBackoff = []time.Duration{
	time.Second, 2 * time.Second, 5 * time.Second, 15 * time.Second, 30 * time.Second,
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
	// The door and the script are KWin's. Anywhere else there is no session
	// bus to look for, and looking means the D-Bus library trying to start
	// one: a child process, which on Windows is a program that is not there.
	if runtime.GOOS != "linux" {
		svc.SetDesktop("hotkeys are not available on " + runtime.GOOS + " yet")
		cmd.Println("no hotkeys: not available on " + runtime.GOOS + " yet")
		return
	}
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

	runtimeDir, err := config.RuntimeDir()
	if err != nil {
		svc.SetDesktop(err.Error())
		return
	}
	install := &desktop.Installer{
		KWin:     desktop.NewKWin(conn),
		Dir:      filepath.Join(runtimeDir, "keys"),
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
