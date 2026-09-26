package gui

import (
	"context"
	"sync"

	"github.com/ushineko/hotaru/internal/api"
)

/*
The window's hold on a preview.

One at a time, and it is the connection: a goroutine sits on the request, and
the window closing, crashing or being killed drops the socket. The service puts
the lights back without having been told, which is the half of this that cannot
be forgotten about by an editor that ended badly.
*/
type preview struct {
	mu      sync.Mutex
	release func()
	// token is the lease, so a change to the draft is a change to this
	// preview rather than a second one.
	token string
}

/*
Preview shows a draft on the hardware.

**The lease is taken once and then kept.** Releasing and re-taking it for every
change was the obvious shape and the wrong one: releasing tells the service to
put the lights back, so every colour went to the hardware with a revert to the
previous one a moment ahead of it. What somebody saw was the colour flashing
back -- the picker looking broken while doing exactly what it was told.

So the first call takes the lease, and every call after it says what the draft
is now under that same lease: not recorded, and not reconciled over either,
because the lease this window is still holding is what keeps re-assertion off
those devices.
*/
func (a *App) Preview(ctx context.Context, scene api.Scene) error {
	if a.Previewing() {
		return a.update(ctx, scene)
	}
	return a.hold(ctx, scene)
}

// hold puts a draft up and keeps the lease, which is the first call and the
// one that has nothing to change yet.
func (a *App) hold(_ context.Context, scene api.Scene) error {
	/*
		Deliberately not the caller's context.

		The lease *is* this request: it lives as long as the connection. The
		shell cancels an operation's context the moment the operation returns,
		so holding on it put the draft up and took it down again within a
		millisecond -- the button worked perfectly and the lights never moved.
	*/
	held, release, err := a.client.HoldDraft(context.Background(), scene, "hotaru-gui")
	if err != nil {
		return err
	}

	a.preview.mu.Lock()
	a.preview.release = release
	if held.Preview != nil {
		a.preview.token = held.Preview.Token
	}
	a.preview.mu.Unlock()
	return nil
}

/*
update writes a draft to the devices this window already holds.

**Through the same door a named scene goes through.** It used to build a
colours-only request and send it to the plain lighting route, which has no
effects in it at all -- so every change to a draft put the keyboard into
Direct, and the effect only ever came back when something applied a saved
scene. One scene, two ways of becoming lights, and only one of them knew what
an effect was. See spec 054.

The lease is named rather than re-taken, so nothing is released and no
previous colours flash back on the way.
*/
func (a *App) update(ctx context.Context, scene api.Scene) error {
	a.preview.mu.Lock()
	token := a.preview.token
	a.preview.mu.Unlock()

	if token == "" {
		// A hold that came back without one. Nothing to change; the draft
		// goes up the way anything else does.
		return a.hold(ctx, scene)
	}
	_, err := a.client.Redraft(ctx, token, scene)
	return err
}

// Previewing reports whether the window is holding one, which the editor says
// out loud.
func (a *App) Previewing() bool {
	a.preview.mu.Lock()
	defer a.preview.mu.Unlock()
	return a.preview.release != nil
}

/*
EndPreview lets go, and the lights go back.

Safe to call when there is nothing to end, because everything that should end a
preview calls it: leaving the editor, saving, closing the window. A rule that
has to be remembered in four places is a rule that will be forgotten in one.
*/
func (a *App) EndPreview() {
	a.preview.mu.Lock()
	release := a.preview.release
	a.preview.release, a.preview.token = nil, ""
	a.preview.mu.Unlock()

	if release != nil {
		release()
	}
}
