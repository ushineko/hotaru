# The API

hotaru's service speaks HTTP and JSON over a Unix socket at
`$XDG_RUNTIME_DIR/hotaru/hotaru.sock`. There is no TCP listener: the socket's
permissions (`0600`) are the whole authentication story, and adding a port would
mean inventing an authentication scheme for a program that changes the colour of
lights.

**This is an interface, not a convenience.** The CLI is its first client, the
GUI is its second, and a future Go rewrite of the battery monitor is meant to be
its third. Changing a field here is a breaking change.

Everything on the wire is something a person could have typed — targets like
`kraken/fan-top`, colours like `#ff8800` or `red`. Parsing happens on the
service's side, so a client needs no knowledge of devices at all.

The transcripts below are recorded, not written by hand: the reads come from a
live service on the development machine, the write from the test suite against
the in-memory server. Each one established a behaviour before there was any
consumer to depend on it, which is the point of recording them.

## GET /v1/health

Why nothing is happening, in a sentence meant for a person. Four states with
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

`health` answers even when nothing else can, which is why it is its own route:
with no OpenRGB server, `/v1/devices` is a 503 and this still tells you what to
do about it.

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

Every device the server knows, with what it is showing and whether hotaru would
drive it. `in_scope` is here so a listing answers "why did nothing happen to
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
      "in_scope": true
    }
  ]
}
```

Zones are contiguous runs in device LED order; `first` is counted rather than
reported, because the protocol gives sizes and leaves offsets implicit. A
segment named in the rules file appears in `segments`, and a device whose colour
is re-asserted carries the interval in `reassert`.

## POST /v1/lighting/apply

`colour` is everything in scope; `assignments` are the exceptions; `off` turns
devices off rather than colouring them black, which is a different thing on
hardware with a backlight. `devices` narrows to particular hardware by name.

`preview` writes without remembering. What it lights is not what the machine
restores at boot, which is how the mapping wizard can flash colours at somebody
without the answers becoming their configuration. `hotaru light set --preview`
is the same thing from the command line.

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

That response is the design in miniature. The first attempt was **accepted and
not honoured**: the server took the write, and reading the device back found it
still in Rainbow Wave. Nothing reported an error; only the read-back noticed.
hotaru moved to the next candidate, confirmed it, and recorded both attempts so
a user can learn what their hardware does.

`changed` is how many devices actually changed, so a client exits non-zero
without counting results itself. **A request that changed nothing does not
report success** — three devices skipped for three good reasons is still a scene
that lit nothing.

Each result is one of three things, kept apart deliberately:

| Field | Means |
|---|---|
| `applied` with `mode` | written, and confirmed by reading the device back |
| `skipped` | the device cannot express this, and why. Not a failure, not worth retrying |
| `error` | the server or the hardware went wrong |

## GET /v1/status

What the service *is*, rather than what it can see. `health` answers "why is
nothing happening"; this answers "what is running, and what does it remember".

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

`remembered` lists the devices hotaru would put back. Its absence above is a
fresh install that has been asked for nothing — which is why it restores
nothing, and why installing hotaru cannot disturb lighting configured elsewhere.

## GET /v1/scenes

Every saved scene. A scene is colour assignments addressed at whatever depth
somebody meant them, an effect per device, and what the cooler's screen shows.

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
      "effects": { "keychron": "Solid Splash" },
      "screen": "dashboard"
    }
  ]
}
```

`screen` is `dashboard`, `readout`, or a path to a GIF. **Absent means the
scene says nothing about the screen and applying it changes nothing about it**
-- a lighting scene must not take somebody's dashboard away because its author
never thought about the panel.

`effects` names a mode the device advertises, keyed by any part of the device's
name. It is preferred, not forced: a mode that cannot carry the frame falls
through as it always does, and a name the device does not have costs the effect
rather than the scene and is reported in that device's `problems`.

## PUT /v1/scenes/{name}

Writes a scene under a name, replacing one already there. The body is a scene
without its name, which comes from the path.

## DELETE /v1/scenes/{name}

Removes one. Deleting a scene that is not there is not an error.

## POST /v1/scenes/{name}/capture

Saves what the lights are showing now, under a name, from desired state. The
body may carry `screen`, which is what the saved scene should say about the
panel.

## POST /v1/scenes/{name}/apply

Lights a scene. Recorded as what the machine should be showing, so it survives
a reboot and is re-sent to hardware that forgets.

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

`{"preview": true}` lights the scene **without meaning it**: nothing is
recorded, and re-assertion is suspended for the devices it covers so that the
timer which exists to correct hardware that forgets does not correct the person
looking at a draft instead.

A preview is a lease, and it ends when its holder does. Two ways, and a client
uses whichever it already has:

- `{"preview": true, "hold": true}` -- the response is sent as soon as the
  draft is up and **the request stays open**. The lease is that connection: the
  socket closing is the client going away, reported by the kernel, with no
  clock involved. This is what `hotaru scene preview` does, and killing it with
  `kill -9` puts the lights back.
- `{"preview": true}` alone -- the lease carries an expiry the holder renews.
  For a client that cannot sit on a connection.

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

Devices showing a draft carry it in `GET /v1/devices`, because a device whose
re-assertion is suspended otherwise looks exactly like one that is behaving:

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

## POST /v1/preview/renew

Pushes a lease's expiry out. `{"token": "…"}`. A lease that has already lapsed
is not revived -- the devices may belong to somebody else by now.

## POST /v1/preview/release

Ends a preview and puts the lights back to what was last applied, answering
with the same shape as `/v1/reconcile`. `{"token": "…"}`.

## POST /v1/reconcile

Puts the lights back to what was last asked for. Not an apply: nothing here is
a new user choice, so desired state is unchanged and a re-assert cannot be
mistaken for an instruction.

```console
$ curl -s --unix-socket … -X POST http://hotaru/v1/reconcile
{
  "applied": 0,
  "complete": true
}
```

`complete: false` with `missing` naming devices is an **unfinished restore**,
not a failure: OpenRGB enumerates once at server start, and a cold boot has been
seen finding two devices of six. It retries, and completes when they appear.

## POST /v1/reload

Re-reads the rules file, reporting what was wrong with it entry by entry rather
than refusing the file.

```console
$ curl -s --unix-socket … -X POST http://hotaru/v1/reload
{
  "rules_file": "/home/you/.config/hotaru/hotaru.yml"
}
```

## POST /v1/lighting/probe

Finds out what each device can actually do: sets modes, reads back which ones
the device honoured, and puts everything back. A POST because it writes.

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

Two things that transcript shows. The suggestion is a **comment**: black is off
for a mousepad and a dead backlight for a keyboard, and nothing in the protocol
says which this is — so the probe reports what it found and leaves the decision
to someone who can see the machine.

And what probing cannot do. Run against an ASUS board, it reports that Static
"took", because the mode change does take — the addressable headers going dark
is invisible to a read-back. A user with only the probe would never be offered
the direct-first rule. That is the gap the mapping wizard fills: it asks a
person to look.

## Status codes

| Code | When |
|---|---|
| 200 | the request was understood, whatever the devices did with it |
| 400 | the request does not decode, or names a colour or target that does not parse. Nothing is written |
| 503 | no OpenRGB server. The request was fine; the machine is not ready |

A 503 is deliberately not a 500: the client asked for something reasonable and
the answer is about the machine, not about what was asked.
