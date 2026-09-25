package openrgb

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestASpeedIsHeldInsideTheModesOwnRange(t *testing.T) {
	/*
		A number out of range is held rather than refused: the range is the
		mode's and the slider that produced it may be a version behind, and a
		scene that will not apply because a speed is one too high would be a
		worse answer than the fastest the mode has.

		Backwards bounds are a driver that counts down, which OpenRGB has.
	*/
	for _, one := range []struct {
		name      string
		speed     int
		low, high uint32
		want      uint32
	}{
		{"inside", 100, 0, 255, 100},
		{"above", 900, 0, 255, 255},
		{"below", -5, 0, 255, 0},
		{"counting down", 900, 255, 0, 255},
		{"at the edge", 255, 0, 255, 255},
	} {
		t.Run(one.name, func(t *testing.T) {
			require.Equal(t, one.want, clampSpeed(one.speed, one.low, one.high))
		})
	}
}
