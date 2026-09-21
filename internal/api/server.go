package api

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/ushineko/hotaru/internal/colour"
	"github.com/ushineko/hotaru/internal/cooler"
	"github.com/ushineko/hotaru/internal/devices"
	"github.com/ushineko/hotaru/internal/images"
	"github.com/ushineko/hotaru/internal/readings"
	"github.com/ushineko/hotaru/internal/scenes"
	"github.com/ushineko/hotaru/internal/service"
	"github.com/ushineko/hotaru/internal/version"
)

/*
Routes lists every route the service serves.

Enumerated rather than implied, so a test can assert that both shells can reach
all of them. Parity is otherwise a thing people mean to keep and do not: a
capability added to the API and wired into only the GUI looks finished from
every angle except a terminal.
*/
func Routes() []string {
	return []string{
		"GET /" + Version + "/health",
		"GET /" + Version + "/devices",
		"GET /" + Version + "/status",
		"GET /" + Version + "/cooling",
		"POST /" + Version + "/screen",
		"POST /" + Version + "/lighting/apply",
		"POST /" + Version + "/lighting/probe",
		"GET /" + Version + "/scenes",
		"PUT /" + Version + "/scenes/{name}",
		"DELETE /" + Version + "/scenes/{name}",
		"POST /" + Version + "/scenes/{name}/apply",
		"POST /" + Version + "/scenes/{name}/capture",
		"GET /" + Version + "/readings",
		"GET /" + Version + "/images",
		"POST /" + Version + "/images/preview",
		"PUT /" + Version + "/images/{name}",
		"DELETE /" + Version + "/images/{name}",
		"POST /" + Version + "/images/{name}/scene",
		"POST /" + Version + "/images/{name}/show",
		"GET /" + Version + "/keys",
		"POST /" + Version + "/keys/bind",
		"POST /" + Version + "/preview",
		"POST /" + Version + "/preview/renew",
		"POST /" + Version + "/preview/release",
		"POST /" + Version + "/reconcile",
		"POST /" + Version + "/reload",
	}
}

