# The API

hotaru's service serves HTTP and JSON over a Unix socket at
`$XDG_RUNTIME_DIR/hotaru/hotaru.sock`. It opens no TCP listener. The socket's
`0600` permissions are the whole of the authentication, and adding a port
would mean inventing an authentication scheme for a program that changes the
colour of lights.

**This is an interface, not a convenience.** The CLI is its first client, the
window is its second, and a future Go rewrite of the battery monitor is
intended as its third. Changing a field here breaks those clients.

Everything on the wire is something a person could have typed: targets such as
`kraken/fan-top`, and colours such as `#ff8800` or `red`. The service parses
them, so a client needs no knowledge of devices at all.

Somebody recorded the transcripts below rather than writing them by hand. The
reads come from a live service on the development machine, and the write comes
from the test suite against the in-memory server. Each one fixed a behaviour
before any consumer depended on it, which is why they are recorded here.

## GET /v1/health

Why nothing is happening, in a sentence written for a person. Four states with
four remedies: `unreachable`, `no-devices`, `none-in-scope`, `healthy`.

```console
$ curl -s --unix-socket $XDG_RUNTIME_DIR/hotaru/hotaru.sock http://hotaru/v1/health
{
  "state": "healthy",
  "detail": "6 of 6 devices are in scope",
  "address": "127.0.0.1:6742",
  "protocol": 3,
  "devices": 6,
  "in_scope": 6,
  "version": "0.0.0"
}
```

`health` answers when nothing else can, which is why it is a route of its own.
With no OpenRGB server, `/v1/devices` returns a 503, and this route still
tells you what to do about it.

```json
{
  "state": "unreachable",
  "detail": "no OpenRGB server at 127.0.0.1:6742. Start it, and lighting works from then on.",
  "address": "127.0.0.1:6742",
  "devices": 0,
  "in_scope": 0,
  "version": "0.0.0"
}
```

## GET /v1/devices

Every device the server knows, with what it shows and whether hotaru drives
it. `in_scope` is here so that one listing answers "why did nothing happen to
this one?" without a second call.

```console
$ curl -s --unix-socket $XDG_RUNTIME_DIR/hotaru/hotaru.sock http://hotaru/v1/devices
{
  "devices": [
    {
      "name": "MSI GeForce RTX 4090 Suprim Liquid X",
      "leds": 1,
      "modes": ["Off", "Direct", "Rainbow Wave", "Magic", "Color Cycle", "..."],
      "active_mode": "Direct",
      "zones": [
        { "name": "GPU", "first": 0, "count": 1 }
      ],
      "colours": ["#ff5500"],
      "one_colour": ["Static"],
      "coloured": {
        "Breathing": { "least": 1, "most": 2 },
        "Color Cycle": { "least": 1, "most": 3 }
      },
      "paced": { "Rainbow Wave": { "slowest": 0, "fastest": 255, "now": 127 } },
      "in_scope": true
    }
  ]
}
```

`one_colour` and `paced` are asked of the mode rather than of the device,
exactly as `dimmable` is: a keyboard paints a hundred keys in Direct and shows
one colour in a reactive mode, and it is the same keyboard. A client drawing an
editor needs both before it draws, so that a colour per light is offered where
the device can show one and not where it cannot. `paced` gives each mode's own
range: the numbers come from the mode and go back to it, and `slowest` may be
the larger of the two on a driver that counts down.

**`coloured` is how many colours of its own each mode takes** (spec 061).
`least` is how many the mode needs and `most` is how many it accepts. A mode
that takes a colour per light is not listed, because its colour is the frame.
The numbers come from OpenRGB's `colors_min` and `colors_max` for a firmware
mode. A mode that keeps a colour and reports no count takes one. An editor
draws one colour picker per slot from this. `one_colour` still says which
modes show one colour of their own in place of the frame.

A zone is a contiguous run in device LED order. hotaru counts `first` rather
than reading it, because the protocol gives sizes and leaves the offsets
implicit. A segment named in the rules file appears in `segments`. A device
whose colour hotaru rewrites on a timer carries the interval in `reassert`.

