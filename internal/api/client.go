package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"time"
)

/*
Client talks to a running hotaru over its socket.

Every shell uses this and nothing else: the CLI, the GUI, and whatever imports
hotaru later. A shell holds no device handle and speaks to no daemon but this
one, which is what makes "the service is the only writer" true by construction
rather than by discipline.

The base URL's host is a fiction — the transport dials a path, not a name — but
http.Client wants one, and "hotaru" reads better in an error than a placeholder.
*/
type Client struct {
	http   *http.Client
	socket string
}

// DialTimeout is how long a client waits to reach the service. Short: the
// service is on the same machine, and a shell that hangs is worse than one that
// says the service is not running.
const DialTimeout = 2 * time.Second

// NewClient talks to the service listening on this socket path.
func NewClient(socket string) *Client {
	dialer := &net.Dialer{Timeout: DialTimeout}
	return &Client{
		socket: socket,
		http: &http.Client{
			Transport: &http.Transport{
				DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
					return dialer.DialContext(ctx, "unix", socket)
				},
			},
		},
	}
}

/*
NotRunning is the service not being there.

Its own type because every shell wants to say the same thing about it, in one
line, and exit non-zero — rather than printing a transport error about a socket
path the user never typed.
*/
type NotRunning struct {
	Socket string
	Err    error
}

func (e *NotRunning) Error() string {
	return fmt.Sprintf("hotaru is not running (no service on %s): start it with `systemctl --user start hotaru`", e.Socket)
}

func (e *NotRunning) Unwrap() error { return e.Err }

// Health is what the service can see, and what to do if that is not enough.
func (c *Client) Health(ctx context.Context) (Health, error) {
	var out Health
	err := c.do(ctx, http.MethodGet, "/"+Version+"/health", nil, &out)
	return out, err
}

// Devices is every device the service knows, with scope and rules resolved.
func (c *Client) Devices(ctx context.Context) ([]Device, error) {
	var out DevicesResponse
	if err := c.do(ctx, http.MethodGet, "/"+Version+"/devices", nil, &out); err != nil {
		return nil, err
	}
	return out.Devices, nil
}

// Apply asks the service to write lighting. The shell never writes a device
// itself; this is the whole of how it asks.
func (c *Client) Apply(ctx context.Context, req ApplyRequest) (ApplyResponse, error) {
	var out ApplyResponse
	err := c.do(ctx, http.MethodPost, "/"+Version+"/lighting/apply", req, &out)
	return out, err
}

// Probe asks the service to find out what each device can actually do. It
// writes, and puts everything back.
func (c *Client) Probe(ctx context.Context, req ProbeRequest) ([]Finding, error) {
	var out ProbeResponse
	err := c.do(ctx, http.MethodPost, "/"+Version+"/lighting/probe", req, &out)
	return out.Findings, err
}

// Status is what the service is and what it remembers.
func (c *Client) Status(ctx context.Context) (Status, error) {
	var out Status
	err := c.do(ctx, http.MethodGet, "/"+Version+"/status", nil, &out)
	return out, err
}

// Reconcile puts the lights back to what was last asked for.
func (c *Client) Reconcile(ctx context.Context) (RestoreResponse, error) {
	var out RestoreResponse
	err := c.do(ctx, http.MethodPost, "/"+Version+"/reconcile", struct{}{}, &out)
	return out, err
}

// Reload re-reads the rules file and reports what was wrong with it.
func (c *Client) Reload(ctx context.Context) (ReloadResponse, error) {
	var out ReloadResponse
	err := c.do(ctx, http.MethodPost, "/"+Version+"/reload", struct{}{}, &out)
	return out, err
}

