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

## Status codes

| Code | When |
|---|---|
| 200 | the request was understood, whatever the devices did with it |
| 400 | the request does not decode, or names a colour or target that does not parse. Nothing is written |
| 503 | no OpenRGB server. The request was fine; the machine is not ready |

A 503 is deliberately not a 500: the client asked for something reasonable and
the answer is about the machine, not about what was asked.