/*
Handler serves the API over any transport an http.Server can take.

A plain http.Handler rather than a bespoke server, so the socket is a detail of
how it is listened on and a test can drive it with httptest. The service is the
only thing it holds: this layer parses, calls, and encodes.
*/
func Handler(svc *service.Service) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /"+Version+"/health", func(w http.ResponseWriter, r *http.Request) {
		health := svc.Health(r.Context())
		write(w, http.StatusOK, Health{
			State:    string(health.State),
			Detail:   health.Detail,
			Remedies: health.Remedies,
			Address:  health.Address,
			Protocol: health.Protocol,
			Devices:  health.Devices,
			InScope:  health.InScope,
			Version:  version.Version,
		})
	})

	mux.HandleFunc("GET /"+Version+"/devices", func(w http.ResponseWriter, r *http.Request) {
		views, err := svc.List(r.Context())
		if err != nil {
			fail(w, err)
			return
		}
		out := DevicesResponse{Devices: make([]Device, 0, len(views))}
		for _, view := range views {
			out.Devices = append(out.Devices, describe(view))
		}
		write(w, http.StatusOK, out)
	})

	mux.HandleFunc("POST /"+Version+"/lighting/apply", func(w http.ResponseWriter, r *http.Request) {
		var req ApplyRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			write(w, http.StatusBadRequest, Error{Error: "that request does not decode", Detail: err.Error()})
			return
		}

		parsed, err := parse(req)
		if err != nil {
			write(w, http.StatusBadRequest, Error{Error: err.Error()})
			return
		}

		results, err := svc.Apply(r.Context(), parsed)
		if err != nil {
			fail(w, err)
			return
		}

		out := ApplyResponse{Results: make([]Result, 0, len(results))}
		for _, got := range results {
			if got.Applied {
				out.Changed++
			}
			out.Results = append(out.Results, result(got))
		}
		write(w, http.StatusOK, out)
	})

	mux.HandleFunc("POST /"+Version+"/lighting/probe", func(w http.ResponseWriter, r *http.Request) {
		var req ProbeRequest
		if r.ContentLength > 0 {
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				write(w, http.StatusBadRequest, Error{Error: "that request does not decode", Detail: err.Error()})
				return
			}
		}
		findings, err := svc.Probe(r.Context(), req.Devices)
		if err != nil {
			fail(w, err)
			return
		}
		out := ProbeResponse{Findings: make([]Finding, 0, len(findings))}
		for _, found := range findings {
			out.Findings = append(out.Findings, finding(found))
		}
		write(w, http.StatusOK, out)
	})

	mux.HandleFunc("GET /"+Version+"/status", func(w http.ResponseWriter, r *http.Request) {
		health := svc.Health(r.Context())
		desired := svc.Desired()
		status := Status{
			Version:    version.Version,
			Connected:  health.State != service.StateUnreachable,
			Address:    health.Address,
			Protocol:   health.Protocol,
			RulesFile:  svc.RulesPath(),
			Remembered: sortedNames(desired.Names()),
		}
		write(w, http.StatusOK, status)
	})

	/*
		The cooler, or the fact that there is not one.

		A machine with no cooler answers 200 with Absent set, rather than 404
		or an error: "this machine has no cooler" is a fact a consumer wants,
		and making it an error means every caller writes the same special
		case. The same goes for a cooler that is present and would not answer
		-- that is Detail, not a failed request.
	*/
	mux.HandleFunc("GET /"+Version+"/cooling", func(w http.ResponseWriter, r *http.Request) {
		status, device, err := svc.Cooling(r.Context())
		switch {
		case errors.Is(err, cooler.ErrNoCooler):
			write(w, http.StatusOK, Cooling{Absent: true, Detail: err.Error()})
		case err != nil:
			write(w, http.StatusOK, Cooling{Device: device.Name, Absent: true, Detail: err.Error()})
		default:
			write(w, http.StatusOK, Cooling{
				Device:   device.Name,
				Coolant:  status.Coolant,
				PumpRPM:  status.PumpRPM,
				PumpDuty: status.PumpDuty,
				FanRPM:   status.FanRPM,
				FanDuty:  status.FanDuty,
				Taken:    status.Taken,
			})
		}
	})

	mux.HandleFunc("POST /"+Version+"/screen", func(w http.ResponseWriter, r *http.Request) {
		var in ScreenRequest
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
			fail(w, fmt.Errorf("read the request: %w", err))
			return
		}
		gif, err := base64.StdEncoding.DecodeString(in.Image)
		if err != nil {
			fail(w, fmt.Errorf("the image is not base64: %w", err))
			return
		}
		err = svc.Draw(r.Context(), service.Screen{
			Image: gif, Readout: in.Readout, Dashboard: in.Dashboard,
			Brightness: in.Brightness, Orientation: in.Orientation,
		})
		if err != nil {
			fail(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})

	mux.HandleFunc("GET /"+Version+"/scenes", func(w http.ResponseWriter, _ *http.Request) {
		saved, err := svc.Scenes()
		if err != nil {
			fail(w, err)
			return
		}
		out := ScenesResponse{Scenes: make([]Scene, 0, len(saved))}
		for _, scene := range saved {
			out.Scenes = append(out.Scenes, asScene(scene))
		}
		write(w, http.StatusOK, out)
	})

	mux.HandleFunc("PUT /"+Version+"/scenes/{name}", func(w http.ResponseWriter, r *http.Request) {
		var in Scene
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
			write(w, http.StatusBadRequest, Error{Error: "that request does not decode", Detail: err.Error()})
			return
		}
		in.Name = r.PathValue("name")
		if err := svc.SaveScene(fromScene(in)); err != nil {
			fail(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})

	mux.HandleFunc("DELETE /"+Version+"/scenes/{name}", func(w http.ResponseWriter, r *http.Request) {
		if err := svc.DeleteScene(r.PathValue("name")); err != nil {
			fail(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})

	mux.HandleFunc("POST /"+Version+"/scenes/{name}/capture", func(w http.ResponseWriter, r *http.Request) {
		var in CaptureRequest
		if r.Body != nil {
			_ = json.NewDecoder(r.Body).Decode(&in)
		}
		scene, err := svc.SceneFrom(r.PathValue("name"), in.Screen)
		if err != nil {
			fail(w, err)
			return
		}
		if err := svc.SaveScene(scene); err != nil {
			fail(w, err)
			return
		}
		write(w, http.StatusOK, asScene(scene))
	})

	mux.HandleFunc("POST /"+Version+"/scenes/{name}/apply", func(w http.ResponseWriter, r *http.Request) {
		var in SceneRequest
		if r.Body != nil {
			// An empty body is an ordinary apply: a caller with nothing to
			// say should not have to send "{}".
			_ = json.NewDecoder(r.Body).Decode(&in)
		}
		name := r.PathValue("name")

		if !in.Preview {
			outcome, err := svc.ApplyScene(r.Context(), name)
			if err != nil {
				fail(w, err)
				return
			}
			write(w, http.StatusOK, asOutcome(outcome))
			return
		}

		outcome, err := svc.PreviewScene(r.Context(), name, holderOf(in, r), in.Hold)
		if err != nil {
			fail(w, err)
			return
		}
		if !in.Hold || outcome.Lease == nil {
			write(w, http.StatusOK, asOutcome(outcome))
			return
		}

		/*
			The caller said it would sit on this request, so the lease is the
			connection. The response goes out now -- the client wants to know
			the draft is up -- and the handler then waits, doing nothing, until
			the socket closes. That close is the client going away, reported by
			the kernel, with no clock and no heartbeat involved.
		*/
		hold(w, r, svc, outcome)
	})

	mux.HandleFunc("GET /"+Version+"/readings", func(w http.ResponseWriter, r *http.Request) {
		taken := svc.Readings(r.Context())
		out := ReadingsResponse{Readings: make([]ReadingValue, 0, len(readings.All))}
		for _, source := range readings.All {
			label, unit := readings.Describe(source)
			value, known := taken.Value(source)
			out.Readings = append(out.Readings, ReadingValue{
				Source: string(source), Label: label, Unit: unit,
				Value: value, Known: known, Text: taken.Text(source),
			})
		}
		write(w, http.StatusOK, out)
	})

	mux.HandleFunc("GET /"+Version+"/images", func(w http.ResponseWriter, _ *http.Request) {
		stored, err := svc.Images()
		if err != nil {
			fail(w, err)
			return
		}
		out := ImagesResponse{Images: make([]Image, 0, len(stored))}
		for _, image := range stored {
			out.Images = append(out.Images, Image{
				Name: image.Name, Path: image.Path, Bytes: image.Bytes,
				Frames: image.Frames, Added: image.Added,
			})
		}
		write(w, http.StatusOK, out)
	})

	mux.HandleFunc("POST /"+Version+"/images/preview", func(w http.ResponseWriter, r *http.Request) {
		var in ImageRequest
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
			write(w, http.StatusBadRequest, Error{Error: "that request does not decode", Detail: err.Error()})
			return
		}
		source, err := base64.StdEncoding.DecodeString(in.Image)
		if err != nil {
			write(w, http.StatusBadRequest, Error{Error: "the image is not base64", Detail: err.Error()})
			return
		}

		converted, frames, err := svc.PreviewImage(source)
		if err != nil {
			fail(w, err)
			return
		}
		write(w, http.StatusOK, ConvertedImage{
			Image:  base64.StdEncoding.EncodeToString(converted),
			Bytes:  len(converted),
			Frames: frames,
		})
	})

	mux.HandleFunc("PUT /"+Version+"/images/{name}", func(w http.ResponseWriter, r *http.Request) {
		var in ImageRequest
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
			write(w, http.StatusBadRequest, Error{Error: "that request does not decode", Detail: err.Error()})
			return
		}
		stored, err := kept(svc, r.PathValue("name"), in)
		if err != nil {
			fail(w, err)
			return
		}
		write(w, http.StatusOK, Image{
			Name: stored.Name, Path: stored.Path, Bytes: stored.Bytes,
			Frames: stored.Frames, Added: stored.Added,
		})
	})

	mux.HandleFunc("DELETE /"+Version+"/images/{name}", func(w http.ResponseWriter, r *http.Request) {
		if err := svc.RemoveImage(r.PathValue("name")); err != nil {
			fail(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})

	mux.HandleFunc("POST /"+Version+"/images/{name}/scene", func(w http.ResponseWriter, r *http.Request) {
		var in SceneFromImageRequest
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
			write(w, http.StatusBadRequest, Error{Error: "that request does not decode", Detail: err.Error()})
			return
		}

		scene, err := svc.SceneFromImage(r.Context(), r.PathValue("name"), in.Scene)
		if err != nil {
			fail(w, err)
			return
		}
		write(w, http.StatusOK, asScene(scene))
	})

	mux.HandleFunc("POST /"+Version+"/images/{name}/show", func(w http.ResponseWriter, r *http.Request) {
		if err := svc.ShowImage(r.Context(), r.PathValue("name")); err != nil {
			fail(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})

	mux.HandleFunc("GET /"+Version+"/keys", func(w http.ResponseWriter, _ *http.Request) {
		keys, err := svc.Keys()
		if err != nil {
			fail(w, err)
			return
		}
		out := KeysResponse{Reserved: keys.Reserved, Desktop: keys.Desktop}
		for _, binding := range keys.Bindings {
			out.Bindings = append(out.Bindings, Binding{
				Key: binding.Key, Scene: binding.Scene, Missing: binding.Missing,
			})
		}
		for _, claim := range keys.Claimed {
			out.Claimed = append(out.Claimed,
				fmt.Sprintf("%s holds %s (in %s)", claim.Entry, claim.Key, claim.Component))
		}
		write(w, http.StatusOK, out)
	})

	mux.HandleFunc("POST /"+Version+"/keys/bind", func(w http.ResponseWriter, r *http.Request) {
		var in BindRequest
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
			write(w, http.StatusBadRequest, Error{Error: "that request does not decode", Detail: err.Error()})
			return
		}
		if err := svc.Bind(in.Key, in.Scene); err != nil {
			fail(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})

	/*
		Previewing a scene that has no name.

		The editor's route: a draft is not a saved scene and must not have to
		become one to be looked at. Everything else is the named form's, lease
		included.
	*/
	mux.HandleFunc("POST /"+Version+"/preview", func(w http.ResponseWriter, r *http.Request) {
		var in DraftRequest
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
			write(w, http.StatusBadRequest, Error{Error: "that request does not decode", Detail: err.Error()})
			return
		}

		outcome, err := svc.Preview(r.Context(), fromScene(in.Scene),
			holderOf(SceneRequest{Holder: in.Holder}, r), in.Hold)
		if err != nil {
			fail(w, err)
			return
		}
		if !in.Hold || outcome.Lease == nil {
			write(w, http.StatusOK, asOutcome(outcome))
			return
		}
		hold(w, r, svc, outcome)
	})

	mux.HandleFunc("POST /"+Version+"/preview/renew", func(w http.ResponseWriter, r *http.Request) {
		var in PreviewRequest
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
			write(w, http.StatusBadRequest, Error{Error: "that request does not decode", Detail: err.Error()})
			return
		}
		if err := svc.Renew(in.Token); err != nil {
			fail(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})

	mux.HandleFunc("POST /"+Version+"/preview/release", func(w http.ResponseWriter, r *http.Request) {
		var in PreviewRequest
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
			write(w, http.StatusBadRequest, Error{Error: "that request does not decode", Detail: err.Error()})
			return
		}
		restore, err := svc.Release(r.Context(), in.Token)
		if err != nil {
			fail(w, err)
			return
		}
		write(w, http.StatusOK, RestoreResponse{
			Applied: restore.Applied, Missing: restore.Missing, Complete: restore.Complete(),
		})
	})

	mux.HandleFunc("POST /"+Version+"/reconcile", func(w http.ResponseWriter, r *http.Request) {
		restore, err := svc.Reconcile(r.Context(), nil)
		if err != nil {
			fail(w, err)
			return
		}
		out := RestoreResponse{
			Applied:  restore.Applied,
			Missing:  restore.Missing,
			Complete: restore.Complete(),
		}
		for _, got := range restore.Results {
			out.Results = append(out.Results, result(got))
		}
		write(w, http.StatusOK, out)
	})

	mux.HandleFunc("POST /"+Version+"/reload", func(w http.ResponseWriter, _ *http.Request) {
		problems, err := svc.Reload()
		if err != nil {
			fail(w, err)
			return
		}
		out := ReloadResponse{RulesFile: svc.RulesPath()}
		for _, problem := range problems {
			out.Problems = append(out.Problems, problem.Error())
		}
		write(w, http.StatusOK, out)
	})

	return mux
}

func finding(in service.Finding) Finding {
	out := Finding{
		Device:    in.Device,
		NoOffMode: in.NoOffMode,
		Suggested: in.Suggested,
	}
	if in.Err != nil {
		out.Error = in.Err.Error()
	}
	for _, mode := range in.Modes {
		out.Modes = append(out.Modes, ModeFinding{
			Name: mode.Name, PerLED: mode.PerLED, Tried: mode.Tried, Took: mode.Took,
		})
	}
	for _, zone := range in.Zones {
		out.Zones = append(out.Zones, Zone{
			Name: zone.Name, Shape: string(zone.Shape), First: zone.First, Count: zone.Count,
		})
	}
	return out
}

func sortedNames(in []string) []string {
	out := append([]string(nil), in...)
	sortStrings(out)
	return out
}

// parse turns the wire's strings into the service's types. A client sends what
// a person would type; the knowledge of what those words mean lives here.
func parse(req ApplyRequest) (service.Request, error) {
	out := service.Request{
		Off: req.Off, Devices: req.Devices,
		Mode: req.Mode, Exactly: req.Exactly, Preview: req.Preview,
		Brightness: req.Brightness,
	}
	if req.Colour != "" {
		col, err := colour.Parse(req.Colour)
		if err != nil {
			return service.Request{}, fmt.Errorf("colour %q: %w", req.Colour, err)
		}
		out.Colour = &col
	}
	for _, a := range req.Assignments {
		target, err := devices.ParseTarget(a.Target)
		if err != nil {
			return service.Request{}, fmt.Errorf("target %q: %w", a.Target, err)
		}
		col, err := colour.Parse(a.Colour)
		if err != nil {
			return service.Request{}, fmt.Errorf("colour %q: %w", a.Colour, err)
		}
		out.Assignments = append(out.Assignments, devices.Assignment{Target: target, Colour: col})
	}
	if len(out.Assignments) == 0 && out.Colour == nil && !out.Off {
		return service.Request{}, errors.New("nothing to apply: give a colour, assignments, or off")
	}
	return out, nil
}

func describe(view service.View) Device {
	device := Device{
		Name:       view.Device.Name,
		LEDs:       view.Device.LEDCount,
		Modes:      view.Device.ModeNames(),
		ActiveMode: view.Device.ActiveMode,
		InScope:    view.InScope,
	}
	for _, mode := range view.Device.Modes {
		if mode.Brightness {
			device.Dimmable = append(device.Dimmable, mode.Name)
		}
	}
	for _, zone := range view.Device.Zones {
		device.Zones = append(device.Zones, Zone{
			Name: zone.Name, Shape: string(zone.Shape), First: zone.First, Count: zone.Count,
		})
	}
	for _, col := range view.Device.Colours {
		device.Colours = append(device.Colours, col.String())
	}
	device.Preview = asPreview(view.Preview)
	if view.Rule.Reassert > 0 {
		device.Reassert = view.Rule.Reassert.String()
	}
	for name := range view.Rule.Segments {
		device.Segments = append(device.Segments, name)
	}
	sortStrings(device.Segments)
	return device
}

func result(got service.Result) Result {
	out := Result{
		Device:      got.Device,
		Applied:     got.Applied,
		Mode:        got.Mode,
		Skipped:     got.Skipped,
		Superseded:  got.Superseded,
		Unconfirmed: got.Unconfirmed,
		Problems:    got.Problems,
	}
	if got.Err != nil {
		out.Error = got.Err.Error()
	}
	for _, attempt := range got.Attempts {
		out.Attempts = append(out.Attempts, Attempt{
			Mode:     attempt.Mode,
			Accepted: attempt.Accepted,
			Active:   attempt.Active,
			Showing:  attempt.Showing,
			Settled:  attempt.Settled,
			Why:      attempt.Why,
		})
	}
	return out
}

/*
fail maps a service error onto a status.

An unreachable OpenRGB server is 503: the request was fine, the machine is not
ready, and a client should say so rather than blame what was asked.
*/
func fail(w http.ResponseWriter, err error) {
	var down *service.Unreachable
	if errors.As(err, &down) {
		write(w, http.StatusServiceUnavailable, Error{Error: err.Error()})
		return
	}
	write(w, http.StatusInternalServerError, Error{Error: err.Error()})
}

func write(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ") // a transcript is read by people
	_ = enc.Encode(body)
}

func sortStrings(s []string) {
	for i := 1; i < len(s); i++ {
		for j := i; j > 0 && s[j] < s[j-1]; j-- {
			s[j], s[j-1] = s[j-1], s[j]
		}
	}
}

// ReadTimeout and friends are the server's manners, kept here so the daemon and
// any test server share them.
const (
	ReadHeaderTimeout = 5 * time.Second
	IdleTimeout       = 60 * time.Second
)

/*
kept stores a picture, or several as one slideshow.

One route rather than two, because the difference is how many pictures were
sent and a client that has to choose a path for that is a client doing the
service's arithmetic.
*/
func kept(svc *service.Service, name string, in ImageRequest) (images.Image, error) {
	if len(in.Images) > 0 {
		sources := make([][]byte, 0, len(in.Images))
		for i, encoded := range in.Images {
			source, err := base64.StdEncoding.DecodeString(encoded)
			if err != nil {
				return images.Image{}, fmt.Errorf("picture %d is not base64: %w", i+1, err)
			}
			sources = append(sources, source)
		}
		return svc.AddSlideshow(name, sources)
	}

	source, err := base64.StdEncoding.DecodeString(in.Image)
	if err != nil {
		return images.Image{}, fmt.Errorf("the image is not base64: %w", err)
	}
	return svc.AddImage(name, source)
}

// asScene is a saved scene on the wire.
func asScene(scene scenes.Scene) Scene {
	out := Scene{
		Name: scene.Name, Colour: scene.Colour, Off: scene.Off,
		Shipped: scene.Shipped, Effects: scene.Effects, Screen: scene.Screen,
	}
	for _, a := range scene.Assignments {
		out.Assignments = append(out.Assignments, SceneAssignment{Target: a.Target, Colour: a.Colour})
	}
	return out
}

// fromScene is the reverse, for a client saving one.
func fromScene(in Scene) scenes.Scene {
	out := scenes.Scene{
		Name: in.Name, Colour: in.Colour, Off: in.Off,
		Effects: in.Effects, Screen: in.Screen,
	}
	for _, a := range in.Assignments {
		out.Assignments = append(out.Assignments, scenes.Assignment{Target: a.Target, Colour: a.Colour})
	}
	return out
}

// asOutcome is what a scene did, on the wire.
func asOutcome(outcome service.SceneOutcome) SceneResponse {
	out := SceneResponse{Scene: outcome.Scene, Problems: outcome.Problems, Screen: outcome.Screen}
	for _, got := range outcome.Results {
		out.Results = append(out.Results, result(got))
	}
	out.Preview = asPreview(outcome.Lease)
	return out
}

// asPreview is a lease on the wire.
func asPreview(lease *service.Lease) *Preview {
	if lease == nil {
		return nil
	}
	out := &Preview{
		Token: lease.Token, Scene: lease.Scene,
		Holder: lease.Holder, Devices: lease.Devices,
	}
	if !lease.Expires.IsZero() {
		expires := lease.Expires
		out.Expires = &expires
	}
	return out
}

/*
holderOf names who is looking at a draft.

The caller's own description where it gave one, because "hotaru-gui on this
desktop" is more use in a listing than anything this layer can work out. A
caller that said nothing gets the truthful minimum.
*/
func holderOf(in SceneRequest, r *http.Request) string {
	if in.Holder != "" {
		return in.Holder
	}
	if agent := r.UserAgent(); agent != "" {
		return agent
	}
	return "an API client"
}

/*
hold keeps a preview alive for as long as the caller keeps the request open.

The response is written and flushed first: the client is waiting to hear that
the draft is up, and a body nobody sends is a client that blocks forever. After
that this goroutine does nothing at all until the request context ends, which
happens when the socket closes -- deliberately, or because the process on the
other end died.
*/
func hold(w http.ResponseWriter, r *http.Request, svc *service.Service, outcome service.SceneOutcome) {
	write(w, http.StatusOK, asOutcome(outcome))
	if flusher, ok := w.(http.Flusher); ok {
		flusher.Flush()
	}

	<-r.Context().Done()

	/*
		The request's context is already cancelled, so the revert cannot use
		it: every write it makes would be cancelled before it left. A fresh
		context with a bound of its own is the difference between putting
		somebody's lights back and merely intending to.
	*/
	ctx, cancel := context.WithTimeout(context.WithoutCancel(r.Context()), revertWithin)
	defer cancel()
	if outcome.Lease != nil {
		_, _ = svc.Release(ctx, outcome.Lease.Token)
	}
}

// revertWithin bounds putting the lights back after a preview's holder goes
// away. Long enough for every device, short enough not to pile up.
const revertWithin = 30 * time.Second
