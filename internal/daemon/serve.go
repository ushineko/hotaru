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
	"time"

	"github.com/spf13/cobra"
	"github.com/ushineko/hotaru/internal/api"
	"github.com/ushineko/hotaru/internal/config"
	"github.com/ushineko/hotaru/internal/openrgb"
	"github.com/ushineko/hotaru/internal/queue"
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

	// One goroutine per device, created on first write. A reconcile and a
	// user's scene cannot interleave on the same device, and a write that is
	// superseded before it runs is replaced rather than queued behind the
	// thing that countermanded it.
	writes := queue.New(ctx)
	defer writes.Close()
	svc.SetQueue(writes)

	listener, err := api.Listen(ctx, socket)
	if err != nil {
		return err
	}
	cmd.Printf("hotaru %s listening on %s\n", version.Version, socket)

	// The OpenRGB server is a resource that appears, not a dependency that is
	// satisfied: it may start after this does, or never. Connecting happens in
	// the background and keeps trying, so the API is up either way.
	report := func(format string, args ...any) { cmd.PrintErrf(format+"\n", args...) }
	go func() {
		connect(ctx, svc, address, report)
		// Restoring waits for a server rather than being ordered after one:
		// there is no unit to order against, and "started" is not "ready".
		(&Reconciler{Service: svc, Report: report}).Run(ctx)
	}()

	return api.Serve(ctx, listener, svc)
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
