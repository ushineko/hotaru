package service_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ushineko/hotaru/internal/canvas/canvastest"
	"github.com/ushineko/hotaru/internal/colour"
	"github.com/ushineko/hotaru/internal/config"
	"github.com/ushineko/hotaru/internal/service"
	"github.com/ushineko/hotaru/internal/service/servicetest"
)

/*
A canvas whose rule asks for re-assertion is re-asserted, and sent the frame
it holds again; a restore of the same device, without the rule's asking,
sends nothing (spec 077 R5.1).
*/
func TestAReassertReachesACanvas(t *testing.T) {
	mouse := canvastest.NewBoard("G502 X PLUS", canvastest.Zone("Mouse LEDs", 8))
	cfg := &config.Config{Devices: []config.DeviceRule{{Match: "g502", Reassert: config.Duration(time.Minute)}}}
	svc := servicetest.New(t, cfg, mouse)
	svc.SetRecorder(recorder(t))
	red := colour.MustParse("red")
	_, err := svc.Apply(t.Context(), service.Request{Colour: &red})
	require.NoError(t, err)
	require.Eventually(t, func() bool { return mouse.Frames() == 1 }, eventually, every)

	rules, err := svc.ReassertRules(t.Context())
	require.NoError(t, err)
	assert.Equal(t, time.Minute, rules["G502 X PLUS"])

	_, err = svc.Reconcile(t.Context(), []string{"G502 X PLUS"})
	require.NoError(t, err)
	require.Never(t, func() bool { return mouse.Frames() != 1 }, 50*time.Millisecond, every,
		"a restore re-sent a held frame")

	_, err = svc.Reassert(t.Context(), []string{"G502 X PLUS"})
	require.NoError(t, err)
	require.Eventually(t, func() bool { return mouse.Frames() == 2 }, eventually, every, "the re-assert sent no frame")
}
