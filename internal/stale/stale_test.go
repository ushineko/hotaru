package stale_test

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/ushineko/hotaru/internal/service"
	"github.com/ushineko/hotaru/internal/stale"
)

/*
TestADeletedHandleIsNotHealthy is spec 058 AC2.

The state this is about is invisible from everything health looks at. The
server answers, the device count is right, the scope is right, and the hardware
is not being driven -- so a report that looks perfect is exactly the report a
dead connection produces, and the only thing that may replace it is this.
*/
func TestADeletedHandleIsNotHealthy(t *testing.T) {
	verdict := stale.Verdict{
		Stale:    true,
		Detail:   "the OpenRGB server is still addressing Keychron K4 HE at a connection that has gone.",
		Remedies: []string{"Have the server look again with `hotaru light rescan`."},
	}

	state, detail, remedies := verdict.Apply("healthy", "6 of 6 devices are in scope", nil)
	require.Equal(t, "stale", state, "a server holding a removed descriptor stayed healthy")
	require.Contains(t, detail, "Keychron K4 HE")
	require.Contains(t, remedies[0], "hotaru light rescan",
		"the state was reported without the command that fixes it")
}

/*
TestAWorseVerdictIsNotOverwritten is spec 058 AC4, from the other side.

A server that is unreachable, knows of nothing, or has everything out of scope
is already in a state with its own remedy, reached without looking at a single
descriptor. Replacing any of those with this one would be trading a better
answer for a worse one.
*/
func TestAWorseVerdictIsNotOverwritten(t *testing.T) {
	verdict := stale.Verdict{Stale: true, Detail: "something moved"}

	for _, state := range []string{"unreachable", "no-devices", "none-in-scope"} {
		got, detail, _ := verdict.Apply(state, "the original reason", nil)
		require.Equal(t, state, got, "%s was replaced by a staleness verdict", state)
		require.Equal(t, "the original reason", detail)
	}
}

// What it cannot tell, it does not claim: the zero verdict changes nothing.
func TestWhatItCannotSeeItDoesNotClaim(t *testing.T) {
	state, detail, remedies := stale.Verdict{}.Apply("healthy", "6 of 6 devices are in scope", nil)
	require.Equal(t, "healthy", state)
	require.Equal(t, "6 of 6 devices are in scope", detail)
	require.Empty(t, remedies)
}

/*
The clients and the service have to spell this the same.

They cannot share the constant without the clients linking against the service
they exist not to link against, so the agreement is asserted instead.
*/
func TestTheClientsAndTheServiceAgreeOnTheName(t *testing.T) {
	require.Equal(t, string(service.StateStale), stale.State)
	require.Equal(t, string(service.StateHealthy), stale.Healthy)
}