**A canvas device** is a device hotaru draws itself, because it has no
effects of its own (spec 060). It carries `canvas`, which says what the
drawing is doing, and `led_names`, its own names for its keys. Its `modes`
are hotaru's renderers. With `drawing` false the device is holding the one
frame it was sent. The OpenRGB listing of the same hardware carries
`handed_to`, and hotaru writes nothing to it. This shape comes from the test
suite; no live transcript is recorded yet.

```json
{
  "name": "SteelSeries Apex Pro TKL Wireless Gen 3",
  "leds": 85,
  "modes": ["Static", "Breathing", "Spectrum", "Rainbow Wave", "Off"],
  "active_mode": "Rainbow Wave",
  "zones": [{ "name": "Keys", "shape": "grid", "first": 0, "count": 85 }],
  "one_colour": ["Breathing"],
  "coloured": {
    "Breathing": { "least": 0, "most": 4 },
    "Spectrum": { "least": 0, "most": 8 },
    "Rainbow Wave": { "least": 0, "most": 8 }
  },
  "paced": {
    "Breathing": { "slowest": 0, "fastest": 100, "now": 50 },
    "Spectrum": { "slowest": 0, "fastest": 100, "now": 50 },
    "Rainbow Wave": { "slowest": 0, "fastest": 100, "now": 50 }
  },
  "led_names": ["A", "B", "C", "..."],
  "canvas": { "device": "SteelSeries Apex Pro TKL Wireless Gen 3", "effect": "Rainbow Wave",
              "drawing": true, "interval": "56ms", "rate": 17.8, "frames": 1068 },
  "in_scope": true
}
```

`/v1/health` lists the same `canvas` object for every canvas device, under
`canvases`.

A renderer needs no colours of its own. Breathing breathes the frame's
colour without them, and one breath per colour in turn with several.
Spectrum goes round the colour wheel without them, and round the colours
given with them. Rainbow Wave is the full rainbow without them, and a moving
gradient through the colours given with them.

## POST /v1/lighting/apply

`colour` sets everything in scope. `assignments` are the exceptions. `off`
turns devices off rather than colouring them black, which is a different thing
on hardware with a backlight. `devices` narrows the request to named hardware.

`preview` writes without remembering. What it lights is not what the machine
restores at boot. That is how the mapping wizard flashes colours at somebody
without their answers becoming their configuration. `hotaru light set
--preview` does the same thing from the command line.

```console
$ curl -s --unix-socket $XDG_RUNTIME_DIR/hotaru/hotaru.sock \
    -H 'Content-Type: application/json' \
    -d '{"colour":"blue","assignments":[{"target":"ASUS/Addressable 1[0:0]","colour":"red"}]}' \
    http://hotaru/v1/lighting/apply
{
  "results": [
    {
      "device": "ASUS ROG MAXIMUS Z790 HERO",
      "applied": true,
      "mode": "Direct",
      "attempts": [
        {
          "mode": "Static",
          "accepted": true,
          "active": "Rainbow Wave"
        },
        {
          "mode": "Direct",
          "accepted": true,
          "active": "Direct"
        }
      ]
    }
  ],
  "changed": 1
}
```

That response is the design in miniature. The device **accepted the first
attempt and did not honour it**. The server took the write, and reading the
device back found it still in Rainbow Wave. Nothing reported an error, and
only the read-back noticed. hotaru moved to the next candidate, confirmed it,
and recorded both attempts, so that a user learns what their hardware does.

`changed` counts the devices that changed, so a client exits non-zero without
counting the results itself. **A request that changed nothing does not report
success.** Three devices skipped for three good reasons is still a scene that
lit nothing.

Each result is one of three things, and they are kept apart deliberately:

| Field | Means |
|---|---|
| `applied` with `mode` | written, and confirmed by reading the device back |
| `skipped` | the device cannot express this, and why. Not a failure, not worth retrying |
| `handed_to` | with `skipped`: the canvas device that draws this hardware. The same device listed twice, so a count leaves it out |
| `error` | the server or the hardware went wrong |

## GET /v1/status

What the service *is*, rather than what it can see. `health` answers "why is
nothing happening". This route answers "what is running, and what does it
remember".

```console
$ curl -s --unix-socket $XDG_RUNTIME_DIR/hotaru/hotaru.sock http://hotaru/v1/status
{
  "version": "0.0.0",
  "connected": true,
  "address": "127.0.0.1:6742",
  "protocol": 3,
  "rules_file": "/home/you/.config/hotaru/hotaru.yml"
}
```

