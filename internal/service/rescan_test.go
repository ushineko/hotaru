package service_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/ushineko/hotaru/internal/colour"
	"github.com/ushineko/hotaru/internal/devices"
	"github.com/ushineko/hotaru/internal/openrgb"
	"github.com/ushineko/hotaru/internal/service"
)

/*
restarts is a machine whose server can be bounced, and which swaps in the
server that comes back.

A restart produces a different process with its own enumeration, so a test of
one has to be able to hand back a server that has never been written to --
otherwise it is a test of calling a function.
*/
type restarts struct {
	machine
	svc   *service.Service
	after openrgb.Client
	done  bool
}

func (r *restarts) Bounce(context.Context) error {
	if r.bounce != nil {
		return r.bounce
	}
	r.done = true
	r.svc.SetClient(r.after)
	return nil
}

/*
TestRescanBouncesTheServerAndRestores is spec 058 AC6.

The point is the second server. A rescan that reconnected to the same process
would have changed nothing: its picture of the hardware is fixed at the moment
it started, which is the whole reason a replugged device cannot be recovered
any other way.
*/
func TestRescanBouncesTheServerAndRestores(t *testing.T) {
	before, after := openrgb.NewFake(board(), keyboard()), openrgb.NewFake(board(), keyboard())
	svc := service.New(nil, before, "")
	svc.SetRecorder(recorder(t))

	target, err := devices.ParseTarget("Keychron")
	require.NoError(t, err)
	_, err = svc.Apply(t.Context(), service.Request{
		Assignments: []devices.Assignment{{Target: target, Colour: colour.MustParse("red")}},
	})
	require.NoError(t, err)

	env := &restarts{svc: svc, after: after}
	svc.SetEnvironment(env)

	got, err := svc.Rescan(t.Context())
	require.NoError(t, err)
	require.True(t, env.done, "the server was never restarted")
	require.Equal(t, 2, got.Devices, "the count the server found the second time")
	require.Positive(t, got.Applied, "the lights were not put back on the server that came back")

	showing, ok := after.Showing("Keychron K4 HE")
	require.True(t, ok)
	require.Equal(t, colour.MustParse("red"), showing.Colours[0],
		"the device that was asked for red came back showing something else")
}

/*
TestAUnitItCannotRestartIsAdvice is spec 058 AC7.

The distribution package ships a system unit, whose restart needs root. hotaru
does not ask for a password, and a lighting daemon that did would be the other
kind of annoying -- so it hands back the command instead, which is a sentence
somebody can act on.
*/
func TestAUnitItCannotRestartIsAdvice(t *testing.T) {
	svc := service.New(nil, openrgb.NewFake(board(), keyboard()), "")
	env := &restarts{machine: machine{bounce: &service.NotOurs{Command: "sudo systemctl restart openrgb.service"}}}
	env.svc = svc
	svc.SetEnvironment(env)

	_, err := svc.Rescan(t.Context())
	require.Error(t, err)
	require.False(t, env.done, "a unit needing root was restarted anyway")

	var notOurs *service.NotOurs
	require.ErrorAs(t, err, &notOurs)
	require.Contains(t, err.Error(), "sudo systemctl restart openrgb.service",
		"the command to run instead was not offered")
}

// A machine hotaru cannot ask about at all says so, rather than pretending the
// bounce happened.
func TestRescanWithNothingToAskSaysSo(t *testing.T) {
	svc := service.New(nil, openrgb.NewFake(board()), "")

	_, err := svc.Rescan(t.Context())
	var notOurs *service.NotOurs
	require.ErrorAs(t, err, &notOurs)
}

/*
TestARescanWaitsForDevicesThatHaveNotEnumeratedYet is the machine's own bug
report, from 28 Sep.

A restarted server answers before it has finished finding hardware, and it
answers without an error: asked immediately, it reported none of six devices
and no failure with it. The first rescan took that for the result and announced
"the server found 0 devices; restored 0" while the restore it had set off was
still running and the lights were, in fact, coming back.

So a reply is not readiness. What the server has is settled for, not sampled.
*/
func TestARescanWaitsForDevicesThatHaveNotEnumeratedYet(t *testing.T) {
	before := openrgb.NewFake(board(), keyboard())
	svc := service.New(nil, before, "")
	svc.SetRecorder(recorder(t))

	target, err := devices.ParseTarget("Keychron")
	require.NoError(t, err)
	_, err = svc.Apply(t.Context(), service.Request{
		Assignments: []devices.Assignment{{Target: target, Colour: colour.MustParse("red")}},
	})
	require.NoError(t, err)

	// The server that comes back is answering and has found nothing yet.
	after := openrgb.NewFake()
	svc.SetEnvironment(&restarts{svc: svc, after: after})

	go func() {
		time.Sleep(400 * time.Millisecond)
		after.Add(board())
		after.Add(keyboard())
	}()

	got, err := svc.Rescan(t.Context())
	require.NoError(t, err)
	require.Equal(t, 2, got.Devices,
		"reported what the server had before it finished finding its hardware")
	require.Empty(t, got.Missing, "gave up on devices that were still arriving")

	showing, ok := after.Showing("Keychron K4 HE")
	require.True(t, ok)
	require.Equal(t, colour.MustParse("red"), showing.Colours[0])
}
