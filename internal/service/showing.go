package service

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

// Applied is the scene last applied, or empty if none has been since the
// service started.
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
	s.mu.Unlock()
}

// drawing records what the panel was asked for.
func (s *Service) drawing(what string) {
	s.mu.Lock()
	s.showing = what
	s.mu.Unlock()
}
