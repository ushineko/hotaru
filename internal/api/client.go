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
