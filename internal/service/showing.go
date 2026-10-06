package service

import (
	"context"
	"fmt"
	"os"

	"github.com/ushineko/hotaru/internal/state"
)

/*
What is on the machine right now: the scene somebody applied, and what the
cooler's panel is showing.

Neither was recorded anywhere. The service knew the desired *state* -- a frame
per device -- which is what reconciliation needs and is not what somebody
looking at the window is asking. "Which of my scenes is this?" is a question
about the last thing they pressed, and the colours alone cannot answer it: two
scenes can light the machine identically and mean different things.

Remembered rather than derived, and deliberately shallow. It is a label for
the last thing that happened, not a claim about the present: something that
changed the lights by another route -- a `hotaru light set`, another program,
a device waking up wrong -- leaves the label saying what it said, which is why
the window shows it as "last applied" rather than as "this is the scene".
*/

// Applied is the scene last applied, or empty if none ever has been. It
// survives a restart through the recorder (#183): forgetting it turned saving
// the scene on the machine into writing a file and nothing else.
func (s *Service) Applied() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.applied
}

// Showing is what the panel was last asked to draw, in the words the window
// says it in. Empty means nothing has asked.
func (s *Service) Showing() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.showing
}

// applying records the scene somebody applied. A preview is not recorded: it
// is not what anybody asked the machine to be.
func (s *Service) applying(scene string) {
	s.mu.Lock()
	s.applied = scene
	recorder := s.recorder
	s.mu.Unlock()
	if recorder != nil {
		_ = recorder.RecordScene(scene)
	}
}

/*
shown records what the panel is showing: the label, and what it takes to put
it back (#184). The image is the picture's bytes, which the recorder keeps
only when there is no file to name.
*/
func (s *Service) shown(screen state.Screen, image []byte) {
	s.mu.Lock()
	s.showing = screen.Showing
	recorder := s.recorder
	s.mu.Unlock()
	if recorder == nil {
		return
	}
	if image != nil && screen.Picture != "" {
		image = nil
	}
	_ = recorder.RecordScreen(screen, image)
}

// dashboardShown is the record for the dashboard drawing, by the name of the
// one in use where there is a store to ask.
func (s *Service) dashboardShown() state.Screen {
	if store, err := s.boards(); err == nil {
		return state.Screen{Showing: "dashboard: " + store.Active().Name}
	}
	return state.Screen{Showing: "the dashboard"}
}

/*
RestoreScreen puts back what the panel was last asked to show, for a cooler
that has just attached.

The screen's half of what reconciling does for the lights. Nothing recorded
is nothing to put back, which is a fresh install and the dashboard. The
dashboard recorded is the same: it is what an attached cooler draws already.
A picture whose file has gone is reported and left; the dashboard stays.
*/
func (s *Service) RestoreScreen(ctx context.Context) error {
	s.mu.RLock()
	recorder := s.recorder
	s.mu.RUnlock()
	if recorder == nil {
		return nil
	}
	screen := recorder.Snapshot().Screen
	switch {
	case screen == nil:
		return nil
	case screen.Readout:
		return s.Draw(ctx, Screen{Readout: true})
	case screen.Picture != "":
		gif, err := os.ReadFile(screen.Picture) //nolint:gosec // a path hotaru recorded
		if err != nil {
			return fmt.Errorf("put back %s: %w", screen.Showing, err)
		}
		return s.Draw(ctx, Screen{Image: gif, Source: screen.Picture})
	}
	return nil
}
