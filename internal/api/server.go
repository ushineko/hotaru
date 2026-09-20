package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/ushineko/hotaru/internal/colour"
	"github.com/ushineko/hotaru/internal/devices"
	"github.com/ushineko/hotaru/internal/service"
	"github.com/ushineko/hotaru/internal/version"
)

/*
Handler serves the API over any transport an http.Server can take.

A plain http.Handler rather than a bespoke server, so the socket is a detail of
how it is listened on and the tests can drive it with httptest. The service is
the only thing it holds: this layer parses, calls, and encodes.
*/
func Handler(svc *service.Service) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /"+Version+"/health", func(w http.ResponseWriter, r *http.Request) {
		health := svc.Health(r.Context())
		write(w, http.StatusOK, Health{
			State:    string(health.State),
			Detail:   health.Detail,
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
		out.Zones = append(out.Zones, Zone{Name: zone.Name, First: zone.First, Count: zone.Count})
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
	out := service.Request{Off: req.Off, Devices: req.Devices}
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
	for _, zone := range view.Device.Zones {
		device.Zones = append(device.Zones, Zone{Name: zone.Name, First: zone.First, Count: zone.Count})
	}
	for _, col := range view.Device.Colours {
		device.Colours = append(device.Colours, col.String())
	}
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
		Device:  got.Device,
		Applied: got.Applied,
		Mode:    got.Mode,
		Skipped: got.Skipped,
	}
	if got.Err != nil {
		out.Error = got.Err.Error()
	}
	for _, attempt := range got.Attempts {
		out.Attempts = append(out.Attempts, Attempt{
			Mode:     attempt.Mode,
			Accepted: attempt.Accepted,
			Active:   attempt.Active,
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
