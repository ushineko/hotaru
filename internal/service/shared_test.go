package service_test

import (
	"bytes"
	"image"
	"image/png"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/ushineko/hotaru/internal/cooler"
	"github.com/ushineko/hotaru/internal/cooler/coolertest"
	"github.com/ushineko/hotaru/internal/dashboard"
	"github.com/ushineko/hotaru/internal/images"
	"github.com/ushineko/hotaru/internal/readings"
	"github.com/ushineko/hotaru/internal/service/servicetest"
)

/*
Two askers within half a second share one reading: the dashboard loop asks,
and asks again for the trail, and each reading is a pass over every sensor,
nvidia-smi included (spec 081's profile). A later one reads again.
*/
func TestAReadingIsSharedForHalfASecond(t *testing.T) {
	svc := servicetest.New(t, nil)
	dev := coolertest.New()
	svc.SetCooler(cooler.New(dev))

	first := svc.Readings(t.Context())
	second := svc.Readings(t.Context())
	require.Equal(t, 1, dev.Statuses(), "the second asker read the sensors again")
	v1, _ := first.Value(readings.Coolant)
	v2, _ := second.Value(readings.Coolant)
	require.InDelta(t, v1, v2, 1e-9)

	second.Set(readings.Coolant, -1)
	again := svc.Readings(t.Context())
	v3, _ := again.Value(readings.Coolant)
	require.InDelta(t, v1, v3, 1e-9, "a caller's change reached the shared reading")

	time.Sleep(550 * time.Millisecond)
	svc.Readings(t.Context())
	require.Equal(t, 2, dev.Statuses(), "a reading older than half a second was shared")
}

// A dashboard's picture is decoded once and handed out again, and decoded
// again when the file changes.
func TestAPictureBehindIsDecodedOnce(t *testing.T) {
	svc := servicetest.New(t, nil)
	library, err := images.Open(filepath.Join(t.TempDir(), "images"))
	require.NoError(t, err)
	svc.SetImages(library)
	store, err := dashboard.Open(filepath.Join(t.TempDir(), "dashboards.yml"))
	require.NoError(t, err)
	svc.SetDashboards(store)

	var pic bytes.Buffer
	require.NoError(t, png.Encode(&pic, image.NewRGBA(image.Rect(0, 0, 64, 64))))
	_, err = library.Add("view", pic.Bytes())
	require.NoError(t, err)
	require.NoError(t, store.Save(dashboard.Dashboard{Name: "shown",
		Background: dashboard.Background{Kind: dashboard.Picture, Picture: "view"}}))
	require.NoError(t, svc.UseDashboard("shown"))

	_, first := svc.Look(t.Context())
	_, second := svc.Look(t.Context())
	require.NotNil(t, first)
	require.Same(t, first, second, "the picture was decoded again")

	time.Sleep(10 * time.Millisecond) // a later modification time
	var other bytes.Buffer
	require.NoError(t, png.Encode(&other, image.NewRGBA(image.Rect(0, 0, 32, 48))))
	_, err = library.Add("view", other.Bytes())
	require.NoError(t, err)
	_, third := svc.Look(t.Context())
	require.NotSame(t, first, third, "a replaced picture was not decoded again")
}
