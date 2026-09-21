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

import "time"

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
	/*
		Dimmable is the modes that take a brightness, by name.

		Not a single flag for the device. A Keychron's Direct mode -- the one
		hotaru writes a colour in -- takes no brightness, while its animated
		cycle modes all do. Asked as "can this device be dimmed?", the answer
		is yes and the dimming does nothing: offered, demonstrated, written
		down, and inert. The question is only answerable about a mode.
	*/
	Dimmable []string `json:"dimmable,omitempty"`

	InScope bool `json:"in_scope"`

	// Preview, when set, is who is holding a draft on this device and what
	// they are looking at. A device whose re-assertion is suspended otherwise
	// looks identical to one that is simply behaving.
	Preview *Preview `json:"preview,omitempty"`

	// Reassert, when set, is how often this device's colour is re-sent,
	// because it does not hold what it is told.
	Reassert string `json:"reassert,omitempty"`
	// Segments are the names this machine's owner gave parts of the device.
	Segments []string `json:"segments,omitempty"`
}

/*
Zone shapes, as a device describes its own.

On the wire because a client needs them and cannot import the device packages:
only a line of lights can be several separate things on one connector, which is
the difference between a question worth asking somebody and one that invites
the answer "a hundred keys".
*/
const (
	ShapeSingle = "single"
	ShapeLine   = "line"
	ShapeGrid   = "grid"
)

// Zone is a contiguous run of LEDs, as the device groups them.
type Zone struct {
	Name string `json:"name"`
	// Shape is "single", "line" or "grid". Only a line can be several separate
	// things sharing one connector; a grid is one object with many lights.
	Shape string `json:"shape,omitempty"`
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

	// Mode is a mode to prefer over hotaru's usual order. The mapping wizard
	// uses it once a person has told it which mode actually lights a device --
	// a question no read-back can settle, since the mode it would otherwise
	// choose is accepted and reported back while nothing lights.
	//
	// Preferred, not forced: a mode that cannot carry the frame is skipped
	// rather than used to show one colour where several were asked for.
	Mode string `json:"mode,omitempty"`

	// Preview writes without remembering. What the mapping wizard lights is
	// not what the user wants their machine to look like: it is a question
	// being asked, and recording it would mean "put the lights back" put back
	// the questions.
	Preview bool `json:"preview,omitempty"`

	// Brightness overrides the rule's, for this write only. The wizard needs
	// it to show somebody what "turned down" looks like before there is a
	// rule saying so -- demonstrating rather than asking for a number.
	Brightness *int `json:"brightness,omitempty"`

	// Exactly stops the fall-through, so the answer is about the mode that was
	// asked for and no other. The wizard needs this: "what does Custom do on
	// this device" cannot be answered by quietly trying Direct instead and
	// reporting that it worked.
	Exactly bool `json:"exactly,omitempty"`
}

/*
Result is what happened to one device.

Applied, skipped and failed are three different things and stay three different
fields. A skip is a device saying it cannot do this and why — not an error, and
not something to retry.
*/
type Result struct {
	Device  string `json:"device"`
	Applied bool   `json:"applied"`
	Mode    string `json:"mode,omitempty"`
	Skipped string `json:"skipped,omitempty"`
	// Superseded is a write replaced by a newer one for the same device before
	// it ran. Not a failure, and not a device declining: the caller asked for
	// something else immediately afterwards, and that is what happened.
	Superseded bool      `json:"superseded,omitempty"`
	Error      string    `json:"error,omitempty"`
	Attempts   []Attempt `json:"attempts,omitempty"`
	// Unconfirmed is a device that accepted the write and will not say what it
	// is showing. Some hardware reports a stale buffer while displaying
	// exactly what it was sent, so this is a note rather than a failure.
	Unconfirmed string `json:"unconfirmed,omitempty"`

	// Problems are assignments that named something the device does not have.
	// Present even on a write that succeeded: an exception that did not apply
	// is a scene quietly doing something other than what was asked.
	Problems []string `json:"problems,omitempty"`
}

