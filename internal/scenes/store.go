package scenes

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

// The top-level keys this file uses.
const (
	sectionKey  = "scenes"
	bindingsKey = "bindings"
)

/*
Store is scenes.yml.

The one file hotaru serialises. Rules are the user's and are never rewritten;
desired state is machine state nobody reads; scenes are in between -- written
by the program because the GUI is their editor, and read by people because they
are in the configuration directory and somebody will open them.

**A file that will not parse is reported, never replaced.** Machine state can
be discarded on a bad read: the cost is one scene's memory. Somebody's named
scenes cannot, because a hand edit with a typo in it would otherwise delete
work, silently, at the moment they most want it back.
*/
type Store struct {
	mu    sync.Mutex
	file  *settings.Store
	path  string
	cache map[string]Scene
	/*
		bound is what somebody has changed about the keys, not the keys.

		Shipped bindings are code and are merged in on read, so a file that
		says nothing about the keys gets the bank this desk has always had,
		and one that rebinds Num4 says only that. An empty scene name unbinds
		a shipped key, which is a thing somebody has to be able to express.
	*/
	bound map[string]string
}

// Open reads the scenes file, creating nothing until something is saved.
func Open(path string) (*Store, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, fmt.Errorf("make the configuration directory: %w", err)
	}

	file, err := settings.Open(path)
	if err != nil {
		var parse *settings.ParseError
		if errors.As(err, &parse) {
			// Deliberately not recovered. See the type's comment.
			return nil, fmt.Errorf("read %s: %w", path, err)
		}
		return nil, fmt.Errorf("read %s: %w", path, err)
	}

	store := &Store{
		file: file, path: path,
		cache: map[string]Scene{}, bound: map[string]string{},
	}
	var loaded map[string]Scene
	if file.Get(sectionKey, &loaded) {
		for name, scene := range loaded {
			scene.Name = name
			store.cache[name] = scene
		}
	}
	var bound map[string]string
	if file.Get(bindingsKey, &bound) {
		store.bound = bound
	}
	return store, nil
}

/*
Bindings are the keys, shipped ones included.

Merged rather than replaced: a file that mentions one key changes that key and
leaves the other eight as they were. A binding to an empty name is a shipped
key somebody has deliberately unbound, and is dropped here rather than being
reported as a binding to nothing.
*/
func (s *Store) Bindings() map[string]string {
	s.mu.Lock()
	defer s.mu.Unlock()

	out := DefaultBindings()
	for key, scene := range s.bound {
		if scene == "" {
			delete(out, key)
			continue
		}
		out[key] = scene
	}
	return out
}

// Bind points a key at a scene. An empty name unbinds it, including a shipped
// one -- which is why this records the unbinding rather than forgetting it.
func (s *Store) Bind(key, scene string) error {
	if strings.TrimSpace(key) == "" {
		return errors.New("a binding needs a key")
	}
	s.mu.Lock()
	s.bound[key] = scene
	snapshot := make(map[string]string, len(s.bound))
	for k, v := range s.bound {
		snapshot[k] = v
	}
	s.mu.Unlock()

	if err := s.file.Set(bindingsKey, snapshot); err != nil {
		return fmt.Errorf("bind %s: %w", key, err)
	}
	return s.Flush()
}

// Path is the file this store reads and writes.
func (s *Store) Path() string { return s.path }

/*
All is every scene, shipped ones included, in the order a listing shows them.

Shipped scenes are code and are never written to the file: a fresh install has
the nine and has written nothing, which is the inert rule holding for scenes as
well as for hardware. Saving one under a shipped name replaces it for as long
as the saved one exists, and deleting that override brings the shipped one
back.
*/
func (s *Store) All() []Scene {
	s.mu.Lock()
	defer s.mu.Unlock()

	merged := map[string]Scene{}
	for _, scene := range Shipped() {
		scene.Shipped = true
		merged[scene.Name] = scene
	}
	for name, scene := range s.cache {
		merged[name] = scene
	}
	out := make([]Scene, 0, len(merged))
	for _, scene := range merged {
		out = append(out, scene)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

/*
Get finds a scene by name, accepting any unambiguous prefix.

Somebody typing `hotaru scene apply even` means the one scene beginning with
those letters, and being told it does not exist when it plainly does is the
kind of pedantry that makes a program unpleasant. Two matches is a real
ambiguity and says so.
*/
func (s *Store) Get(name string) (Scene, error) {
	all := s.All()
	for _, scene := range all {
		if scene.Name == name {
			return scene, nil
		}
	}
	var matches []Scene
	for _, scene := range all {
		if strings.HasPrefix(strings.ToLower(scene.Name), strings.ToLower(name)) {
			matches = append(matches, scene)
		}
	}
	switch len(matches) {
	case 1:
		return matches[0], nil
	case 0:
		return Scene{}, fmt.Errorf("no scene called %q", name)
	}
	sort.Slice(matches, func(i, j int) bool { return matches[i].Name < matches[j].Name })
	names := make([]string, len(matches))
	for i, scene := range matches {
		names[i] = scene.Name
	}
	return Scene{}, fmt.Errorf("%q matches %s", name, strings.Join(names, ", "))
}

// Save writes a scene, replacing one of the same name.
func (s *Store) Save(scene Scene) error {
	if strings.TrimSpace(scene.Name) == "" {
		return errors.New("a scene needs a name")
	}
	s.mu.Lock()
	s.cache[scene.Name] = scene
	snapshot := s.copy()
	s.mu.Unlock()

	if err := s.file.Set(sectionKey, snapshot); err != nil {
		return fmt.Errorf("save %s: %w", scene.Name, err)
	}
	return s.Flush()
}

// Delete removes a scene. Removing one that is not there is not an error:
// the caller wanted it gone and it is gone.
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

// Flush writes any pending change immediately. A scene somebody saved and a
// machine that lost power a second later should not disagree.
func (s *Store) Flush() error {
	if err := s.file.Flush(); err != nil {
		return fmt.Errorf("write %s: %w", s.path, err)
	}
	return nil
}

// copy is the map as it goes to the file. The caller holds the lock.
func (s *Store) copy() map[string]Scene {
	out := make(map[string]Scene, len(s.cache))
	for name, scene := range s.cache {
		out[name] = scene
	}
	return out
}