`remembered` lists the devices hotaru puts back. It is absent above because
that is a fresh install that nobody has asked for anything. Such an install
restores nothing, which is why installing hotaru cannot disturb lighting that
was configured elsewhere.

## GET /v1/scenes

Every saved scene. A scene holds colour assignments addressed at whatever
depth somebody meant them, an effect per device, and what the cooler's screen
shows.

```console
$ curl -s --unix-socket … http://hotaru/v1/scenes
{
  "scenes": [
    {
      "name": "evening",
      "assignments": [
        { "target": "kraken",   "colour": "#201040" },
        { "target": "keychron", "colour": "#100820" }
      ],
      "effects": {
        "keychron": {
          "mode": "Solid Reactive",
          "colour": "#0000ff",
          "speed": 127
        },
        "graphics": {
          "mode": "Breathing",
          "colour": "#ff2000",
          "colours": ["#ff2000", "#2040ff"],
          "colours_from": "picture"
        },
        "kraken": "Breathing"
      },
      "screen": "dashboard"
    }
  ]
}
```

`screen` is `dashboard`, `readout`, or a path to a GIF. **When it is absent,
the scene says nothing about the screen, and applying it changes nothing about
the screen.** A lighting scene must not remove somebody's dashboard because
its author never considered the panel.

`effects` names a mode the device advertises, keyed by any part of the
device's name. **Either a mode's name on its own, or an object naming the mode
with the colour and speed to run it at** -- both forms are read, and the short
one is written wherever nothing else was set.

An effect is written even when the scene gives that device more colours than
the mode can show: the mode is the thing being asked for, and the frame is what
gives way. Such a mode is lit in `colour` where the scene names one and in the
colour most of the frame is otherwise. `speed` is in the device's own units,
within the range `paced` gives for that mode, and is ignored by a mode that
advertises no speed.

**An effect with several colours lists them in `colours`**, first to last
(spec 061). The first is also written as `colour`, so an older hotaru runs
the effect in its first colour. One colour is written as `colour` alone, as
before. The colours fill the mode's slots in order, up to `most` in
`coloured`. Fewer than `least` repeat the last one. One colour fills every
slot the mode has, so a mode that takes two shows that colour twice.

`colours_from: picture` marks colours that hotaru picked from the picture or
dashboard the scene was made from. `POST /v1/scenes/{name}/recolour` picks
those again. Colours without the mark are somebody's choice, and a recolour
keeps them.

A name the device does not have costs the effect rather than the scene, and
appears in that device's `problems`, as does a colour that will not parse.

## PUT /v1/scenes/{name}

Writes a scene under a name, replacing one already there. The body is a scene
without its name, which comes from the path.

## DELETE /v1/scenes/{name}

Removes one. Deleting a scene that is not there is not an error.

## POST /v1/scenes/{name}/rename

Renames a scene, and repoints every key bound to it. The body is
`{"to": "dusk"}`.

A rename is a rename plus every reference to the old name. A key names the
scene it fires, and the script installed into the desktop carries the name
rather than the key, so a rename that left the binding behind would leave a
keypress firing a scene that is not there.

The reply says what else changed:

```console
$ curl -s --unix-socket … -X POST -d '{"to":"dusk"}' \
    http://hotaru/v1/scenes/evening/rename
{
  "from": "evening",
  "to": "dusk",
  "keys": 1,
  "changed": "1 key updated"
}
```

A shipped scene cannot be renamed: its name is in the binary, and there is
nothing in a file to move. Renaming onto a name that is taken is refused, and
nothing is written.

## POST /v1/scenes/{name}/capture

Saves what the lights are showing now, under a name, from desired state. The
body may carry `screen`, which is what the saved scene should say about the
panel.

## POST /v1/scenes/{name}/apply

Lights a scene. hotaru records it as what the machine should show, so it
survives a reboot and hotaru writes it again to hardware that forgets.

