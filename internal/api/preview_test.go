package api_test

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"image"
	"image/gif"
	"image/png"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/ushineko/hotaru/internal/api"
	"github.com/ushineko/hotaru/internal/dashboard"
	"github.com/ushineko/hotaru/internal/service"
)

/*
The preview is what the panel would show (spec 080): a PNG of the full-colour
frame for a panel that takes frames, with no floor, and the GIF with its floor
for one that does not.
*/
func TestThePreviewIsInThePanelsFormat(t *testing.T) {
	for _, c := range []struct {
		frames bool
		format string
		floor  float64
	}{
		{frames: true, format: "png", floor: 0},
		{frames: false, format: "gif", floor: 2},
	} {
		t.Run(c.format, func(t *testing.T) {
			svc := service.New(nil)
			store, err := dashboard.Open(filepath.Join(t.TempDir(), "dashboards.yml"))
			require.NoError(t, err)
			svc.SetDashboards(store)
			svc.SetCooler(fakeCooler{frames: c.frames})

			rec := httptest.NewRecorder()
			path := "/" + api.Version + "/dashboards/" + dashboard.Shipped()[0].Name + "/preview"
			api.Handler(svc).ServeHTTP(rec, httptest.NewRequestWithContext(t.Context(), http.MethodPost, path, nil))
			require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

			var got api.PreviewedDashboard
			require.NoError(t, json.NewDecoder(rec.Body).Decode(&got))
			require.Equal(t, c.format, got.Format)
			require.InDelta(t, c.floor, got.Floor, 0.001)
			body, err := base64.StdEncoding.DecodeString(got.Image)
			require.NoError(t, err)
			require.Equal(t, len(body), got.Bytes)

			var decoded image.Image
			if c.frames {
				decoded, err = png.Decode(bytes.NewReader(body))
			} else {
				decoded, err = gif.Decode(bytes.NewReader(body))
			}
			require.NoError(t, err)
			require.Equal(t, image.Rect(0, 0, dashboard.Size, dashboard.Size), decoded.Bounds())
		})
	}
}
