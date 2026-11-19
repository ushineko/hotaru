package service_test

import (
	"bytes"
	"image"
	"image/color"
	"image/color/palette"
	"image/gif"
	"image/png"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/ushineko/hotaru/internal/dashboard"
	"github.com/ushineko/hotaru/internal/images"
	"github.com/ushineko/hotaru/internal/service/servicetest"
)

// spinning is an animated GIF of three frames, 120 ms each.
func spinning(t *testing.T) []byte {
	t.Helper()
	g := &gif.GIF{}
	for i := range 3 {
		frame := image.NewPaletted(image.Rect(0, 0, 64, 64), palette.Plan9)
		frame.Set(i, 0, color.RGBA{R: 255, A: 255})
		g.Image = append(g.Image, frame)
		g.Delay = append(g.Delay, 12)
	}
	var out bytes.Buffer
	require.NoError(t, gif.EncodeAll(&out, g))
	return out.Bytes()
}

/*
An animated picture behind a dashboard is its frames and their delays, decoded
once and kept (spec 081 R5); a still picture, and a background that is not a
picture, are no frames.
*/
func TestABackdropIsAnAnimatedPicturesFrames(t *testing.T) {
	svc := servicetest.New(t, nil)
	library, err := images.Open(filepath.Join(t.TempDir(), "images"))
	require.NoError(t, err)
	svc.SetImages(library)

	_, err = library.Add("spin", spinning(t))
	require.NoError(t, err)
	var still bytes.Buffer
	require.NoError(t, png.Encode(&still, image.NewRGBA(image.Rect(0, 0, 64, 64))))
	_, err = library.Add("flat", still.Bytes())
	require.NoError(t, err)

	on := func(name string) dashboard.Dashboard {
		return dashboard.Dashboard{Background: dashboard.Background{Kind: dashboard.Picture, Picture: name}}
	}
	b := svc.Backdrop(t.Context(), on("spin"))
	require.Len(t, b.Frames, 3)
	require.Equal(t, []time.Duration{120 * time.Millisecond, 120 * time.Millisecond, 120 * time.Millisecond}, b.Delays)
	require.Equal(t, image.Rect(0, 0, images.Panel, images.Panel), b.Frames[0].Bounds())

	again := svc.Backdrop(t.Context(), on("spin"))
	require.Same(t, b.Frames[0], again.Frames[0], "the picture was decoded again")

	require.Empty(t, svc.Backdrop(t.Context(), on("flat")).Frames)
	require.Empty(t, svc.Backdrop(t.Context(), dashboard.Dashboard{}).Frames)
	require.Empty(t, svc.Backdrop(t.Context(), on("gone")).Frames)
}
