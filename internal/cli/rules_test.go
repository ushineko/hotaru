package cli

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/ushineko/hotaru/internal/config"
)

func TestTogglesSurviveTheWizardRewritingTheFile(t *testing.T) {
	/*
		The wizard renders the whole configuration back, so anything this
		renderer does not know about is something a person loses by running
		the wizard a second time -- and they would not notice until their
		toggles stopped having a control in the window.
	*/
	written := yamlFor(&config.Config{Devices: []config.DeviceRule{{
		Match: "keychron",
		Segments: map[string]config.Segment{
			"caps": {Zone: "Keyboard", LEDs: &config.LEDs{First: 55, Last: 55}},
		},
		Toggles: []string{"caps"},
	}}})
	require.Contains(t, written, "toggles: [caps]")

	again, problems, err := config.Parse("rules.yml", []byte(written))
	require.NoError(t, err)
	require.Empty(t, problems, "the file this renderer wrote does not load cleanly")
	require.Equal(t, []string{"caps"}, again.Devices[0].Toggles)
}
