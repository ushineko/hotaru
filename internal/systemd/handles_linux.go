//go:build linux

package systemd

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// hidrawNodes is where the kernel lists the character devices OpenRGB drives
// its USB hardware through, and devNode is where it puts them.
const (
	hidrawNodes = "/sys/class/hidraw"
	devNode     = "/dev/"
)

// deletedMark is what the kernel appends to a /proc descriptor whose target has
// been removed. It is the whole of the evidence: a node is deleted or it is not.
const deletedMark = " (deleted)"

/*
held looks at the running server's descriptors.

Every step can fail into "not known", and each failure is ordinary rather than
exceptional: no unit on this machine, a unit that is not running, a PID that
belongs to root, a /proc entry that vanished while being read. None of them is
evidence of a stale handle, and none is worth a remedy.
*/
func serverPID(ctx context.Context) (int, bool) {
	facts := look(ctx)
	if !facts.Installed || !facts.Active {
		return 0, false
	}
	return mainPID(ctx, facts.Unit, facts.User)
}

func heldBy(pid int) Handles {
	if !isServer(pid) {
		return Handles{}
	}
	open, deleted, ok := descriptors(pid)
	if !ok {
		return Handles{}
	}
	return Handles{Deleted: deleted, Orphaned: orphaned(open), Known: true}
}

/*
isServer is whether this PID is still the OpenRGB server.

A PID looked up a minute ago may have exited, and the number may since belong
to something else this user is running. Reading what it calls itself is cheaper
than asking systemd again and rules out reporting one program's descriptors as
another's.
*/
func isServer(pid int) bool {
	if pid <= 0 {
		return false
	}
	name, err := os.ReadFile(filepath.Join("/proc", strconv.Itoa(pid), "comm")) //nolint:gosec // built here
	if err != nil {
		return false
	}
	return strings.Contains(strings.ToLower(strings.TrimSpace(string(name))), "openrgb")
}

// mainPID is the process the unit is running as, through the same systemctl
// question the rest of this package asks. Zero means the unit is not running,
// which is a state with its own remedy rather than a stale handle.
func mainPID(ctx context.Context, unit string, user bool) (int, bool) {
	args := []string{"show", unit, "--property=MainPID"}
	if user {
		args = append([]string{"--user"}, args...)
	}
	out, err := systemctl(ctx, args...)
	if err != nil {
		return 0, false
	}

	_, value, found := strings.Cut(strings.TrimSpace(out), "=")
	if !found {
		return 0, false
	}
	pid, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil || pid <= 0 {
		return 0, false
	}
	return pid, true
}

/*
descriptors is which device nodes the process holds, and whether any of them
has been removed underneath it.

Two ways to see nothing, and they must not be confused. A process this user
does not own cannot be listed at all. A process that can be listed but whose
links cannot be resolved is the more dangerous one: the directory answers, every
entry is there, and reading any of them is refused -- which, taken at face
value, looks exactly like a server holding no devices and nothing deleted.

That is what a mount namespace does. A caller under systemd's ProtectSystem,
ProtectHome or PrivateTmp gets a full listing and permission denied on every
link, measured on the machine this was written for. Reporting "looked, found
nothing wrong" from there would be the worst answer available, so a refusal to
resolve is not known.
*/
func descriptors(pid int) (open map[string]bool, deleted, known bool) {
	dir := filepath.Join("/proc", strconv.Itoa(pid), "fd")
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, false, false
	}

	paths := make([]string, 0, len(entries))
	for _, entry := range entries {
		paths = append(paths, filepath.Join(dir, entry.Name()))
	}
	return resolve(paths, os.Readlink)
}

/*
resolve turns descriptors into the device nodes behind them.

Separated from finding them so that being refused -- the case that matters, and
the one a machine only produces from inside a mount namespace -- can be put in
a test rather than reasoned about.
*/
func resolve(paths []string, readlink func(string) (string, error)) (open map[string]bool, deleted, known bool) {
	open = make(map[string]bool, len(paths))
	for _, path := range paths {
		target, err := readlink(path)
		if err != nil {
			// Refused is this process being unable to see, which is not a
			// fact about the server. Gone is a descriptor closed between the
			// listing and the read, which is ordinary and skipped.
			if errors.Is(err, os.ErrPermission) {
				return nil, false, false
			}
			continue
		}
		node, gone := strings.CutSuffix(target, deletedMark)
		if !strings.HasPrefix(node, devNode+"hidraw") {
			continue
		}
		if gone {
			deleted = true
			continue
		}
		open[node] = true
	}
	return open, deleted, true
}

/*
orphaned is every device present that the server holds no descriptor for.

Grouped by the hardware rather than by the node, because one device can offer
several. A Keychron K4 HE presents two hidraw interfaces and OpenRGB drives
one of them; counting nodes would call the other an orphan and report a
working keyboard as broken, which is the one thing a health check must not do.
*/
func orphaned(open map[string]bool) []string { return ungrouped(scan(), open) }

// hidNode is one character device, what it calls itself, and which piece of
// hardware it belongs to.
type hidNode struct {
	node string // the /dev name, e.g. "hidraw15"
	name string // what the device calls itself
	key  string // the hardware several nodes can share
}

// scan is every hidraw node this machine has. Separated from the grouping so
// that the grouping -- which is where being wrong would matter -- can be tested
// without a machine that has the hardware plugged into it.
func scan() []hidNode {
	entries, err := os.ReadDir(hidrawNodes)
	if err != nil {
		return nil
	}
	out := make([]hidNode, 0, len(entries))
	for _, entry := range entries {
		key, name, ok := hardware(entry.Name())
		if !ok {
			continue
		}
		out = append(out, hidNode{node: entry.Name(), name: name, key: key})
	}
	return out
}

/*
ungrouped is the hardware with no descriptor held against any of its nodes.

Any is the important word. A device offering several interfaces, with the
server driving one of them, is a working device; counting nodes would report
the ones it does not drive and call a working keyboard broken.
*/
func ungrouped(nodes []hidNode, open map[string]bool) []string {
	type group struct {
		name string
		held bool
	}
	groups := make(map[string]*group)
	order := make([]string, 0, len(nodes))

	for _, n := range nodes {
		if _, seen := groups[n.key]; !seen {
			groups[n.key] = &group{name: n.name}
			order = append(order, n.key)
		}
		if open[devNode+n.node] {
			groups[n.key].held = true
		}
	}

	var out []string
	for _, key := range order {
		if g := groups[key]; !g.held {
			out = append(out, g.name)
		}
	}
	return out
}

/*
hardware identifies the physical device a hidraw node belongs to, and what it
calls itself.

The node's `device` link points at the HID device, whose grandparent is the USB
device that the interfaces hang off; two interfaces of one keyboard resolve to
the same one. A device that is not shaped like that groups alone, which is the
conservative way to be wrong: it can only ever under-report.
*/
func hardware(node string) (key, name string, ok bool) {
	dir := filepath.Join(hidrawNodes, node, "device")
	resolved, err := filepath.EvalSymlinks(dir)
	if err != nil {
		return "", "", false
	}

	name = hidName(filepath.Join(dir, "uevent"))
	if name == "" {
		return "", "", false
	}
	return filepath.Dir(filepath.Dir(resolved)), name, true
}

// hidName is what the device calls itself, from the uevent the kernel writes
// beside it. Empty means a node that cannot be identified, which is a node
// this does not reason about.
func hidName(path string) string {
	body, err := os.ReadFile(path) //nolint:gosec // a path built from this file's own constants
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(body), "\n") {
		if value, found := strings.CutPrefix(strings.TrimSpace(line), "HID_NAME="); found {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