func (c *Client) do(ctx context.Context, method, path string, body, out any) error {
	var reader io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("encode the request: %w", err)
		}
		reader = bytes.NewReader(encoded)
	}

	req, err := http.NewRequestWithContext(ctx, method, "http://hotaru"+path, reader)
	if err != nil {
		return fmt.Errorf("build a %s request for %s: %w", method, path, err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.http.Do(req)
	if err != nil {
		var opErr *net.OpError
		if errors.As(err, &opErr) || errors.Is(err, context.DeadlineExceeded) {
			return &NotRunning{Socket: c.socket, Err: err}
		}
		return fmt.Errorf("ask hotaru for %s: %w", path, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode >= http.StatusBadRequest {
		var failure Error
		if err := json.NewDecoder(resp.Body).Decode(&failure); err != nil || failure.Error == "" {
			return fmt.Errorf("hotaru answered %s", resp.Status)
		}
		if failure.Detail != "" {
			return fmt.Errorf("%s: %s", failure.Error, failure.Detail)
		}
		return errors.New(failure.Error)
	}

	if out == nil {
		return nil
	}
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return fmt.Errorf("read hotaru's answer to %s: %w", path, err)
	}
	return nil
}

/*
Cooling is what the liquid cooler reports, or that there is none.

Absence is not an error here: a machine without a cooler answers with Absent
set, because "no cooler" is a fact a caller wants rather than a failure it has
to distinguish from a broken socket.
*/
func (c *Client) Cooling(ctx context.Context) (Cooling, error) {
	var out Cooling
	err := c.do(ctx, http.MethodGet, "/"+Version+"/cooling", nil, &out)
	return out, err
}

/*
Screen puts something on the cooler's panel, or hands it back.

The GIF is encoded here rather than by the caller, because "base64" is a
detail of this transport and not something a command should have to know.
*/
func (c *Client) Screen(ctx context.Context, what ScreenRequest) error {
	return c.do(ctx, http.MethodPost, "/"+Version+"/screen", what, nil)
}

// Scenes is every saved scene.
func (c *Client) Scenes(ctx context.Context) ([]Scene, error) {
	var out ScenesResponse
	err := c.do(ctx, http.MethodGet, "/"+Version+"/scenes", nil, &out)
	return out.Scenes, err
}

// SaveScene writes a scene under a name, replacing one already there.
func (c *Client) SaveScene(ctx context.Context, scene Scene) error {
	return c.do(ctx, http.MethodPut, "/"+Version+"/scenes/"+url.PathEscape(scene.Name), scene, nil)
}

// DeleteScene removes one.
func (c *Client) DeleteScene(ctx context.Context, name string) error {
	return c.do(ctx, http.MethodDelete, "/"+Version+"/scenes/"+url.PathEscape(name), nil, nil)
}

/*
CaptureScene saves what the lights are showing now, under a name.

The shortcut somebody reaches for after fiddling until it looks right: the
scene is built from desired state rather than from anything the caller has to
describe.
*/
func (c *Client) CaptureScene(ctx context.Context, name, screen string) (Scene, error) {
	var out Scene
	err := c.do(ctx, http.MethodPost,
		"/"+Version+"/scenes/"+url.PathEscape(name)+"/capture", CaptureRequest{Screen: screen}, &out)
	return out, err
}

// ApplyScene lights a scene and records it as what the machine should show.
func (c *Client) ApplyScene(ctx context.Context, name string, req SceneRequest) (SceneResponse, error) {
	var out SceneResponse
	err := c.do(ctx, http.MethodPost,
		"/"+Version+"/scenes/"+url.PathEscape(name)+"/apply", req, &out)
	return out, err
}

/*
HoldScene previews a scene and keeps it up until the context ends.

The lease is this connection: the handler answers, then waits on the socket, so
letting go of the context -- or dying -- is what puts the lights back. The
response comes back as soon as the draft is up; the returned function blocks
until the preview is over, which is what a caller with a terminal waits on.

The alternative for a client that cannot sit on a connection is Preview plus
Renew, and it exists for exactly the clients that cannot do this.
*/
func (c *Client) HoldScene(ctx context.Context, name string, req SceneRequest) (SceneResponse, func(), error) {
	req.Preview, req.Hold = true, true
	encoded, err := json.Marshal(req)
	if err != nil {
		return SceneResponse{}, nil, fmt.Errorf("encode the request: %w", err)
	}

	request, err := http.NewRequestWithContext(ctx, http.MethodPost,
		"http://hotaru/"+Version+"/scenes/"+url.PathEscape(name)+"/apply", bytes.NewReader(encoded))
	if err != nil {
		return SceneResponse{}, nil, fmt.Errorf("build the preview request: %w", err)
	}
	request.Header.Set("Content-Type", "application/json")

	resp, err := c.http.Do(request)
	if err != nil {
		var opErr *net.OpError
		if errors.As(err, &opErr) || errors.Is(err, context.DeadlineExceeded) {
			return SceneResponse{}, nil, &NotRunning{Socket: c.socket, Err: err}
		}
		return SceneResponse{}, nil, fmt.Errorf("ask hotaru to preview %s: %w", name, err)
	}

	if resp.StatusCode >= http.StatusBadRequest {
		defer func() { _ = resp.Body.Close() }()
		var failure Error
		if err := json.NewDecoder(resp.Body).Decode(&failure); err != nil || failure.Error == "" {
			return SceneResponse{}, nil, fmt.Errorf("hotaru answered %s", resp.Status)
		}
		if failure.Detail != "" {
			return SceneResponse{}, nil, fmt.Errorf("%s: %s", failure.Error, failure.Detail)
		}
		return SceneResponse{}, nil, errors.New(failure.Error)
	}

	var out SceneResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		_ = resp.Body.Close()
		return SceneResponse{}, nil, fmt.Errorf("read hotaru's answer: %w", err)
	}
	// Closing the body closes the connection, which is what ends the lease.
	return out, func() { _ = resp.Body.Close() }, nil
}

// PreviewScene shows a scene under a lease the caller renews. For clients that
// cannot hold a connection open; those that can should use HoldScene.
func (c *Client) PreviewScene(ctx context.Context, name string, req SceneRequest) (SceneResponse, error) {
	req.Preview, req.Hold = true, false
	var out SceneResponse
	err := c.do(ctx, http.MethodPost,
		"/"+Version+"/scenes/"+url.PathEscape(name)+"/apply", req, &out)
	return out, err
}

// RenewPreview pushes a lease's expiry out.
func (c *Client) RenewPreview(ctx context.Context, token string) error {
	return c.do(ctx, http.MethodPost, "/"+Version+"/preview/renew", PreviewRequest{Token: token}, nil)
}

// ReleasePreview ends a preview and puts the lights back to what was asked for.
func (c *Client) ReleasePreview(ctx context.Context, token string) (RestoreResponse, error) {
	var out RestoreResponse
	err := c.do(ctx, http.MethodPost, "/"+Version+"/preview/release", PreviewRequest{Token: token}, &out)
	return out, err
}

// Keys are the shortcuts, what they apply, and what is in their way.
func (c *Client) Keys(ctx context.Context) (KeysResponse, error) {
	var out KeysResponse
	err := c.do(ctx, http.MethodGet, "/"+Version+"/keys", nil, &out)
	return out, err
}

// Bind points a key at a scene. An empty scene name unbinds it.
func (c *Client) Bind(ctx context.Context, key, scene string) error {
	return c.do(ctx, http.MethodPost, "/"+Version+"/keys/bind", BindRequest{Key: key, Scene: scene}, nil)
}

/*
ReleaseKeys removes another program's claim on hotaru's sequences.

Editing somebody else's configuration file, so it is never called on hotaru's
own initiative: a person is shown what is in the way and says yes.
*/
func (c *Client) ReleaseKeys(ctx context.Context) (int, error) {
	var out ReleasedResponse
	err := c.do(ctx, http.MethodPost, "/"+Version+"/keys/release", struct{}{}, &out)
	return out.Removed, err
}
