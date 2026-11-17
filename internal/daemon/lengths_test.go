package daemon_test

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ushineko/hotaru/internal/canvas"
	"github.com/ushineko/hotaru/internal/canvas/canvastest"
	"github.com/ushineko/hotaru/internal/config"
	"github.com/ushineko/hotaru/internal/daemon"
	"github.com/ushineko/hotaru/internal/service"
)

// A rule's lengths are given before anything reads the keys, and a length the
// device refuses is reported (spec 075 R2.2).
func TestARulesLengthsAreGivenAtAttach(t *testing.T) {
	cfg := &config.Config{Devices: []config.DeviceRule{{
		Match:   "fake board",
		Lengths: map[string]int{"Header 1": 18, "Header 9": 4},
	}}}
	svc := service.New(cfg)
	device, ok := canvas.New(canvastest.NewBoard("fake board", canvastest.Zone("Board", 1), canvastest.Header("Header 1", 0, 500)))
	require.True(t, ok)
	var said []string
	daemon.Lengthen(svc, device, func(format string, args ...any) { said = append(said, fmt.Sprintf(format, args...)) })
	assert.Len(t, device.Keys(), 19)
	require.Len(t, said, 1)
	assert.Contains(t, said[0], "Header 9")
}