```console
$ curl -s --unix-socket … -X POST http://hotaru/v1/scenes/evening/apply
{
  "scene": "evening",
  "results": [
    {
      "device": "NZXT Kraken 2024 ELITE Series RGB",
      "applied": true,
      "mode": "Direct",
      "attempts": [ { "mode": "Direct", "accepted": true, "active": "Direct", "showing": true } ]
    },
    {
      "device": "Keychron K4 HE",
      "applied": true,
      "mode": "Solid Splash",
      "attempts": [ { "mode": "Solid Splash", "accepted": true, "active": "Solid Splash", "showing": true } ]
    }
  ],
  "screen": "dashboard"
}
```

### Previewing

`{"preview": true}` lights the scene **without meaning it**. hotaru records
nothing, and suspends its rewrite timer for the devices the scene covers. That
timer exists to correct hardware that forgets, and it must not correct the
person looking at a draft.

A preview is a lease, and it ends when its holder does. There are two ways to
hold one, and a client uses whichever it already has:

- `{"preview": true, "hold": true}` sends the response as soon as the draft is
  up and **keeps the request open**. The lease is that connection. The kernel
  reports the socket closing, which is the client going away, and no clock is
  involved. `hotaru scene preview` works this way, so `kill -9` on it puts the
  lights back.
- `{"preview": true}` alone gives the lease an expiry that the holder renews.
  This is for a client that cannot sit on a connection.

```console
$ curl -s --unix-socket … -X POST -d '{"preview":true,"holder":"a shell"}' \
    http://hotaru/v1/scenes/loud/apply
{
  "scene": "loud",
  "results": [ … ],
  "preview": {
    "token": "3d87ebe9ee7025f8",
    "scene": "loud",
    "holder": "a shell",
    "devices": ["NZXT Kraken 2024 ELITE Series RGB", "Keychron K4 HE"],
    "expires": "2026-09-20T18:39:00.918882713-07:00"
  }
}
```

No `expires` means the lease is bound to a connection instead.

A device showing a draft carries that draft in `GET /v1/devices`. Without it,
a device whose rewrite timer is suspended looks exactly like one that is
behaving:

```console
$ curl -s --unix-socket … http://hotaru/v1/devices
{
  "devices": [
    {
      "name": "Keychron K4 HE",
      "in_scope": true,
      "preview": { "token": "3d87ebe9ee7025f8", "scene": "loud", "holder": "a shell", … }
    }
  ]
}
```

## POST /v1/preview

Previews a scene **that has no name**. The scene travels in the body instead.

This is the editor's route. A draft in a window is in nobody's scene file, and
saving it there in order to look at it would put a half-finished thing in
somebody's list. "Saved" would then mean nothing.

```console
$ curl -s --unix-socket … -X POST http://hotaru/v1/preview \
    -d '{"scene":{"assignments":[{"target":"kraken","colour":"#201040"}]},"holder":"an editor"}'
```

With `token`, this changes the draft a preview is already showing rather than
taking a second lease. An editor takes the lease once and then says what the
draft is now on every change, because taking it again would release it first
and flash the previous colours back. Devices the edited scene has started
naming are taken into the lease; one held by somebody else is refused, exactly
as a new lease is.

It is the same route because it is the same sentence -- this scene is what the
preview shows -- and a scene becoming lights by a second route is how a
scene's effects get left behind. `POST /v1/lighting/apply` carries colours and
not effects, and is for `hotaru light set` and the wizard rather than for
scenes.

`hold` behaves as it does on the named route. With it, the request stays open
and the lease is that connection. Without it, the lease carries an expiry to
renew. Everything else is the same machinery: the suspended rewrite timer, the
revert, and one preview per device.

## POST /v1/preview/renew

Pushes a lease's expiry out. `{"token": "…"}`. hotaru does not revive a lease
that has already lapsed, because the devices may belong to somebody else by
now.

## POST /v1/preview/release

Ends a preview and puts the lights back to what was last applied, answering
with the same shape as `/v1/reconcile`. `{"token": "…"}`.

## GET /v1/dashboards

Everything the panel can draw, which one it draws now, and what an editor
offers. That means the arrangements, with the number of readings and rings
each has room for, and the themes by name. The lists come from here so that a
window carries no copy of its own and cannot show eight of nine after somebody
adds a sensor.

## PUT /v1/dashboards/{name}

Writes one, replacing any of the same name. Saving over a name hotaru ships
replaces it for as long as the saved one exists.

