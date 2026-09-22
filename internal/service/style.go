package service

import (
	"fmt"

	"github.com/ushineko/hotaru/internal/scenes"
)

/*
CopyEffects gives other scenes the style of this one.

**A style and a colour are different things**, and the editor has said so for
a while: a scene names a colour per light and a *mode* per device -- what that
device does with the colours once it has them. Somebody who decides their
keyboard should be reactive has decided that about their keyboard, not about
one scene, and until this the only way to say so across a bank of nine was to
open nine scenes.

So: copy one scene's effects onto others. The colours are untouched, which is
the point -- the two were separable in the model and were not separable in
practice.

**It replaces rather than merges.** A target that had an effect for a device
the source says nothing about would otherwise keep it, and "these scenes now
look like that one" would be true of some devices and not others. Replacing is
the answer somebody can predict from the sentence they typed.
*/
func (s *Service) CopyEffects(from string, to []string) ([]string, error) {
	store, err := s.sceneStore()
	if err != nil {
		return nil, err
	}

	source, err := store.Get(from)
	if err != nil {
		return nil, err
	}

	// Every target is read before any is written: a name that is not there
	// should cost nothing rather than leave half a bank restyled.
	found := make([]scenes.Scene, 0, len(to))
	for _, name := range to {
		scene, err := store.Get(name)
		if err != nil {
			return nil, err
		}
		if scene.Name == source.Name {
			continue
		}
		found = append(found, scene)
	}

	var changed []string
	for _, scene := range found {
		scene.Effects = cloneEffects(source.Effects)
		if err := store.Save(scene); err != nil {
			return changed, fmt.Errorf("save %s: %w", scene.Name, err)
		}
		changed = append(changed, scene.Name)
	}
	return changed, nil
}

// cloneEffects copies the map, because two scenes sharing one would make an
// edit to either an edit to both.
func cloneEffects(in map[string]string) map[string]string {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]string, len(in))
	for device, mode := range in {
		out[device] = mode
	}
	return out
}
