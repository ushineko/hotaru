package openrgb_test

import (
	"context"
	"reflect"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/ushineko/hotaru/internal/openrgb"
)

/*
Every call that touches a device or a socket takes a context.

Not a style preference: a write to a sleeping wireless device can block, and a
shell that cannot give up is one a user has to kill. The two methods without a
context are the two that ask the client about itself rather than about the
hardware.
*/
func TestEveryDeviceCallCanBeGivenUpOn(t *testing.T) {
	client := reflect.TypeOf((*openrgb.Client)(nil)).Elem()
	contextType := reflect.TypeOf((*context.Context)(nil)).Elem()

	local := map[string]bool{"ProtocolVersion": true, "Close": true}

	for i := range client.NumMethod() {
		method := client.Method(i)
		if local[method.Name] {
			continue
		}
		require.Positive(t, method.Type.NumIn(), method.Name)
		require.True(t, method.Type.In(0).Implements(contextType) || method.Type.In(0) == contextType,
			"%s does not take a context, so a caller cannot give up on it", method.Name)
	}
}
