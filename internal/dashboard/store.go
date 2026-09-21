package dashboard

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/ushineko/fynedesygn/settings"
	_ "github.com/ushineko/fynedesygn/settings/yamlcodec" // .yml is YAML
)

// The top-level keys the dashboards file uses.
const (
	sectionKey = "dashboards"
	activeKey  = "active"
)

/*
Store is dashboards.yml.

The same shape as the scene store and for the same reasons: written by the
program because the window is its editor, read by people because it is in the
configuration directory and somebody will open it, and never replaced when it
will not parse -- a hand edit with a typo in it would otherwise delete work at
the moment somebody most wants it back.
*/
type Store struct {
	mu     sync.Mutex
	file   *settings.Store
	path   string
	cache  map[string]Dashboard
	active string
}

// Open reads the dashboards file, creating nothing until something is saved.
func Open(path string) (*Store, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, fmt.Errorf("make the configuration directory: %w", err)
	}
	file, err := settings.Open(path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}

	store := &Store{file: file, path: path, cache: map[string]Dashboard{}}
	var loaded map[string]Dashboard
	if file.Get(sectionKey, &loaded) {
		for name, one := range loaded {
			one.Name = name
			store.cache[name] = one
		}
	}
	var active string
	if file.Get(activeKey, &active) {
		store.active = active
	}
	return store, nil
}

// Path is the file this store reads and writes.
func (s *Store) Path() string { return s.path }

/*
All is every dashboard, shipped ones included, in the order a listing shows
them.

Shipped ones are code and are never written to the file: a fresh install has
them and has written nothing. Saving one under a shipped name replaces it for
as long as the saved one exists, and deleting that override brings the shipped
one back.
*/
func (s *Store) All() []Dashboard {
	s.mu.Lock()
	defer s.mu.Unlock()

	merged := map[string]Dashboard{}
	for _, one := range Shipped() {
		one.Shipped = true
		merged[one.Name] = one
	}
	for name, one := range s.cache {
		merged[name] = one
	}
	out := make([]Dashboard, 0, len(merged))
	for _, one := range merged {
		out = append(out, one)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// Get finds a dashboard by name, accepting any unambiguous prefix -- the same
// courtesy the scene store extends, for the same reason.
func (s *Store) Get(name string) (Dashboard, error) {
	all := s.All()
	for _, one := range all {
		if one.Name == name {
			return one, nil
		}
	}
	var matches []Dashboard
	for _, one := range all {
		if strings.HasPrefix(strings.ToLower(one.Name), strings.ToLower(name)) {
			matches = append(matches, one)
		}
	}
	switch len(matches) {
	case 1:
		return matches[0], nil
	case 0:
		return Dashboard{}, fmt.Errorf("no dashboard called %q", name)
	}
	names := make([]string, len(matches))
	for i, one := range matches {
		names[i] = one.Name
	}
	sort.Strings(names)
	return Dashboard{}, fmt.Errorf("%q matches %s", name, strings.Join(names, ", "))
}

/*
Active is the dashboard the panel draws.

A machine that has never chosen one draws the shipped default, and a machine
whose chosen one has been deleted goes back to it rather than to a blank
screen: the panel is decorative and must keep working through somebody's
editing.
*/
func (s *Store) Active() Dashboard {
	s.mu.Lock()
	name := s.active
	s.mu.Unlock()

	if name != "" {
		if one, err := s.Get(name); err == nil {
			return one
		}
	}
	one, err := s.Get(Default)
	if err != nil {
		return Shipped()[0]
	}
	return one
}

// Use makes one the dashboard the panel draws.
func (s *Store) Use(name string) error {
	one, err := s.Get(name)
	if err != nil {
		return err
	}
	s.mu.Lock()
	s.active = one.Name
	s.mu.Unlock()

	if err := s.file.Set(activeKey, one.Name); err != nil {
		return fmt.Errorf("use %s: %w", name, err)
	}
	return s.Flush()
}

// Save writes a dashboard, replacing one of the same name.
func (s *Store) Save(one Dashboard) error {
	if strings.TrimSpace(one.Name) == "" {
		return errors.New("a dashboard needs a name")
	}
	s.mu.Lock()
	s.cache[one.Name] = one
	snapshot := s.copy()
	s.mu.Unlock()

	if err := s.file.Set(sectionKey, snapshot); err != nil {
		return fmt.Errorf("save %s: %w", one.Name, err)
	}
	return s.Flush()
}

// Delete removes a dashboard. Removing one that is not there is not an error:
// the caller wanted it gone and it is gone. Deleting an override of a shipped
// name brings the shipped one back.
func (s *Store) Delete(name string) error {
	s.mu.Lock()
	delete(s.cache, name)
	snapshot := s.copy()
	s.mu.Unlock()

	if err := s.file.Set(sectionKey, snapshot); err != nil {
		return fmt.Errorf("delete %s: %w", name, err)
	}
	return s.Flush()
}

// Flush writes any pending change immediately.
func (s *Store) Flush() error {
	if err := s.file.Flush(); err != nil {
		return fmt.Errorf("write %s: %w", s.path, err)
	}
	return nil
}

// copy is the map as it goes to the file. The caller holds the lock.
func (s *Store) copy() map[string]Dashboard {
	out := make(map[string]Dashboard, len(s.cache))
	for name, one := range s.cache {
		out[name] = one
	}
	return out
}
