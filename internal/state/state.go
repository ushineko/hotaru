/*
Package state remembers what the lights should be showing.

Desired state, not observed: what the user last asked for, which is the thing
worth putting back after a reboot, after a device wakes up having forgotten, or
after an OpenRGB server restarts and hands back devices with no colour.

It lives in $XDG_STATE_HOME rather than among the configuration, because it is
not configuration. It changes every time a scene is applied, it means nothing on
another machine, and nobody wants it in a file they back up as settings.

**Empty is the normal starting condition.** A machine that has never been told
anything has nothing to restore, and hotaru touches no device until it is asked.
That is the whole of the inert-on-install rule: it is not a special case, it is
what an empty file does.
*/
package state

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/ushineko/fynedesygn/settings"
	_ "github.com/ushineko/fynedesygn/settings/yamlcodec" // .yml is YAML
	"github.com/ushineko/hotaru/internal/colour"
	"github.com/ushineko/hotaru/internal/devices"
)

// sectionKey is the one top-level key this file uses.
const sectionKey = "lighting"

// Device is what one device should be showing.
type Device struct {
	// Mode is the mode hotaru settled on, in the device's own spelling. Kept
	// so a restore does not have to re-derive it, and so a reconcile can tell
	// "the device is in the wrong mode" from "the colours drifted".
	Mode string `json:"mode"`

	// Colours is the frame: one per LED. The whole device, because that is the
	// unit everywhere else and because half a remembered frame is worse than
	// none.
	Colours []colour.Colour `json:"colours"`

	// Applied is when the user last asked for this.
	Applied time.Time `json:"applied"`
}

// Frame is the remembered state as a frame.
func (d Device) Frame(name string) devices.Frame {
	return devices.Frame{Device: name, Colours: d.Colours}
}

// Snapshot is every device hotaru has been told about, by name.
type Snapshot struct {
	Devices map[string]Device `json:"devices"`
}

// Empty reports whether there is nothing to restore — the condition of a fresh
// install, and the reason it touches nothing.
func (s Snapshot) Empty() bool { return len(s.Devices) == 0 }

// Names is every device the snapshot remembers, for reporting how much of a
// restore is still outstanding.
func (s Snapshot) Names() []string {
	out := make([]string, 0, len(s.Devices))
	for name := range s.Devices {
		out = append(out, name)
	}
	return out
}

/*
Store is the state file.

Machine-owned, unlike the rules file: hotaru writes this one, and a user who
edits it is editing a cache. That is why an unreadable state file is discarded
rather than refused — losing what the lights were showing costs one scene, and
refusing to save ever again would cost every scene after it.
*/
type Store struct {
	mu    sync.Mutex
	file  *settings.Store
	path  string
	cache Snapshot
}

// Open reads the state file, creating nothing until something is saved.
func Open(path string) (*Store, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, fmt.Errorf("make the state directory: %w", err)
	}

	file, err := settings.Open(path)
	if err != nil {
		var parse *settings.ParseError
		if !errors.As(err, &parse) || file == nil {
			return nil, fmt.Errorf("read the state file: %w", err)
		}
		// Unreadable machine state is a cache miss, not a crisis. Start again
		// rather than refuse to ever save: the cost is one scene's memory.
		file.Replace()
	}

	store := &Store{file: file, path: path, cache: Snapshot{Devices: map[string]Device{}}}
	var loaded Snapshot
	if file.Get(sectionKey, &loaded) && loaded.Devices != nil {
		store.cache = loaded
	}
	return store, nil
}

// Path is the file this store reads and writes.
func (s *Store) Path() string { return s.path }

// Snapshot is what is remembered.
func (s *Store) Snapshot() Snapshot {
	s.mu.Lock()
	defer s.mu.Unlock()
	return copyOf(s.cache)
}

// Record remembers what a device should be showing, and schedules a write.
func (s *Store) Record(name string, device Device) error {
	s.mu.Lock()
	if s.cache.Devices == nil {
		s.cache.Devices = map[string]Device{}
	}
	s.cache.Devices[name] = device
	snapshot := copyOf(s.cache)
	s.mu.Unlock()
	if err := s.file.Set(sectionKey, snapshot); err != nil {
		return fmt.Errorf("remember %s: %w", name, err)
	}
	return nil
}

// Forget drops a device, for one that has been turned off deliberately rather
// than merely darkened.
func (s *Store) Forget(name string) error {
	s.mu.Lock()
	delete(s.cache.Devices, name)
	snapshot := copyOf(s.cache)
	s.mu.Unlock()
	if err := s.file.Set(sectionKey, snapshot); err != nil {
		return fmt.Errorf("forget %s: %w", name, err)
	}
	return nil
}

// Flush writes any pending change immediately.
func (s *Store) Flush() error {
	if err := s.file.Flush(); err != nil {
		return fmt.Errorf("write %s: %w", s.path, err)
	}
	return nil
}

func copyOf(in Snapshot) Snapshot {
	out := Snapshot{Devices: make(map[string]Device, len(in.Devices))}
	for name, device := range in.Devices {
		colours := make([]colour.Colour, len(device.Colours))
		copy(colours, device.Colours)
		out.Devices[name] = Device{Mode: device.Mode, Colours: colours, Applied: device.Applied}
	}
	return out
}