Each slot holds a `source`, an optional `second`, an optional `separator` and
an optional `label`. The headline and the smaller readings take the same
shape:

```json
{"source": "cpu_pct", "second": "cpu_c", "separator": " / ", "label": "CPU % / °C"}
```

A slot with a `second` draws both numbers, joined by the separator, which
defaults to `" / "`. The colour and the headline's ring grade on `source`, so
the order is the author's choice about what the colour means. An empty `label`
means the readings' own words, carrying what they are measured in: `CPU °C`
for one reading, and `CPU % / °C` for that pair.

There is no `unit` field. The label is the only text drawn. A saved dashboard
that carries a `unit` keeps it in the file, and hotaru ignores it.

## DELETE /v1/dashboards/{name}

Forgets one. Deleting an override of a shipped name brings the shipped one
back.

## POST /v1/dashboards/{name}/use

Makes it the dashboard the panel draws, and answers with it.

## POST /v1/dashboards/{name}/rename

Renames a dashboard, rewrites every scene naming it, and follows the panel
when it was the one being drawn. The body is `{"to": "ours"}`, and the reply
is the same shape as a scene rename.

A scene spells a particular dashboard as `dashboard:<name>`. That is the
reference a rename has to carry, or the scene puts up a dashboard that is no
longer there.

## POST /v1/dashboards/{name}/clone

Copies a dashboard under another name, and answers with the copy. The body is
`{"to": "mine"}`.

The shipped ones can be cloned, which is how somebody starts from one. The
copy is in the file and is not shipped, so it can be edited, renamed and
deleted. It is not put on the panel: copying a dashboard is not asking to see
it.

## POST /v1/dashboards/{name}/preview

A rendered frame, base64-encoded, with its size and the seconds the panel
needs between frames of that size. With a body, it draws that unsaved edit
instead of the stored dashboard, which is what an editor with a preview in it
needs. With no body, it draws the stored one.

The service renders the frame rather than the client. The panel takes a
640x640 GIF, and this route makes them. A second renderer would be a second
answer about what the screen shows.

## GET /v1/readings

Every number this machine can put on the panel, each with its label, its unit
and whether the machine had it to give.

```console
$ curl -s --unix-socket … http://hotaru/v1/readings
{
  "readings": [
    {"source": "coolant",  "label": "Coolant", "unit": "°C",  "value": 38.5, "known": true, "text": "38.5"},
    {"source": "cpu_pct",  "label": "CPU",     "unit": "%",   "value": 3,    "known": true, "text": "3"},
    {"source": "mem_pct",  "label": "Memory",  "unit": "%",   "value": 43.7, "known": true, "text": "44"},
    {"source": "mem_gb",   "label": "Memory",  "unit": "GB",  "value": 13.6, "known": true, "text": "14"},
    {"source": "gpu_pct",  "label": "GPU",     "unit": "%",   "known": false, "text": "--"}
  ]
}
```

`known` is the field to read. A sensor that has gone away is absent rather
than zero, because a pump drawn at 0 RPM is the most alarming number this
machine can show, and it would rest on no evidence. A client shows `text`,
which already reads `--` in that case.

Utilisation is a rate. It is the share of the interval since the last time
anything asked, which between dashboard ticks is about two seconds.

## GET /v1/images

The pictures stored for the cooler's screen, already converted.

```console
$ curl -s --unix-socket … http://hotaru/v1/images
{
  "images": [
    {
      "name": "wallpaper",
      "path": "/home/you/.local/share/hotaru/images/wallpaper.gif",
      "bytes": 202752,
      "frames": 1,
      "added": "2026-09-20T22:53:41-07:00"
    }
  ]
}
```

Read `bytes`. The panel's refresh floor scales with frame size rather than
being a fixed rate limit, so a large picture is a slow one.

## PUT /v1/images/{name}

Converts a picture and keeps it. `{"image": "<base64>"}`, as any JPEG, PNG or
GIF. hotaru crops it to the middle, scales it to 640x640, and reduces it to
256 colours chosen from the picture itself. A name that already exists is
replaced.

`{"images": ["<base64>", …]}` makes a slideshow out of several instead. Each
picture holds, crossfades into the next, and the last one fades back into the
first. hotaru shortens the fade until the reel fits the panel's memory,
because a slideshow missing a photograph is not the one somebody asked for.

