package api

import (
	"bytes"
	"context"
	"encoding/base64"
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

// Readings is every number this machine can put on the panel, each with its
// label, its unit, and whether the machine had it to give.
func (c *Client) Readings(ctx context.Context) ([]ReadingValue, error) {
	var out ReadingsResponse
	err := c.do(ctx, http.MethodGet, "/"+Version+"/readings", nil, &out)
	return out.Readings, err
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
	return c.stream(ctx, "/"+Version+"/scenes/"+url.PathEscape(name)+"/apply", req)
}

/*
stream makes a request the server answers and then keeps open.

The lease mechanism, on the client's side: the response comes back as soon as
the draft is up, and the connection stays until the returned function closes
it. Closing the body is what ends the preview, which is why it is handed back
rather than deferred here -- and why a client that simply dies ends it too.

Not c.do: that reads the body to completion and closes it, which is exactly
what must not happen.
*/
func (c *Client) stream(ctx context.Context, path string, body any) (SceneResponse, func(), error) {
	encoded, err := json.Marshal(body)
	if err != nil {
		return SceneResponse{}, nil, fmt.Errorf("encode the request: %w", err)
	}

	request, err := http.NewRequestWithContext(ctx, http.MethodPost,
		"http://hotaru"+path, bytes.NewReader(encoded))
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
		return SceneResponse{}, nil, fmt.Errorf("ask hotaru to preview: %w", err)
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
HoldDraft previews a scene that has no name, keeping it up until the returned
function is called or the process ends.

The editor's call. The lease is this connection: letting go of it -- or dying --
is what puts the lights back, with no release call and no clock.
*/
func (c *Client) HoldDraft(ctx context.Context, scene Scene, holder string) (SceneResponse, func(), error) {
	return c.stream(ctx, "/"+Version+"/preview",
		DraftRequest{Scene: scene, Hold: true, Holder: holder})
}

// PreviewDraft previews an unnamed scene under a lease the caller renews, for
// a client that cannot hold a connection open.
func (c *Client) PreviewDraft(ctx context.Context, scene Scene, holder string) (SceneResponse, error) {
	var out SceneResponse
	err := c.do(ctx, http.MethodPost, "/"+Version+"/preview",
		DraftRequest{Scene: scene, Holder: holder}, &out)
	return out, err
}

// Images is the stored pictures, converted to what the panel takes.
func (c *Client) Images(ctx context.Context) ([]Image, error) {
	var out ImagesResponse
	err := c.do(ctx, http.MethodGet, "/"+Version+"/images", nil, &out)
	return out.Images, err
}

/*
ConvertImage converts a picture and does not keep it, so a caller can look at
what the panel would show before deciding.
*/
func (c *Client) ConvertImage(ctx context.Context, source []byte) ([]byte, int, error) {
	var out ConvertedImage
	err := c.do(ctx, http.MethodPost, "/"+Version+"/images/preview",
		ImageRequest{Image: base64.StdEncoding.EncodeToString(source)}, &out)
	if err != nil {
		return nil, 0, err
	}
	converted, err := base64.StdEncoding.DecodeString(out.Image)
	if err != nil {
		return nil, 0, fmt.Errorf("read the converted image: %w", err)
	}
	return converted, out.Frames, nil
}

// AddImage converts a picture and stores it under a name.
func (c *Client) AddImage(ctx context.Context, name string, source []byte) (Image, error) {
	var out Image
	err := c.do(ctx, http.MethodPut, "/"+Version+"/images/"+url.PathEscape(name),
		ImageRequest{Image: base64.StdEncoding.EncodeToString(source)}, &out)
	return out, err
}

/*
AddSlideshow turns several pictures into one animation and stores it under a
name.
*/
func (c *Client) AddSlideshow(ctx context.Context, name string, sources [][]byte) (Image, error) {
	encoded := make([]string, 0, len(sources))
	for _, source := range sources {
		encoded = append(encoded, base64.StdEncoding.EncodeToString(source))
	}

	var out Image
	err := c.do(ctx, http.MethodPut, "/"+Version+"/images/"+url.PathEscape(name),
		ImageRequest{Images: encoded}, &out)
	return out, err
}

// RemoveImage forgets one.
func (c *Client) RemoveImage(ctx context.Context, name string) error {
	return c.do(ctx, http.MethodDelete, "/"+Version+"/images/"+url.PathEscape(name), nil, nil)
}

/*
SceneFromImage builds a scene whose lights match a picture and saves it.

Every light gets the part of the image at its own position in its zone, so a
run of lights carries the picture's own sweep rather than one averaged colour.
*/
func (c *Client) SceneFromImage(ctx context.Context, picture, scene string) (Scene, error) {
	var out Scene
	err := c.do(ctx, http.MethodPost,
		"/"+Version+"/images/"+url.PathEscape(picture)+"/scene",
		SceneFromImageRequest{Scene: scene}, &out)
	return out, err
}

// ShowImage puts a stored picture on the panel, taking it from the dashboard.
func (c *Client) ShowImage(ctx context.Context, name string) error {
	return c.do(ctx, http.MethodPost,
		"/"+Version+"/images/"+url.PathEscape(name)+"/show", struct{}{}, nil)
}
