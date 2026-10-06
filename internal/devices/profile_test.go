package devices_test

import (
	"math"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/ushineko/hotaru/internal/colour"
	"github.com/ushineko/hotaru/internal/config"
	"github.com/ushineko/hotaru/internal/devices"
)

// hsv is a colour's hue in degrees, and its saturation and value from 0 to 1.
func hsv(c colour.Colour) (h, s, v float64) {
	r, g, b := float64(c.R)/255, float64(c.G)/255, float64(c.B)/255
	hi, lo := max(r, g, b), min(r, g, b)
	v = hi
	if hi == 0 {
		return 0, 0, 0
	}
	s = (hi - lo) / hi
	if hi == lo {
		return 0, s, v
	}
	switch hi {
	case r:
		h = math.Mod((g-b)/(hi-lo), 6)
	case g:
		h = (b-r)/(hi-lo) + 2
	default:
		h = (r-g)/(hi-lo) + 4
	}
	return math.Mod(h*60+360, 360), s, v
}

func TestIce1sFanColourComesOutAsTheCardWasMatched(t *testing.T) {
	/*
		The measurement spec 066 starts from: in the scene ice1, the RTX
		4090 was matched by eye to the fans' #80aad1 by writing it #003eff.
		Saturation and value at full, and a curve on green, land on it.
	*/
	card := devices.Profile{Saturation: 1, Value: 1, Green: 2.15}
	got := card.Apply(colour.MustParse("#80aad1"))
	require.Equal(t, "#003eff", got.String())

	// Without the curve the hue is the one asked for, at full strength.
	plain := devices.Profile{Saturation: 1, Value: 1}.Apply(colour.MustParse("#80aad1"))
	h, s, v := hsv(plain)
	asked, _, _ := hsv(colour.MustParse("#80aad1"))
	require.InDelta(t, asked, h, 1, plain.String())
	require.InDelta(t, 1, s, 0.01, plain.String())
	require.InDelta(t, 1, v, 0.01, plain.String())
}

func TestAProfileNeverLightsTheDarkOrInventsAHue(t *testing.T) {
	full := devices.Profile{Saturation: 1, Value: 1, Green: 2.15}
	for _, c := range []string{"black", "white", "red", "green", "blue"} {
		require.Equal(t, colour.MustParse(c), full.Apply(colour.MustParse(c)), c)
	}

	full = devices.Profile{Saturation: 1, Value: 0.5}

	grey := full.Apply(colour.MustParse("#404040"))
	require.Equal(t, grey.R, grey.G, "a grey was given a hue: %s", grey)
	require.Equal(t, grey.G, grey.B, "a grey was given a hue: %s", grey)

	// Part of the way is part of the way: halfway from 62% saturation.
	half := devices.Profile{Saturation: 0.5}
	_, s, v := hsv(half.Apply(colour.MustParse("#80aad1")))
	_, s0, v0 := hsv(colour.MustParse("#80aad1"))
	require.InDelta(t, s0+(1-s0)/2, s, 0.01)
	require.InDelta(t, v0, v, 0.01, "value was not asked to move")

	// The zero profile is no profile.
	for _, c := range []string{"#80aad1", "#123456", "#ff8800"} {
		require.Equal(t, colour.MustParse(c), devices.Profile{}.Apply(colour.MustParse(c)))
	}
}

func TestColourProfilesMergeOneSettingAtATime(t *testing.T) {
	saturation, value, later, green, blue := 0.8, 0.6, 1.0, 2.0, 1.5
	rule := devices.MergeRules([]config.DeviceRule{
		{Match: "msi", Colour: &config.ColourProfile{Saturation: &saturation, Value: &value,
			Curve: &config.Curve{Green: &green}}},
		{Match: "geforce", Colour: &config.ColourProfile{Value: &later, Curve: &config.Curve{Blue: &blue}}},
	})
	require.Equal(t, devices.Profile{Saturation: 0.8, Value: 1, Green: 2, Blue: 1.5}, rule.Colour)
}