// Attempt is one mode that was tried, and what the device did with it.
type Attempt struct {
	Mode     string `json:"mode"`
	Accepted bool   `json:"accepted"`
	Active   string `json:"active,omitempty"`
	// Showing is whether the device was displaying the colours it was sent.
	// False with Active matching Mode is a device that took the mode and
	// ignored the frame -- which looks like success to everything but a read.
	Showing bool `json:"showing"`
	// Settled is a device that needed the colours written twice, because its
	// mode change had not finished when the first frame arrived.
	Settled bool   `json:"settled,omitempty"`
	Why     string `json:"why,omitempty"`
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
	State  string `json:"state"`
	Detail string `json:"detail"`
	// Remedies are things a person could do about it, each a sentence with a
	// command in it.
	Remedies []string `json:"remedies,omitempty"`
	Address  string   `json:"address"`
	Protocol uint32   `json:"protocol,omitempty"`
	Devices  int      `json:"devices"`
	InScope  int      `json:"in_scope"`
	Version  string   `json:"version"`
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

/*
Cooling is the body of GET /v1/cooling: what the liquid cooler reports.

Raw measurements and when they were taken, rather than what any one view needs.
The pump-failure alert moves here later and a future Go rewrite of
peripheral-battery-monitor reads the same snapshot, so a field nobody currently
draws is still worth carrying. See spec 012.
*/
type Cooling struct {
	// Device is what was found, empty on a machine with no cooler.
	Device string `json:"device,omitempty"`

	Coolant  float64 `json:"coolant_c"`
	PumpRPM  int     `json:"pump_rpm"`
	PumpDuty int     `json:"pump_duty"`
	FanRPM   int     `json:"fan_rpm"`
	FanDuty  int     `json:"fan_duty"`

	// Taken is when the cooler was read, so a consumer can judge staleness
	// rather than assuming the number is current.
	Taken time.Time `json:"taken"`

	// Absent says there is no cooler on this machine, which is an ordinary
	// state and not an error. Detail says why, when there is more to say.
	Absent bool   `json:"absent,omitempty"`
	Detail string `json:"detail,omitempty"`
}

/*
ScreenRequest is the body of POST /v1/screen.

Exactly one thing at a time. A GIF arrives base64-encoded because this is JSON
and a picture is not text; the alternative is a second content type for one
route, which costs every client more than it saves.
*/
type ScreenRequest struct {
	// Image is a base64 GIF. Still pictures are not retained by the firmware,
	// so one frame of a GIF is how a static image is shown.
	Image string `json:"image,omitempty"`
	// Readout hands the panel back to the cooler's own display.
	Readout bool `json:"readout,omitempty"`
	// Dashboard gives the screen back to hotaru's dashboard.
	Dashboard bool `json:"dashboard,omitempty"`
	// Brightness is 0-100, Orientation one of 0, 90, 180, 270. The device
	// keeps both across restarts.
	Brightness  *int `json:"brightness,omitempty"`
	Orientation *int `json:"orientation,omitempty"`
}

/*
Preview is a draft somebody is holding on some devices.

A preview is what a person is looking at, never what they want, so it is
reported apart from desired state everywhere it appears.
*/
type Preview struct {
	// Token is what the holder renews and releases with.
	Token string `json:"token"`
	// Scene is the name being previewed, where it is a saved one.
	Scene string `json:"scene,omitempty"`
	// Holder is who has it, in a form a person can read.
	Holder string `json:"holder,omitempty"`
	// Devices are the devices it covers.
	Devices []string `json:"devices,omitempty"`
	// Expires is when it lapses unless renewed. Absent when the lease is
	// bound to a connection instead, which is the better signal where the
	// client can hold one.
	Expires *time.Time `json:"expires,omitempty"`
}

// Scene is a named lighting state, as a client sees it.
type Scene struct {
	Name string `json:"name"`
	// Colour is one colour across every device in scope, applied before any
	// assignments. What the nine shipped scenes are.
	Colour string `json:"colour,omitempty"`
	// Off turns lighting off rather than colouring it. Off is not a colour:
	// it resolves the device's own Off mode and honours the correction for a
	// keyboard that treats black as a dead backlight.
	Off bool `json:"off,omitempty"`
	// Shipped marks one of the nine hotaru carries in code. Saving a scene
	// under the same name replaces it; deleting that one brings it back.
	Shipped     bool              `json:"shipped,omitempty"`
	Assignments []SceneAssignment `json:"assignments,omitempty"`
	// Effects name what each device should be doing, by any part of its name.
	Effects map[string]string `json:"effects,omitempty"`
	// Screen is "dashboard", "readout", or a path to a GIF. Empty means the
	// scene says nothing about the screen and the screen does not change.
	Screen string `json:"screen,omitempty"`
}

// Screen states a scene can name, beyond a path to an image.
const (
	ScreenDashboard = "dashboard"
	ScreenReadout   = "readout"
)

// CaptureRequest saves what the lights are showing now as a named scene.
type CaptureRequest struct {
	// Screen is what the scene should say about the cooler's panel. Empty
	// means the scene says nothing and applying it leaves the screen alone.
	Screen string `json:"screen,omitempty"`
}

// SceneAssignment is one colour on one target, in the form somebody types.
type SceneAssignment struct {
	Target string `json:"target"`
	Colour string `json:"colour"`
}

// ScenesResponse is every scene, shipped ones included.
type ScenesResponse struct {
	Scenes []Scene `json:"scenes"`
}

/*
Binding is a key and the scene it applies.

The key is KDE's own spelling, as it appears in kglobalshortcutsrc, because
that string is what gets registered: translating it here would be one more
place for a key to be claimed and then do nothing.
*/
type Binding struct {
	Key   string `json:"key"`
	Scene string `json:"scene"`
	// Missing marks a binding whose scene is not there, which is a key that
	// will report rather than light something when it is pressed.
	Missing bool `json:"missing,omitempty"`
}

// KeysResponse is the shortcuts, what they do, and what is in the way.
type KeysResponse struct {
	Bindings []Binding `json:"bindings"`
	// Reserved are sequences hotaru deliberately leaves free, for scenes
	// somebody writes themselves.
	Reserved []string `json:"reserved,omitempty"`
	// Desktop says whether the KWin integration is installed, and why not.
	Desktop string `json:"desktop,omitempty"`
	// Claimed are shortcuts another program still holds, which is how this
	// class of bug presents: the registration succeeds and the key does
	// nothing.
	Claimed []string `json:"claimed,omitempty"`
}

// ReleasedResponse says how many stale claims were removed.
type ReleasedResponse struct {
	Removed int `json:"removed"`
}

// BindRequest points a key at a scene. An empty scene unbinds it.
type BindRequest struct {
	Key   string `json:"key"`
	Scene string `json:"scene,omitempty"`
}

/*
SceneRequest applies or previews a scene.

Preview and Hold are separate because they answer different questions: whether
this is a draft, and whether the caller can keep a connection open to hold it.
A GUI sets both; a shell script sets Preview alone and renews.
*/
type SceneRequest struct {
	// Preview writes the scene without meaning it: not recorded, not
	// reconciled over, and held under a lease that ends with its holder.
	Preview bool `json:"preview,omitempty"`
	// Hold says the caller is sitting on this request and the preview should
	// end when the connection does. Ignored unless Preview is set.
	Hold bool `json:"hold,omitempty"`
	// Holder is who to name in a listing. Optional; the route fills in
	// something honest when it is empty.
	Holder string `json:"holder,omitempty"`
}

/*
SceneResponse is what a scene did.

Per device, because "the scene worked" says nothing useful about six devices
when one of them is dark. Problems are the scene's own -- a line that would not
parse, a GIF that is not there -- and a scene that cannot be fully applied
applies the rest and reports them.
*/
type SceneResponse struct {
	Scene    string   `json:"scene"`
	Results  []Result `json:"results,omitempty"`
	Problems []string `json:"problems,omitempty"`
	// Screen is what the panel was set to, absent when the scene left it be.
	Screen string `json:"screen,omitempty"`
	// Preview is the lease, when this was a preview.
	Preview *Preview `json:"preview,omitempty"`
}

// PreviewRequest renews or releases a lease by token.
type PreviewRequest struct {
	Token string `json:"token"`
}
