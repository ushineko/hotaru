package service

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
	"time"
)

/*
Lease is a preview somebody is holding.

A preview is not what anybody wants their machine to look like -- it is what
they are looking at while they decide. Two things follow, and both are the
reason this type exists rather than a boolean.

It suspends re-assertion for the devices it covers, because otherwise the timer
that exists to correct hardware that forgets restores desired state underneath
the person studying a draft -- on the G502, within sixty seconds.

And it has to end when its holder does. The draft is on somebody's hardware and
the correction has been switched off on that program's behalf, so a client that
crashes leaves a machine showing a state nobody chose and nothing to put it
back.
*/
type Lease struct {
	// Token identifies the lease to whoever took it.
	Token string
	// Scene is the name previewed, for reporting. Empty for an ad-hoc preview.
	Scene string
	// Devices are the device names the preview covers, as the server knows
	// them, which is what re-assertion checks against.
	Devices []string
	// Holder is who is looking at it, in a form a person can read: what
	// `hotaru light list` prints next to a device showing a draft.
	Holder string
	// Taken is when it started.
	Taken time.Time
	/*
		Expires is when the lease lapses unless it is renewed. Zero means it
		is bound to a connection instead: the holder is sitting on an open
		request, and the socket closing is a better signal than any clock.
	*/
	Expires time.Time
}

// Held reports whether the lease is still current at a given moment.
func (l *Lease) Held(now time.Time) bool {
	return l.Expires.IsZero() || now.Before(l.Expires)
}

/*
PreviewFor is how long an unrenewed lease lasts.

Short enough that a killed client does not hold somebody's hardware for long,
long enough that a renewal every few seconds is not a busy loop. A client that
can hold a connection open does not use this at all.
*/
const PreviewFor = 10 * time.Second

// Take records a preview lease over some devices.
//
// One device, one preview: a second caller is told who has it rather than
// quietly writing over a draft somebody else is looking at.
func (s *Service) Take(scene, holder string, devices []string, connection bool) (*Lease, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()
	s.sweep(now)
	for _, device := range devices {
		if held, ok := s.leases[device]; ok {
			return nil, fmt.Errorf("%s is showing a preview held by %s", device, held.Holder)
		}
	}

	lease := &Lease{
		Token: token(), Scene: scene, Holder: holder,
		Devices: append([]string(nil), devices...), Taken: now,
	}
	if !connection {
		lease.Expires = now.Add(PreviewFor)
	}
	if s.leases == nil {
		s.leases = map[string]*Lease{}
	}
	for _, device := range devices {
		s.leases[device] = lease
	}
	return lease, nil
}

// Renew pushes a lease's expiry out, for a holder that cannot keep a
// connection open. A lease that has already lapsed is not revived: the devices
// may belong to somebody else by now.
func (s *Service) Renew(token string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()
	s.sweep(now)
	for _, lease := range s.leases {
		if lease.Token == token {
			if !lease.Expires.IsZero() {
				lease.Expires = now.Add(PreviewFor)
			}
			return nil
		}
	}
	return fmt.Errorf("no preview with that token; it may have expired")
}

/*
Release ends a preview and puts the hardware back to what was asked for.

The reverting is the point. Dropping the lease alone would leave the draft on
the devices until something else happened to write to them, which on hardware
that holds its colour is forever.
*/
func (s *Service) Release(ctx context.Context, token string) (Restore, error) {
	s.mu.Lock()
	var covered []string
	for device, lease := range s.leases {
		if lease.Token == token {
			covered = append(covered, device)
			delete(s.leases, device)
		}
	}
	s.mu.Unlock()

	if len(covered) == 0 {
		return Restore{}, nil
	}
	sort.Strings(covered)
	return s.Reconcile(ctx, covered)
}

/*
Expired ends every lapsed lease and reverts what they covered.

Called on the reconciler's tick rather than by a timer per lease: the loop that
would notice a device needing re-assertion is exactly the loop that should
notice one whose holder stopped talking.
*/
func (s *Service) Expired(ctx context.Context) (Restore, error) {
	s.mu.Lock()
	var covered []string
	now := time.Now()
	for device, lease := range s.leases {
		if !lease.Held(now) {
			covered = append(covered, device)
			delete(s.leases, device)
		}
	}
	s.mu.Unlock()

	if len(covered) == 0 {
		return Restore{}, nil
	}
	sort.Strings(covered)
	return s.Reconcile(ctx, covered)
}

/*
Previewing reports the lease covering a device, if one does.

Two callers: re-assertion, which must not write to a device somebody is looking
at, and the listing, which says so out loud. A machine showing something nobody
chose is the confusion this project keeps running into, so it is never left
implicit.
*/
func (s *Service) Previewing(device string) (*Lease, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	lease, ok := s.leases[device]
	if !ok || !lease.Held(time.Now()) {
		return nil, false
	}
	return lease, true
}

// Leases is every current preview, newest last, for reporting.
func (s *Service) Leases() []*Lease {
	s.mu.RLock()
	defer s.mu.RUnlock()

	now := time.Now()
	seen := map[string]bool{}
	var out []*Lease
	for _, lease := range s.leases {
		if seen[lease.Token] || !lease.Held(now) {
			continue
		}
		seen[lease.Token] = true
		out = append(out, lease)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Taken.Before(out[j].Taken) })
	return out
}

// sweep drops lapsed leases. The caller holds the write lock. It does not
// revert anything: reverting writes to hardware, and that cannot happen under
// a lock the write path also wants.
func (s *Service) sweep(now time.Time) {
	for device, lease := range s.leases {
		if !lease.Held(now) {
			delete(s.leases, device)
		}
	}
}

// token is an identifier a client keeps and hands back. Not a secret -- the
// socket's permissions are the authentication -- just unguessable enough that
// two clients cannot collide.
func token() string {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		// crypto/rand does not fail on Linux; a timestamp is still unique
		// enough to tell two leases apart if it ever does.
		return strings.ReplaceAll(time.Now().Format("150405.000000000"), ".", "")
	}
	return hex.EncodeToString(b[:])
}
