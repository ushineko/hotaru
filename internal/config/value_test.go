package config_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/ushineko/hotaru/internal/config"
)

func TestADurationIsWrittenTheWayAPersonWritesOne(t *testing.T) {
	var d config.Duration
	require.NoError(t, json.Unmarshal([]byte(`"90s"`), &d))
	require.Equal(t, 90*time.Second, d.Duration())

	require.NoError(t, json.Unmarshal([]byte(`"5m"`), &d))
	require.Equal(t, 5*time.Minute, d.Duration())

	// A bare number is seconds. Nanoseconds is nobody's intent in a file that
	// says how often to re-assert a mouse's colour.
	require.NoError(t, json.Unmarshal([]byte(`45`), &d))
	require.Equal(t, 45*time.Second, d.Duration())

	require.Error(t, json.Unmarshal([]byte(`"a while"`), &d))
}

func TestADurationSurvivesARoundTrip(t *testing.T) {
	out, err := json.Marshal(config.Duration(90 * time.Second))
	require.NoError(t, err)
	require.JSONEq(t, `"1m30s"`, string(out))
}

func TestAnLEDRangeIsInclusiveBecauseSomeoneCountedThem(t *testing.T) {
	var r config.LEDs
	require.NoError(t, json.Unmarshal([]byte(`[0, 11]`), &r))
	require.Equal(t, 0, r.First)
	require.Equal(t, 11, r.Last)
	require.Equal(t, 12, r.Count(), "[0, 11] is twelve LEDs, not eleven")

	require.ErrorContains(t, json.Unmarshal([]byte(`[3]`), &r), "two values")
	require.ErrorContains(t, json.Unmarshal([]byte(`"ring"`), &r), "pair")
}