## POST /v1/images/preview

The same conversion, returned rather than kept:
`{"image": "<base64>", "bytes": 202752, "frames": 1}`.

What a client shows somebody before they decide. A wallpaper is wide and the
panel is square, so the cooler receives the middle of the picture. Only the
person looking can say whether that is still the picture they wanted, and they
answer it in front of the result.

## DELETE /v1/images/{name}

Forgets one.

## POST /v1/images/{name}/rename

Renames a stored picture, and everything that names it. The body is
`{"to": "aurora"}`, and the reply is the same shape as a scene rename.

Two references move with it. A scene keeps the path, because it is what the
panel is handed; a dashboard keeps the name, because a background is chosen
from the library. A rename that moved only the file would show up later as a
scene with no picture and a dashboard with a plain colour where a photograph
was.

The name is cleaned before it becomes a filename, so the reply carries the
name as stored.

## POST /v1/images/{name}/scene

Builds a scene whose lights match a stored picture, and keeps it.
`{"scene": "jovian"}`; the scene comes back in the reply.

Every zone gets a run across the picture rather than one colour for the whole
machine. Light *i* of *n* takes the *i*th vertical slice, so a ring carries
the image's own left-to-right sweep. hotaru weights each slice by how much
colour its pixels carry. Half of every slice through a photograph is
background, and averaging that in reads a rust planet as grey-brown. hotaru
then lifts the value to something a light can show, because a photograph is
mostly shadow. The scene names the picture as its screen, so applying it makes
the whole machine agree.

**An effect that takes colours, and was sent with none, gets the picture's.**
`effects` in the body names what each device does, as in a scene. hotaru
gives such an effect the picture's most prominent distinct colours. It gives
as many as the mode takes, four at most, and the colour that covers most of
the picture comes first. A pixel counts by how much colour it has, so a dark
background does not count as a colour. `distance` decides how far apart two
colours must be to count as two. Those colours carry `colours_from:
picture`. `POST /v1/dashboards/{name}/scene` does the same with the frame the
dashboard draws.

## POST /v1/images/{name}/colours

The colours a scene made from the picture would give an effect, without
making a scene. The window shows them in Make a scene before the scene
exists. `count` is how many the mode takes, and `distance` is the scene's.

```console
$ curl -s --unix-socket … -X POST http://hotaru/v1/images/sunset/colours \
    -d '{"count": 2, "distance": 1}'
{ "colours": ["#f83000", "#2040f8"] }
```

Fewer colours than `count` is a picture with fewer distinct colours. An
empty list is a picture with nothing to pick. `POST
/v1/dashboards/{name}/colours` does the same for a dashboard. The CLI reaches
both as `hotaru image colours` and `hotaru dashboard colours`.

## POST /v1/images/{name}/show

Puts a stored picture on the panel, taking it from the dashboard.
`POST /v1/screen` with `{"dashboard": true}` gives it back.

## GET /v1/keys

The shortcuts, what they apply, and what is in the way. The third part is why
this is one call rather than three. A key bound to a scene that no longer
exists, and a key another program still claims, both look exactly like a
working binding from anywhere else.

```console
$ curl -s --unix-socket … http://hotaru/v1/keys
{
  "bindings": [
    { "key": "Ctrl+Alt+Num+1", "scene": "red" },
    { "key": "Ctrl+Alt+Num+9", "scene": "off" }
  ],
  "reserved": ["Ctrl+Alt+Shift+Num+1", "…", "Ctrl+Alt+Shift+Num+9"],
  "claimed": ["AIOScene11 holds Ctrl+Alt+Shift+Num+1 (in kwin)"]
}
```

hotaru reads `claimed` out of `~/.config/kglobalshortcutsrc`, and never writes
that file. KDE keeps one entry per registered shortcut, and **those entries
outlive the program that made them**. While such an entry is there, hotaru's
own registration succeeds and the key does nothing at all. hotaru checks the
`reserved` sequences as well as the bound ones, because that is where
somebody's own scenes go next.

`desktop`, when present, says why the KWin integration is not running: no
session bus, no KWin yet, or another hotaru holding the bus name.

