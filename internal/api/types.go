/*
Package api is the contract between hotaru's service and everything that talks
to it.

HTTP and JSON over a Unix socket in $XDG_RUNTIME_DIR. A microservice in shape —
versioned paths, a request per operation — over a transport scoped to one user's
session. There is no TCP listener: the socket is what makes an unauthenticated
API defensible, because filesystem permissions already say "this user's session
and nothing else", and adding a port would mean adding authentication to go with
it for a program that changes the colour of lights.

Being HTTP still pays. `curl --unix-socket` is a debugging session, every
language has a client, and the shapes below are readable by a person reading a
transcript.

**These types are an interface, not a convenience.** The CLI is the first
client, the GUI is the second, and a future Go rewrite of the battery monitor is
meant to be the third. Changing a field here is a breaking change.

Everything on the wire is a string a person could have typed: targets like
"kraken/fan-top", colours like "#ff8800" or "red". Parsing happens on the
service's side of the socket, so a client needs no device knowledge at all —
and a transcript reads as what someone meant.
*/
package api

// Version is the path prefix every route sits under.
const Version = "v1"

// Device is one controller, as a client sees it.
type Device struct {
	Name       string   `json:"name"`
	LEDs       int      `json:"leds"`
	Modes      []string `json:"modes"`
	ActiveMode string   `json:"active_mode,omitempty"`
	Zones      []Zone   `json:"zones,omitempty"`
	Colours    []string `json:"colours,omitempty"`

	// InScope is whether hotaru would drive this device. Present so a listing
	// answers "why did nothing happen to this one?" without a second call.
	InScope bool `json:"in_scope"`

	// Reassert, when set, is how often this device's colour is re-sent,
	// because it does not hold what it is told.
	Reassert string `json:"reassert,omitempty"`
	// Segments are the names this machine's owner gave parts of the device.
	Segments []string `json:"segments,omitempty"`
}

// Zone is a contiguous run of LEDs, as the device groups them.
type Zone struct {
	Name  string `json:"name"`
	First int    `json:"first"`
	Count int    `json:"count"`
}

// DevicesResponse is the body of GET /v1/devices.
type DevicesResponse struct {
	Devices []Device `json:"devices"`
}

/*
Assignment is one colour applied to one target.

Target is "device", "device/zone", "device/zone[first:last]" or
"device/segment". Colour is a name or hex.
*/
type Assignment struct {
	Target string `json:"target"`
	Colour string `json:"colour"`
}

/*
ApplyRequest is the body of POST /v1/lighting/apply.

Off turns devices off rather than colouring them; it is a separate field rather
than a magic colour value, because "off" is an intent a device may be unable to
express, and black is not the same thing as off on hardware with a backlight.
*/
type ApplyRequest struct {
	// Colour applies to every device in scope, before any assignments. A
	// client that wants "everything red" says so here rather than listing the
	// devices it would have to have fetched first.
	Colour string `json:"colour,omitempty"`

	Assignments []Assignment `json:"assignments,omitempty"`
	Off         bool         `json:"off,omitempty"`
	Devices     []string     `json:"devices,omitempty"`
}

/*
Result is what happened to one device.

Applied, skipped and failed are three different things and stay three different
fields. A skip is a device saying it cannot do this and why — not an error, and
not something to retry.
*/
type Result struct {
	Device   string    `json:"device"`
	Applied  bool      `json:"applied"`
	Mode     string    `json:"mode,omitempty"`
	Skipped  string    `json:"skipped,omitempty"`
	Error    string    `json:"error,omitempty"`
	Attempts []Attempt `json:"attempts,omitempty"`
}

// Attempt is one mode that was tried, and what the device did with it.
type Attempt struct {
	Mode     string `json:"mode"`
	Accepted bool   `json:"accepted"`
	Active   string `json:"active,omitempty"`
	Why      string `json:"why,omitempty"`
}

// ApplyResponse is the body of POST /v1/lighting/apply.
type ApplyResponse struct {
	Results []Result `json:"results"`
	// Changed is how many devices actually changed. A request that changed
	// nothing does not report success, so a client can exit non-zero without
	// counting the results itself.
	Changed int `json:"changed"`
}

/*
Health is the body of GET /v1/health.

State is one of "unreachable", "no-devices", "none-in-scope", "healthy", and
Detail says what to do about it in a sentence meant for a person.
*/
type Health struct {
	State    string `json:"state"`
	Detail   string `json:"detail"`
	Address  string `json:"address"`
	Protocol uint32 `json:"protocol,omitempty"`
	Devices  int    `json:"devices"`
	InScope  int    `json:"in_scope"`
	Version  string `json:"version"`
}

/*
Finding is what probing learned about one device.

Probing writes: it sets modes to see which ones a device honours, and puts
everything back afterwards. It is a POST for that reason.
*/
type Finding struct {
	Device    string        `json:"device"`
	Modes     []ModeFinding `json:"modes,omitempty"`
	Zones     []Zone        `json:"zones,omitempty"`
	NoOffMode bool          `json:"no_off_mode,omitempty"`
	Suggested string        `json:"suggested,omitempty"`
	Error     string        `json:"error,omitempty"`
}

// ModeFinding is one mode, and what the device did with it. Tried without Took
// is the interesting case: accepted, and not honoured.
type ModeFinding struct {
	Name   string `json:"name"`
	PerLED bool   `json:"per_led"`
	Tried  bool   `json:"tried"`
	Took   bool   `json:"took"`
}

// ProbeRequest narrows probing to particular devices.
type ProbeRequest struct {
	Devices []string `json:"devices,omitempty"`
}

// ProbeResponse is the body of POST /v1/lighting/probe.
type ProbeResponse struct {
	Findings []Finding `json:"findings"`
}

/*
Status is the body of GET /v1/status: what the service is, rather than what it
can see. Health answers "why is nothing happening"; this answers "what is
running, and what does it remember".
*/
type Status struct {
	Version    string   `json:"version"`
	Connected  bool     `json:"connected"`
	Address    string   `json:"address"`
	Protocol   uint32   `json:"protocol,omitempty"`
	RulesFile  string   `json:"rules_file,omitempty"`
	Remembered []string `json:"remembered,omitempty"`
}

// RestoreResponse is the body of POST /v1/reconcile.
type RestoreResponse struct {
	Results  []Result `json:"results,omitempty"`
	Applied  int      `json:"applied"`
	Missing  []string `json:"missing,omitempty"`
	Complete bool     `json:"complete"`
}

// ReloadResponse is the body of POST /v1/reload: what was wrong with the rules
// file, entry by entry.
type ReloadResponse struct {
	RulesFile string   `json:"rules_file,omitempty"`
	Problems  []string `json:"problems,omitempty"`
}

// Error is the body of anything that went wrong, with the same shape whatever
// the status code, so a client parses one thing.
type Error struct {
	Error  string `json:"error"`
	Detail string `json:"detail,omitempty"`
}
