/*
Package queue serialises work per device.

One goroutine per device, each with a single pending slot. That shape is the
whole design, and it replaces the bounded job queue this project's predecessor
used -- which dropped the oldest job on overflow, and so dropped work at random
when three scenes were pressed in a second.

**Latest wins, per device.** A background job that is superseded before it runs
is not queued behind the newer one; it is replaced by it. Lighting is a state,
not a sequence: nobody wants the second-to-last scene applied after the last
one, and there is no value in a backlog of instructions that have all been
countermanded.

**Nothing is dropped silently.** A caller waiting on a superseded job is told,
rather than being left to infer it from a device that did not change.
*/
package queue

import (
	"context"
	"sync"
)

// Outcome is what became of a submitted job.
type Outcome struct {
	// Superseded is set when a newer job for the same device replaced this
	// one before it ran.
	Superseded bool
}

// Job is work against one device. The context is the queue's, cancelled when
// the queue shuts down.
type Job func(ctx context.Context)

/*
Set is the per-device queues, created as devices are written to.

A device nobody has written to has no goroutine: a machine with sixty devices
and one scene runs one.
*/
type Set struct {
	mu     sync.Mutex
	boxes  map[string]*box
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// New is an empty set of queues, running until Close.
func New(ctx context.Context) *Set {
	inner, cancel := context.WithCancel(ctx)
	return &Set{boxes: map[string]*box{}, ctx: inner, cancel: cancel}
}

/*
Do runs a job for a device and waits for it.

Used for what a person asked for. Waiting is what lets the caller report what
happened to each device, which is the difference between "the scene was sent"
and "the scene is showing".
*/
func (s *Set) Do(device string, job Job) Outcome {
	done := make(chan Outcome, 1)
	s.submit(device, job, done)
	select {
	case outcome := <-done:
		return outcome
	case <-s.ctx.Done():
		return Outcome{Superseded: true}
	}
}

/*
Post runs a job for a device without waiting.

Used for reconciliation, where nobody is listening and being superseded by a
newer request is exactly the right outcome.
*/
func (s *Set) Post(device string, job Job) {
	s.submit(device, job, nil)
}

func (s *Set) submit(device string, job Job, done chan<- Outcome) {
	s.mu.Lock()
	b, running := s.boxes[device]
	if !running {
		b = &box{wake: make(chan struct{}, 1)}
		s.boxes[device] = b
		s.wg.Add(1)
		go func() {
			defer s.wg.Done()
			b.run(s.ctx)
		}()
	}
	s.mu.Unlock()

	b.put(pending{job: job, done: done})
}

// Close stops every queue and waits for the job in flight.
func (s *Set) Close() {
	s.cancel()
	s.wg.Wait()
}

type pending struct {
	job  Job
	done chan<- Outcome
}

// box is one device's queue: a slot, not a list.
type box struct {
	mu   sync.Mutex
	next *pending
	wake chan struct{}
}

func (b *box) put(p pending) {
	b.mu.Lock()
	if b.next != nil && b.next.done != nil {
		// The caller waiting on the job being replaced is told, rather than
		// waiting for a write that will now never happen.
		b.next.done <- Outcome{Superseded: true}
	}
	b.next = &p
	b.mu.Unlock()

	select {
	case b.wake <- struct{}{}:
	default: // already awake
	}
}

func (b *box) take() *pending {
	b.mu.Lock()
	defer b.mu.Unlock()
	p := b.next
	b.next = nil
	return p
}

func (b *box) run(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			// Anyone still waiting learns the queue stopped rather than
			// hanging on a write that will not happen.
			if p := b.take(); p != nil && p.done != nil {
				p.done <- Outcome{Superseded: true}
			}
			return
		case <-b.wake:
			for {
				p := b.take()
				if p == nil {
					break
				}
				p.job(ctx)
				if p.done != nil {
					p.done <- Outcome{}
				}
			}
		}
	}
}