## POST /v1/keys/bind

`{"key": "Ctrl+Alt+Shift+Num+1", "scene": "evening"}`. An empty scene name
unbinds the key, including one of the nine shipped ones.

No route clears a claim. The service reads the desktop's file and cannot write
it, because its unit gives it write access to its own two directories and
nothing else. `hotaru keys release` therefore edits in the caller's own
process, after showing them what is in the way. The window does the same.

## The hotkey door

This door is not HTTP. A KWin script reaches the outside world only through
`callDBus`, so hotaru puts one D-Bus object with exactly one method on the
session bus:

	org.ushineko.hotaru  /Scenes  org.ushineko.hotaru.Scenes.Apply(scene)

It hands the name to the same service call that `POST
/v1/scenes/{name}/apply` makes. One flow, two doors, and the narrow door
exists because KWin offers no other.

## POST /v1/reconcile

Puts the lights back to what was asked for last. This is not an apply. Nothing
here is a new user choice, so desired state does not change, and nobody can
mistake a rewrite for an instruction.

```console
$ curl -s --unix-socket … -X POST http://hotaru/v1/reconcile
{
  "applied": 0,
  "complete": true
}
```

`complete: false`, with `missing` naming devices, is an **unfinished restore**
rather than a failure. OpenRGB enumerates once at server start, and a cold
boot has found two devices of six. hotaru retries, and completes when the rest
appear.

## POST /v1/reload

Reads the rules file again, and reports what is wrong with it entry by entry
rather than refusing the whole file.

```console
$ curl -s --unix-socket … -X POST http://hotaru/v1/reload
{
  "rules_file": "/home/you/.config/hotaru/hotaru.yml"
}
```

## POST /v1/lighting/probe

Finds out what each device can do. It sets modes, reads back which ones the
device honoured, and puts everything back. It is a POST because it writes.

```console
$ curl -s --unix-socket … -X POST -d '{"devices":["mm700"]}' http://hotaru/v1/lighting/probe
{
  "findings": [
    {
      "device": "Corsair MM700",
      "modes": [
        { "name": "Direct", "per_led": true, "tried": true, "took": true }
      ],
      "zones": [
        { "name": "Left",  "first": 0, "count": 1 },
        { "name": "Right", "first": 1, "count": 1 },
        { "name": "Logo",  "first": 2, "count": 1 }
      ],
      "no_off_mode": true,
      "suggested": "# this device has no Off mode, so `off` writes black to it.\n# if that blanks something that should stay lit, add:\n# never_blank: true"
    }
  ]
}
```

That transcript shows two things. First, the suggestion is a **comment**.
Black is off for a mousepad and a dead backlight for a keyboard, and nothing
in the protocol says which device this is. The probe reports what it found and
leaves the decision to somebody who can see the machine.

Second, it shows what probing cannot do. Against an ASUS board it reports that
Static "took", because the mode change does take. A read-back cannot see the
addressable headers go dark. A user with only the probe would never receive
the direct-first rule. The mapping wizard fills that gap by asking a person to
look.

## POST /v1/lighting/release

Hands a canvas device's lighting back to its firmware. hotaru never does this
on its own. When the service stops, the device keeps the last frame it was
sent. On the Apex this reboots the board, which comes back on another hidraw
node showing its onboard effect. hotaru forgets what the device was last
asked to show, attaches it again, and sends it nothing until a scene asks.

```console
$ curl -s --unix-socket … -X POST -d '{"device":"apex"}' http://hotaru/v1/lighting/release
{
  "device": "SteelSeries Apex Pro TKL Wireless Gen 3",
  "detail": "SteelSeries Apex Pro TKL Wireless Gen 3 shows its own lighting again. ..."
}
```

`device` is any part of the name. A name that matches no canvas device is a
404, and the error lists the devices hotaru draws.

## Status codes

| Code | When |
|---|---|
| 200 | the request was understood, whatever the devices did with it |
| 400 | the request does not decode, or names a colour or target that does not parse. Nothing is written |
| 404 | a release names no device that hotaru draws |
| 503 | no OpenRGB server. The request was fine; the machine is not ready |

A 503 is deliberately not a 500. The client asked for something reasonable,
and the answer describes the machine rather than the request.
