# Spec 001: project scope, the migration contract, and the lighting core

**Issue**: [#1](https://github.com/ushineko/hotaru/issues/1)

## Status: COMPLETE

The second-machine test passed on 2026-09-20, on a CachyOS machine with
hardware this one has none of: four identical sticks of DDR5, a motherboard
appearing as two controllers, two empty addressable headers, and a pair of
strips daisy-chained onto an analogue one. Installed, started, mapped by its
owner through the wizard, and driven by the names they chose — with no
configuration written by hand at any point.

It found ten faults, every one of them invisible from here, and the suite
passed at each moment one was discovered. The full list is in
[spec 008](008-the-mapping-wizard.md) and in the commits of that week; the
short version is that four sticks of RAM sharing a name were collapsed into
one device, and hotaru drove a quarter of that machine's memory while
reporting complete success.

Preview and lease moved to spec 004, and walking a zone LED by LED to spec 005.
Both are marked where they were, with the reasoning.

## Context

`peripheral-battery-monitor` (in `ag-scripts`) is a KDE tray widget for
peripheral battery levels. Over specs 017 through 038 it also became the
machine's cooling monitor, its LCD dashboard renderer, its RGB controller and
its global-hotkey host. None of that is about peripheral batteries. It lives
there because that is where the serialising queue was.

hotaru takes all of it: **everything to do with the AIO cooler, its display, and
lighting** — as a rearchitecture rather than a translation; see "Rearchitecture,
not a port" for what that means in practice. The monitor has no RGB support and touches no device, full stop.

The line is **writes**. Every write to the cooler and to every lit device — mode,
colour, brightness, LCD screen, dashboard push — belongs to hotaru and to nothing
else. The monitor keeps *reading* cooler telemetry for its own tray display, and
does so from its own sources for now; when it is rewritten in Go it will pull
that snapshot from hotaru's CLI instead and stop touching the cooler at all.

The name is 蛍 — fireflies, small lights that pulse.

### What hotaru owns

| Area | Currently | Source |
|---|---|---|
| Lighting | OpenRGB across cooler, GPU, motherboard, mousepad, mouse, keyboard | `rgb_openrgb.py` |
| Cooler telemetry | `liquidctl --json status`, OpenLinkHub HTTP for CPU package temp | `aio_reader.py` (ported, not removed — see below) |
| Pump failure alerting | Threshold alert on pump rpm and coolant temperature | spec 020 — stays in the monitor, which is where it is seen; moves at the Go rewrite |
| The Kraken LCD | `liquidctl set screen` — liquid readout, static, GIF, brightness, orientation | `aio_liquid.py` |
| The LCD dashboard | A 640x640 render of the snapshot, pushed as a GIF, gated and kept alive | `aio_dashboard.py`, spec 028 |
| Device serialisation | One hidraw node, many callers | `aio_queue.py` |
| Scenes | Colour plus LCD mode, applied as a unit, bound to numpad keys | `aio_scenes.py` |
| The hotkeys | KWin script, D-Bus service | `scene_shortcuts.py`, `scene_service.py` |

All of it moves, including every shipped scene, which becomes hotaru's defaults
rather than something a user re-creates.

### What the monitor keeps

`battery_reader.py`, `bandwidth_reader.py`, `bandwidth_section.py`,
`accounts.py`, `usage_cache.py`, `usage_shape.py`, `kwin_window_position.py`,
`install_kwin_rule.py`, and the tray shell around them.

It also keeps its **AIO telemetry display, read-only**: `aio_reader.py`, the 5 s
poll, and the section's coolant / CPU / fan / pump rows and sparkline. That code
is ported to hotaru rather than deleted from the monitor, so both programs read
the cooler for a while. Everything in that section that *writes* goes: the RGB
submenus, the LCD controls, the dashboard renderer and push loop, scenes.

### Later: the monitor becomes a consumer, and hotaru becomes an SDK

When the monitor is rewritten in Go it stops reading the cooler itself. First it
pulls the snapshot from `hotaru cooler status --json`; further out, it imports
hotaru directly — **hotaru as a small SDK for the cooler and for lighting**, the
way `fynedesygn` is the SDK for the interface. Two Go programs on one desk, one
of which knows the hardware.

Given the service, that SDK is most likely a *client* of the running hotaru
rather than a second driver of the devices: the monitor imports a client
package, the service stays the single owner, and no second process opens a
hidraw node. The device packages are what the client is built on, and are
importable too for anyone who wants them; which of the two the monitor uses is
its own decision when it gets there.

The end state is that **hotaru provides every AIO statistic the monitor
displays**, and the monitor opens no device at all. Pump-failure alerting moves
down at the same time; until then it stays where it is visible (open question
2). That sets a requirement on the snapshot hotaru produces: it carries the raw
values, not only what hotaru's own views happen to need.

That is off in the future and no spec here builds it. It has two consequences
now, both cheap to honour and expensive to retrofit:

- **JSON output is an interface, not a convenience.** It is designed as a stable
  shape from the start, because something will consume it before anything
  imports the packages.
- **The core packages are written as if they were already public**, even though
  they sit under `internal/` and nothing can import them yet. Concretely: no
  process-global state (the Python's module-level `_keyboard_effect` is exactly
  what not to carry over), no printing and no `os.Exit` below the `cmd`
  boundary, errors returned rather than logged-and-swallowed, `context` on
  anything that talks to a device or a socket, and no CLI or Fyne concept
  anywhere in them.

Promotion out of `internal/` is then a move and an import rewrite rather than a
redesign. It deliberately does not happen in this spec: exporting before the
shape is known creates a contract that has to be honoured, and nothing is
exported "just in case" — the same rule fynedesygn works under.

### The measured facts that shaped the original, restated

None of these stop being true in Go, and each cost real diagnosis:

1. **Lighting modes are per device and do not overlap.** The RTX 4090 accepts
   `direct/breathing/flashing/off` and rejects `static`; the Kraken accepts
   `static`. A broadcast mode is how the GPU was switched off during the
   original investigation — it errored on the mode and went dark anyway.
2. **OpenRGB indices are not stable.** A rescanned server lists every device
   twice, so an index is meaningful only within one listing. Devices are
   addressed by name; nothing persists an index.
3. **Static is not always the solid mode.** The ASUS Aura mainboard advertises
   both Static and Direct, but Static drives only the onboard LED — its four
   addressable headers go dark. Verified by A/B with one colour.
4. **`off` is not universally safe.** The Keychron advertises no Off mode, so
   `off` resolves to Direct with black, killing the backlight rather than
   dimming it. Some devices are coloured but never blanked.
5. **OpenRGB detects devices once, at server start.** A device enumerated late
   is absent for the session, and waiting on one device says nothing about the
   others (spec 033: the gate keyed on the Kraken, which is not what arrives
   last).
6. **A write that returns success may have changed nothing.** Reading the active
   mode back is the only confirmation; the exit code is not one.
7. **The kernel has no driver for this cooler.** `nzxt-kraken3` matches
   `2007/2014/3008/300C/300E`; the Kraken Elite V2 is `1e71:3012`. No hwmon
   node, `sensors` reports nothing. liquidctl is the only source of pump rpm and
   coolant temperature.
8. **liquidctl exposes no colour channels for this cooler.** Every
   `set <channel> color` returns "operation not supported by the device".
   Lighting is OpenRGB's; the LCD is liquidctl's. Two backends, one device.
10. **The LCD does not retain a static image.** A pushed PNG reverts to the
   built-in display in 5-10 s with nothing touching the device; a GIF plays
   indefinitely. The "flaky dashboard" was an expiry, not a race.
11. **The LCD misbehaves under repeated writes** (liquidctl#774 bucket-switch
    failures, and the firmware readout showing through mid-rewrite). The
    cheapest mitigation is not writing: a machine at steady idle should push
    nothing at all.
12. **One hidraw node, many callers.** The 5 s status poll, LCD writes and the
    OpenRGB server all reach the same device. Two concurrent liquidctl
    processes on one node is a corruption risk.

### Where hotaru departs from the Python

**Lighting speaks the OpenRGB SDK protocol; no subprocess.** The Python drives
the `openrgb` binary in `--client` mode and regex-parses `--list-devices` and
`--list-detailed`, including a hand-written parser for the bracket-and-quote
grammar of a `Modes:` line. That existed because PyQt already had `QProcess`. In
Go the SDK is a binary protocol on TCP 6742 and a client is an ordinary
dependency; the parsers, the 9-second silent-fallback trap and the
"3-character minimum name match" rule all disappear with the subprocess.

Measured on the development machine once the baseline existed, against the same
six devices: **0.6 ms to write one device and 2.4 ms to write all six**,
service-side, including the read-back after every write. Through the CLI, which
starts a process each time, a whole scene is 7.5 ms.

The Python's own measurement was 30 ms per `openrgb --client` invocation, one
invocation per device, serialised behind a queue — call it 180 ms for the same
scene, and 9.1 s for a single device if the server happened not to be running.
That is not a tuning difference. It is one persistent socket instead of a
subprocess per device, one frame per device instead of a mode call and a colour
call, and nothing to wait behind.

**The cooler still goes through liquidctl, as a subprocess.** Reimplementing the
Kraken's HID protocol is not in scope and liquidctl is the reference
implementation. So the queue comes with it — see below.

**The dashboard loses a dependency.** Qt drew the 640x640 image and Pillow
encoded the GIF, because Qt can read GIF but not write it. Go's standard library
writes GIF (`image/gif`), so the render is `image/draw` plus a font, and the
encode is stdlib. Nothing beyond Fyne is needed.

**Machine-specific tables become configuration.** `DEFAULT_SCOPE`,
`SOLID_MODE_OVERRIDES`, `OFF_EXEMPT`, `KEYBOARD_MATCH` and `BRIGHTNESS_DEVICES`
describe one desk: a Kraken, a 4090, a Maximus Z790, an MM700, a G502 and a
Keychron. A public program cannot ship those as law. They become rules in a
config file, matched by name substring, with this desk's values as the shipped
example rather than the default.

**The queue comes across, and for the first time it is complete.** Spec 026
routed lighting writes through `LiquidctlQueue` reasoning that "the Kraken is
reachable by both". The queue in fact serialised only the monitor's own
invocations — the process holding the Kraken's HID handle is the long-running
`openrgb --server`, which was never in it. Under this migration that stops
mattering in the way it did: hotaru becomes the single host-side owner of the
device across both backends, so the queue orders everything the host does to it,
which is what the original design was reaching for. Priorities carry over
(a user's write ahead of the status poll, the dashboard push droppable), as does
the per-device coalescing spec 026 added after three scenes in a second produced
twelve jobs against a bound of eight and dropped work at random.

## Rearchitecture, not a port

This is a rearchitecture. The monitor's code is **evidence, not a design**.

It accumulated over twenty-odd specs under a tray widget that was never meant to
own a cooler, and it shows: `aio_section.py` is 1450 lines holding the poll, the
writes, the menus, the health check, the scene application and the reassert
timer together in one Qt object. Much of its shape is PyQt's — `QProcess`
because the GUI thread must not block, a bounded job queue because `QProcess` is
asynchronous, module-level globals because there was nowhere else to put them.
None of that is a reason to build it that way in Go.

**What carries over is the knowledge**: the eleven measured facts above, the
device quirks, the failures that were diagnosed the hard way. Those were paid
for in dark GPUs, a 100 C CPU and four broken-hotkey investigations, and
re-deriving them would be the only genuinely wasteful thing this project could
do.

**What does not carry over is the structure.** The test for any piece of the
Python is: *which fact makes this necessary?* A behaviour that can cite one is
carried and cites it in a comment. A behaviour that cannot is an artefact of
where it happened to live, and is dropped.

Applied, that already changes several things:

| The Python does | hotaru does | Because |
|---|---|---|
| One 1450-line section object holding poll, writes, menus, health and timers | Small owners with one job each; the service composes them | The god object is why "does lighting work?" was unanswerable in specs 026 and 033 |
| A priority queue with `MAX_PENDING = 8` that drops the oldest job on overflow | A goroutine per device with a single-slot desired-state mailbox; the newest request replaces the pending one | Coalescing was bolted onto the queue in spec 026 after random drops. In Go, "latest wins per device" is the natural shape and overflow stops existing |
| `should_push()` compares coolant, pump, alert state and a CPU delta at displayed precision | Hash the rendered frame; push when the image differs | The Python is approximating "would the screen look different". The rendered bytes answer that exactly, and one hash replaces four hand-tuned thresholds |
| Module-level `_keyboard_effect`, `_lighting_last_color`, cached device lists | Explicit desired state owned by the service, passed as values | Spec 026's second fault was a cached device list nobody owned and nothing invalidated |
| Cooler fallback accepts any channel advertising a temperature | A source identifies the cooler explicitly, or contributes nothing | It matched the HX1000i PSU's "Probe" channels and reported a PSU temperature as coolant |
| `log.warning(...)` then `return None`/`False` | Errors returned as values; the shell decides what the user sees | "Every layer reported health" is the sentence that opens spec 029 |
| Health inferred from whether a call happened to succeed | An explicit status type with named states | Spec 026 had to add a health check precisely because success was ambiguous |
| A context menu as the primary interface | A `fynedesygn` shell window and a glance panel | A menu was what a tray widget could offer. This is a program |

The same test applies to anything discovered later in the Python that this spec
has not catalogued: cite the fact or drop the behaviour.

**The corollary, which matters just as much**: a fact with no test is a fact that
will be lost again. Each carried quirk gets a named test, the way `fynedesygn`
keeps a canary per Fyne quirk, so an upstream fix or a hardware change shows up
as one failing test rather than as three silent bugs.

### The Python tests are not ported

Written fresh, from the facts, against the new structure. The existing
`test_aio_*` and `test_rgb_*` modules are read for what they *assert about the
hardware* and then left behind.

This is a deliberate instruction from experience: the same translation was
attempted on terrariabonker and cost hours of picking through tests one at a
time, because a test written against the old structure encodes that structure.
A test for a 1450-line Qt section object, a bounded job queue or a module-level
`_lighting_last_color` has nothing to say about a service composed of small
owners with per-device mailboxes — it fails for reasons that are about the
port rather than about the program, and each one has to be reasoned about
individually before it can be discarded.

So the rule matches the one for the code: carry the knowledge, not the shape. A
Python test that asserts something true about the *hardware* — the GPU rejects
static, Aura's headers need direct, the Keychron has no Off mode, a rescanned
server lists everything twice — names a fact, and that fact gets a new test in
Go. A Python test that asserts something true about *PyQt* is evidence of
nothing and is not replaced.

## Someone else's machine

hotaru is a public program. The desk it was written on — a Kraken Elite V2, a
4090, a Maximus Z790, an MM700, a G502 and a Keychron — is a **test case, not
the target**. Everything in the Python assumed that hardware; a good deal of it
assumed that hardware *exists*. The rule for this rewrite is that none of it may.

### Nothing is required

Every backend and every capability is independently optional, and absence is a
state the program reports rather than an error it fails on:

| Absent | hotaru does |
|---|---|
| OpenRGB server not running | Lighting operations report it, with the address tried. Cooler features keep working. The service stays up |
| OpenRGB running, no devices | Reported as its own state, distinct from "unreachable" — one is a daemon problem, the other is a hardware or permissions problem |
| `liquidctl` not installed | Cooler telemetry and LCD are absent from the API and from the GUI. Lighting is unaffected |
| A cooler with no LCD, or no cooler | LCD and dashboard features are absent, not broken |
| OpenLinkHub not running | The CPU package temperature is missing from the snapshot; every other field still arrives |
| Not KDE, or not Plasma | Hotkey registration is absent and says why. Everything else works |
| Nothing at all present | The service starts, serves the API, and reports a machine it cannot do anything with |

A missing capability is missing from the interface, not present-and-failing. The
monitor already had the right instinct here — its AIO section stayed hidden
unless OpenLinkHub reported something — and it generalises.

### The default is "whatever is there"

This is a reversal worth stating plainly. The Python's `DEFAULT_SCOPE` is
`("kraken", "geforce", "maximus", "mm700", "g502", "keychron")` — six substrings
matching one desk. Shipping that as a default would mean hotaru lights nothing
at all on anyone else's machine, and offers no clue why.

**With no configuration, scope is every device OpenRGB reports.** Rules exist to
*narrow and correct*, never to enable. A user with no config file gets a program
that lights their machine; this desk's rules ship as a commented example, and as
the fixture the tests run against.

### Quirks are discovered, not assumed

The three device quirks carried from the Python are all instances of one
general fact: **a device's own reported capabilities are the only trustworthy
source, and a write that reports success may not have landed.** That is already
how mode resolution works, and it is what makes the design portable rather than
this-desk-shaped.

So the quirk table is an optimisation, not a prerequisite:

- Resolution consults the device's own mode list, so an unknown device gets a
  mode it actually advertises.
- After a write, the active mode is read back. A device that accepted a mode and
  did not take it falls through to the next candidate and the result is
  remembered for the session — which is the Aura "Static leaves the headers
  dark" discovery, performed by the program rather than by its author.
- `hotaru light probe` characterises what is present — which modes each device
  advertises, which ones actually take, which do not hold their colour — and
  offers the rules it would write. A new user's quirk table is generated from
  their hardware rather than inherited from mine. Where a question needs eyes
  rather than a read-back, it asks: see "Mapping is a wizard".

The shipped rules then read as what they are: known corrections for named
hardware, which a user with different hardware never loads.

### Nothing has a magic number

The LCD is 640x640 **on this cooler**. That comes from the device, not from a
constant: the dashboard renders at whatever size the panel reports, and a cooler
that reports no screen has no dashboard. The same applies to the reassert
interval, the push gate and the scene defaults.

Shipped scenes are therefore **colour-only**. The animation bank references GIFs
under `~/Pictures` that exist on one machine; as defaults they would be
eighteen broken scenes on every other one. They ship as a documented example,
and a scene whose file is missing reports it and still applies its colour half.

### Platforms

The core — OpenRGB client, device rules, scenes, the service, the API — is
portable. The integrations are not, and they are separated accordingly:
`systemd --user` and the socket path, the KWin script, the desktop entry. These
are build-tagged in the `fynedesygn` manner (`*_linux.go`, `*_other.go`), and a
platform without them loses those features and nothing else.

**On any desktop that is not Plasma, the CLI is the binding mechanism.** The
user binds `hotaru scene ember` in their own shortcut editor, which is how every
other desktop expects a program to be driven, and hotaru does not need to know
about their compositor. The KWin script is a KDE convenience, and a nicety
rather than a dependency — worth remembering given how much of this document it
occupies.

### The second-machine test

The acceptance test for everything above is a real one, and it is available: a
second CachyOS machine with substantially different hardware, **already running
OpenRGB in service mode to manage its own lighting**. That last detail makes it
a better test than a bare machine would be, because it exercises coexistence as
well as discovery.

The bar is *install and it works, with no extra steps*:

- [x] Install, start the service, and run `hotaru light list` — every device
      that machine has is listed, with its modes, and none of them are this
      desk's.
- [x] `hotaru light set <colour>` lights them, choosing a mode per device from
      what each advertises, with no rules written by hand.
- [x] `hotaru light probe` characterises that hardware and suggests rules that
      make sense for it.
- [x] Capabilities that machine lacks are reported as absent and are missing
      from the interface. Nothing fails, and the service starts regardless.
- [ ] The GUI opens, its System view draws that machine's devices, and a scene
      can be defined, previewed and saved. **Deferred to spec 005**, which
      builds the GUI; the CLI half of every other criterion here passed.
- [x] Hotkeys bind if it is Plasma; if it is not, the CLI binds in that
      desktop's own shortcut editor and the absence is explained rather than
      silent.
- [x] No step required editing a config file, and no step required knowing
      anything about the author's hardware.

Anything that fails there is over-fitting, by definition. A fix that special-
cases that machine is the same mistake a second time — the fix is whatever makes
hotaru derive the answer from the hardware.

### A fresh install is inert

Coexistence has a consequence worth stating as a rule. That machine's OpenRGB
already holds the user's own settings, and hotaru arriving and stamping colours
over them would be an unpleasant surprise from a program that was only
installed.

**The service applies nothing at startup unless it has desired state that hotaru
itself recorded.** A fresh install has none, so it starts, discovers, serves the
API and touches no device until asked. Reconciliation reasserts only what hotaru
was told to set; it never asserts a default, and there is no "restore a colour
at boot" behaviour that a user did not opt into by setting a colour.

The same rule protects the migration: hotaru installed on the primary machine
before cutover does not fight the monitor, because it has been asked for nothing
yet.

### Mapping is a wizard, because it is a conversation

Naming segments cannot be derived. A zone called "Addressable RGB Header 2" with
sixteen LEDs might be two daisy-chained fans, one fan with an unlit half, or
nothing at all, and no amount of reading the protocol will say which. The only
source is a person looking at the machine.

So hotaru asks, in a loop that is the same in both shells:

1. **Light distinctly.** Several zones at once, each a different colour, rather
   than one at a time — four headers in one write is one question instead of
   four. Beyond a handful of zones, or where colours are hard to tell apart, it
   falls back to lighting one and asking about that one.
2. **Ask what changed**, in the user's words: which fan is green, is any band
   only half lit, did anything not light at all.
3. **Split what needs splitting.** A zone that turned out to hold two fans is
   lit in halves and asked about again, which is how a range gets named without
   anyone counting LEDs.
4. **Write it down**, and from then on the names work everywhere.

This was rehearsed by hand on the development machine, and it took three rounds
to map a board with four headers, two of them empty, one carrying a
daisy-chained pair. The empty headers still reported sixteen LEDs each, which is
the detail that makes the wizard necessary rather than nice: **a zone's size is
what the board declares, not what is attached**, so an assignment to a header
with nothing plugged into it succeeds and lights nothing, and no API can tell
the difference.

#### The colours are the question

The wizard does not ask "which LEDs are the rear fan?" — nobody knows. It
assigns a colour per zone and asks the inverse: **what colour is each thing you
can see?** The user answers in their own words, naming their own hardware —
"rear blue, front dual is green" — and the mapping falls out of it, because
hotaru already knows which zone it made blue.

That shape has three properties worth keeping:

- **The user never learns hotaru's vocabulary.** They say "rear fan"; hotaru
  hears "Addressable RGB Header 3". The names in the rules file are then the
  user's own, which is why `maximus/rear` reads like something a person meant.
- **One round answers many questions.** Four zones in four colours is one look
  and one sentence, not four rounds of blink-and-confirm.
- **Wrong answers are cheap.** Nothing has been written; the lights are a
  preview. A user who mixes up two fans re-runs it, or fixes the name
  afterwards, and nothing on the machine is worse for it.

Use few colours and unmistakable ones. The rehearsal produced "bottom cyan(?)"
— a question mark that is entirely fair, because cyan and teal and white-blue
are the same colour to most people under a tinted case window. Red, green, blue
and white first; anything else only when there are more zones than that.

#### Ask how many things are on the chain

NZXT's own software does this, and so do other vendors' tools: light every zone
a different colour and ask which fan is which. That is worth knowing for two
reasons. It is evidence the interaction is right -- the people with the most to
lose from a confusing setup arrived at the same place -- and it means some users
will recognise it and expect its conventions.

It also asks a question this spec had missed. Before splitting anything, ask
**how many lit things are on this channel**. A daisy-chain is usually identical
fans, so one answer and the LED count give the split arithmetically: the
development machine's 24 LEDs over three fans is eight each, and the even split
was right the first time. Bisection is the fallback for when the division is not
clean -- mixed hardware, a strip on the same chain, a fan with a dead LED --
rather than the first move.

So the order is: how many, divide, light the proposed split, confirm. Three
questions where the naive version asks a dozen.

#### Finding a boundary nobody can count

When one zone turns out to hold several fans, the split has to land exactly
between them, and neither side knows where that is. Bisection finds it: light
the first half one colour and the second half another, and ask **whether any
single fan is showing two colours at once**.

- No fan is split, and the colours land on whole fans: the boundary is right.
- One fan shows both colours: the split falls inside it, and the next round
  moves the line by half the remaining distance.

That question — "is anything showing two colours?" — is the one a person can
always answer by looking, and it converges in a handful of rounds on a chain of
any length. Counting LEDs, which is the alternative, is something nobody does
twice.

It runs at two moments: on first use, and when a device appears that hotaru has
no mapping for. The second needs hotaru to remember which devices it has seen,
which is a line in its state file rather than a new mechanism.

A mapping session is a **preview** in the sense spec 004 gives the word: the
lights it sets are not desired state, reconciliation is suspended for the
devices involved, and what was showing before is restored when it ends —
including when the shell driving it dies halfway through.

**Where the answers are written is an open question** (see below). They are
rules, and the rules file is the user's, which hotaru does not write.

### The test suite must contain machines that are not this one

Tests run against fabricated machine profiles, not only against a fake of this
desk: a machine with one device, a machine with none, a device advertising only
modes nobody has heard of, a cooler with no LCD, an OpenRGB that is up but
empty, a liquidctl that is not installed. The question every test answers is
"what does a stranger see?", which is the question no amount of testing on the
author's machine ever asks.

## Out of the box, or it may as well be a script

Everything hotaru does can be done with a shell script. `openrgb --client` sets a
colour; `liquidctl set screen` pushes an image; a `.desktop` file binds a key.
The author had exactly that before any of this existed.

So the justification for a program is that it removes the gymnastics — and if a
feature does not remove any, a script would have served. That is the standing
test for anything proposed here: **what does this save a user who would
otherwise write ten lines of shell?** Serialising two backends onto one device,
re-asserting a colour a mouse keeps forgetting, discovering what a device can
actually do rather than what it claims, naming LED ranges by looking at them,
binding a key through an API that lies about success — those are the answer.
A wrapper around `openrgb --client` is not.

The same test applied to the first five minutes:

- **No configuration file is required.** Scope defaults to every device present;
  rules exist to correct, never to enable.
- **Nothing must be read first.** A user who does not know that lighting comes
  from OpenRGB and the screen from liquidctl should not be blocked by that —
  which is why the packages depend on the backends rather than suggesting them.
- **Diagnostics name the remedy, not the symptom.** "OpenRGB is installed but
  not running" beats a refused connection to a socket path. Where hotaru can
  perform the remedy itself, it offers to: start the OpenRGB server, enable its
  own user service, restart a server that enumerated a partial device list
  (spec 033's failure, which on someone else's machine is not diagnosable at
  all — hotaru knows what healthy looks like and can say so).
- **The GUI replaces the gymnastics that remain.** Naming LED ranges, binding
  keys, building a scene: each is otherwise hand-edited configuration, and each
  is a thing a picture does better.

### Offer, never act

The balance to hold, because it is easy to overshoot into the opposite
annoyance: hotaru **offers** and never surprises. Starting a daemon, enabling a
unit, writing rules, taking a key — each is one clear question with a visible
answer, not something that happened while the user was looking elsewhere.

This is not in tension with "a fresh install is inert". Inert means hotaru
asserts nothing over hardware nobody asked it to touch. Out-of-the-box means
that when the user *does* ask, nothing between them and the result requires
knowing how the program is built. A program that stamps colours on install is
rude; a program that makes you read an architecture document to see one light
change is useless. The line between them is a question with a Yes button.

## Spec roadmap

This spec defines the project and the migration contract, and implements the
**baseline**: the service, its API, and lighting. Lighting first because it has
no liquidctl dependency, so the whole path — socket, API, client, hardware — can
be stood up and exercised before anything is taken from the monitor. Every later
spec adds capability behind an API that already exists.

| Spec | Scope |
|---|---|
| **001** (this one) | Project scope, the migration contract, and the baseline: config, OpenRGB client, device rules, desired state, reconciliation, `hotaru serve` on its socket, API v1 for lighting, and the CLI as its first client |
| 002 | Cooler telemetry behind the same API: liquidctl reader, OpenLinkHub fallback, per-device serialisation, the snapshot shape |
| 003 | The LCD: `set screen`, the dashboard render at the panel's own size, push gating, GIF keepalive |
| 004 | Scenes, preview and leases: the scene model, `Preview`/`Apply`/revert, reconciliation suspension, shipped defaults |
| 005 | The GUI on `fynedesygn`: service management, visual scene staging, hotkey binding, the System view, and a `glance` panel for the cooler |
| 006 | Hotkeys and cutover: the thin D-Bus door, the KWin script, the monitor's removal commit, hotaru taking the keys |
| 007 | Packaging and release: the PKGBUILD, the AUR packages, the systemd user unit, the desktop entry — shape decided in `docs/packaging.md`, built once there is something to install |

### Packaging is decided now and built later

Settled in [docs/packaging.md](../docs/packaging.md) before there is anything to
package: the Arch packages **depend on all three backends**, and split into
`hotaru` (CLI and service, no graphics stack) and `hotaru-gui`.

That is not a contradiction of the rules above, and the distinction is worth
holding onto. `depends` describes the supported install — what has to be there
for `pacman -S hotaru` to hand someone a program that works. Runtime optionality
describes a machine's state — a daemon that is stopped, a device that vanished,
a `go install` with no package manager involved. The program degrades in all of
those regardless of how it was installed.

The build itself is spec 007, after there is something worth installing.

## Migration from peripheral-battery-monitor

A move, not a fork. The monitor's AIO and RGB support is **removed permanently**
and pushed as an ordinary commit on its own repository.

### What is removed from the monitor

Whole files: `rgb_openrgb.py`, `aio_scenes.py`, `scene_service.py`,
`scene_shortcuts.py`, `aio-scene`, `aio_liquid.py`, `aio_dashboard.py`,
`build_reels.py`, and the `test_rgb_*`, scene and dashboard test modules.

`aio_color.py` loses its OpenLinkHub RGB request builders — `setOverride`,
profile selection, brightness — and with them its reason to exist; the named
colour table and `parse_color` move to hotaru. Whether the remainder is deleted
or kept as a stub depends on what the retained section still references.

`aio_section.py` is stripped rather than deleted: the Colour, Effect and
Brightness submenus, `apply_scene`, the lighting health check, `_lighting_devices`
and its refresh, the lighting reassert (spec 037), the LCD menu and the dashboard
push loop all go. What remains is the poll, the snapshot rows, the sparkline and
the pump alert.

`aio_queue.py` stays, now serialising only the monitor's own reads.

`aio_reader.py` stays, and is **ported** to hotaru rather than moved — both
programs read the cooler until the monitor's Go rewrite makes it a consumer.

From `peripheral-battery.py`: the scene service registration, the shortcut
install, and the `aio_scenes`, lighting-scope and dashboard settings keys.

From `README.md`: the RGB control, scenes, hotkey and LCD/dashboard sections. The
AIO Monitoring section stays, minus everything it says about writing.

The `openrgb-server` systemd user unit is **not** removed. It becomes hotaru's
dependency and its ownership — including the enumeration gate from specs 026 and
033 — transfers with the lighting. Deleting it would break hotaru on the same
machine that just gained it.

### It is one cutover, not a staged retreat

The monitor keeps working untouched until hotaru has parity across lighting,
telemetry, LCD, dashboard, scenes and hotkeys. Then one commit removes every
write path in one go. A staged removal would leave two processes both *writing*
one hidraw node, which is the invariant this stack has been most careful about
for eighteen specs.

**Concurrent reads are accepted; concurrent writes are not.** After cutover the
monitor polls `liquidctl status` every 5 s while hotaru writes, and that is a
deliberate, temporary overlap — it ends when the monitor's Go rewrite pulls from
hotaru's CLI. Before cutover the monitor still writes (the dashboard push), so
the working rule for specs 002 and 003 is to stop the monitor before exercising
hotaru's LCD or lighting writes.

### The cutover order

1. hotaru reaches parity and is installed, with no key bindings at all.
2. The monitor's removal commit lands and is pushed.
3. The monitor is **bounced**, and the release of its claims is verified.
4. The stale `kglobalshortcutsrc` entries are cleared.
5. Only then does hotaru register the bindings.

### Verifying the monitor actually let go

A bounce is not evidence by itself; every layer in this stack has a documented
habit of reporting success while holding on. After restarting the monitor,
confirm all five:

- **The KWin script is gone.** `isScriptLoaded("aio-scenes")` over
  `org.kde.KWin /Scripting` returns false, and introspecting `/Scripting` shows
  no `ScriptN` object belonging to it. An unload that left the object alive is
  the fault spec 034 exists for.
- **The D-Bus object is unexported.** `org.agscripts.PeripheralBatteryMonitor`
  no longer offers `/Scenes`. The name itself persists — the monitor still owns
  it for `kwin_window_position` — so the object, not the name, is the check.
- **No liquidctl process belongs to the monitor**, and no 5 s poll appears
  against the cooler.
- **The keys are unbound.** `Ctrl+Alt+Num+1..9` and `Ctrl+Alt+Shift+Num+1..9`
  do nothing, and `kwin_wayland`'s journal shows no `aio-scenes` registration
  line on monitor start.
- **The stale entries are cleared.** See below — the bounce does not do this one.

### Stale `kglobalshortcutsrc` entries

Eighteen `AIOScene*` entries currently sit in the `[kwin]` section of
`~/.config/kglobalshortcutsrc`:

```
AIOScene1=Ctrl+Alt+Num+1,none,AIO lighting scene 1
AIOScene11=Ctrl+Alt+Shift+Num+1,none,AIO lighting scene 11
...
```

KDE writes them when the KWin script registers its shortcuts, and they **outlive
the script**. Unloading the script does not remove them; neither does restarting
KWin. They are not stow-managed — nothing in dotfiles references `aio-scene` —
so this is machine state, cleared by hand-editing that file, not a dotfiles
commit.

They must go before hotaru registers anything, because they hold a claim on
exactly the sequences hotaru wants. A claimed key is how this class of bug
presents: the registration reports success and the key does nothing.

## KDE hotkey ownership (read before writing any of it)

Global shortcuts on this desk have broken four times, in four different ways, and
each fix looked complete. Specs 025, 029, 033, 034 and 036 are the record. The
rules below are not preferences; each is a fault diagnosed the hard way, and
hotaru will reproduce every one of them if it re-derives the mechanism from the
documentation. They belong in this spec rather than in 005 because they shape the
service interface hotaru exposes, which is built here.

**The mechanism is KWin scripting, not `.desktop` shortcuts.** The first
implementation used the mechanism every other custom shortcut here uses: a
`.desktop` file plus a `[services]` entry in `kglobalshortcutsrc`. It works — but
only from the next login, because the key *grabs* for service shortcuts are
installed when KDE enumerates that file at session start. Registering the same
component later over the KGlobalAccel D-Bus API puts an entry in the table, and
the key even reports as claimed, but installs no grab and leaves no process to
notify. Both were verified on this hardware. KWin scripting registers a real grab
immediately.

**The handle is a plugin name, not a file path.** The API is:

```
loadScript(in s filePath, in s pluginName, out i)
loadScript(in s filePath, out i)
unloadScript(in s pluginName, out b)
isScriptLoaded(in s pluginName, out b)
```

The one-argument `loadScript` defaults the plugin name to the path, which is why
passing paths appears to work. Loading under an explicit name allocates a fresh
Script object; the path-only form has been observed returning an id belonging to
an unrelated already-running script, whose `run()` is a silent no-op.

**Five calls in this chain report success while doing nothing.** Every one is
attested:

1. `loadScript` on a path KWin has already seen returns the cached script's id
   *without re-reading the file*. Hence a unique path per install.
2. `run()` on an already-run object returns success and does nothing.
3. `unloadScript` returns true and does **not** destroy the Script object; its
   `/Scripting/ScriptN` node stays registered and its id stays occupied.
   `stop()` on the object is what destroys it.
4. The id `loadScript` returns can collide with a survivor of (3) — observed
   returning `2` while creating no object at all.
5. `isScriptLoaded(path)` returns true for a script that is not running.

So: a unique path per install, sweep stale generated files, unload the
predecessor by **name** and `stop()` its object, identify the object a load
created by introspection difference rather than by the returned id, and verify
with `isScriptLoaded(name)` — treating false as a failed install rather than
assuming success.

**The shortcuts live exactly as long as the loaded script.** They are not
persistent registrations. Reinstalling the script on every start is what keeps
the program and its bindings from drifting apart, and is why a stale script is
worse than no script.

**The KWin half must be visible in the journal.** The generated script prints its
registration count on run and prints on every activation. Without it there is no
way to distinguish a key that never fired from one that fired and failed to reach
the program — a distinction that cost days across specs 029 and 034. The
chattiness is the feature.

**One process owns the bindings.** Exactly one instance installs the script,
behind a single-instance guard; a second instance registering over the first
produces (3) and (4) at will.

## hotaru runs as a user service

Settled, and it decides a good deal of the architecture below. hotaru is a
resident `systemd --user` service; the CLI and the GUI are clients of it.

Three things force it, none of which a one-shot process can do:

**Devices lose state and nothing puts it back.** The G502 X PLUS is wireless,
OpenRGB sets a *volatile* effect, and the mouse restores its onboard state on
wake — so the scene colour is silently lost minutes after it was set. OpenRGB's
CLI has no device-persistence option (`--save-profile` is host-side), so the
colour cannot be made to stick; it has to be re-sent. Spec 037 landed a 60 s
re-assert scoped to that one device, after eliminating two well-supported wrong
theories (a Solaar conflict — no solaar daemon runs here; and the device's idle
timer — `RGBIdleTimeout.read()` returns a hardcoded 60, not a device query, and
its write drives a host-side manager). The conclusion stands: **some devices
need their state re-asserted forever**, and something must be alive to do it.

**The LCD does not retain a static image.** A pushed PNG reverts to the built-in
display in 5-10 s; a GIF plays indefinitely. That is why the dashboard is a
single-frame GIF — but the dashboard is also a *live* render, so something must
be alive to re-render and re-push it when the values move (and, per spec 028's
gating, to decline to push when they have not).

**Startup is a reconciliation problem, not an event.** OpenRGB enumerates once
at server start, and on a cold boot it has been observed finding two devices out
of six (spec 033). A process that applies lighting once at login applies it to
whatever happened to be enumerated at that instant and reports success. A
service that holds desired state and reconciles toward it recovers on its own.

### What the service owns

- **Desired state**: the scene or colour last asked for, per device, and the LCD
  mode. Not "what was applied" — what *should* be true.
- **Reconciliation**: re-asserting desired state on an interval, per device rule
  rather than for everything. Spec 037's `LIGHTING_REASSERT_SCOPE = ("g502",)`
  generalises into a `reassert` interval on a device rule, alongside the other
  quirk fields, so the rule reads as "this device does not hold its colour"
  rather than as a special case in code.
- **The LCD**: the dashboard render loop, the push gate, and keepalive.
- **The queue**: it is the single writer, so serialisation and coalescing have
  one home.
- **The D-Bus interface** the hotkeys and the shells call.
- **The KWin script**: one process installs it, which is the single-instance
  requirement in "KDE hotkey ownership" satisfied by construction.

A re-assert is **not a user choice**: it neither persists settings nor emits a
change event, so it cannot be mistaken for an instruction and cannot ring the
UI. Spec 037 tested exactly this, and the rule carries over.

### Starting at boot, not at login

The service starts with the machine, not with a desktop session. The point of
recording desired state is to put the lights back the way the user left them,
and a restore that waits for someone to log into Plasma is a restore that has
missed the moment it existed for.

So:

- **A `systemd --user` unit with no desktop dependency.** `WantedBy=default.target`,
  and nothing about `graphical-session.target`. Nothing in the service needs a
  compositor, a display or a toolkit — the whole of it is a socket, an OpenRGB
  connection and a liquidctl subprocess.
- **Boot-time start needs lingering** (`loginctl enable-linger`), because a user
  manager otherwise starts at first login. hotaru detects when it is not enabled
  and offers to enable it, per "Offer, never act" — the package does not do it,
  because turning on a user manager at boot is not a packager's decision.
- **The socket is unaffected.** `$XDG_RUNTIME_DIR` exists from boot under
  lingering, so the API is reachable before any login, and the user-scoped
  security argument for having no authentication stands unchanged. This is the
  reason not to make it a system service: root, `/etc`, and a socket the whole
  machine can reach would all be new problems in exchange for a start-up
  ordering that lingering already solves.
- **The OpenRGB server may be anyone's.** The `openrgb` package ships a *system*
  unit and udev rules; this machine runs a user unit of its own instead. hotaru
  connects to the configured address and does not care which started it — but it
  does mean the server's own start-up is outside hotaru's control, and the
  restore has to tolerate that.

#### Ordering is the wrong tool for the OpenRGB dependency

A restore obviously depends on the OpenRGB server being up, and the reflex is to
express that in the unit: `After=`, `Requires=`, `Wants=`. It does not work, for
two independent reasons.

**There is no one unit to name.** The `openrgb` package ships a *system* unit,
`openrgb.service`, with udev rules. This machine instead runs a *user* unit,
`openrgb-server.service`, carrying the HID enumeration gate from specs 026 and
033. Someone else runs the server by hand, or from their desktop's autostart, or
inside a container. A hard dependency on a unit name is a dependency on one
person's setup — the same over-fitting the rest of this document is trying to
avoid — and a failed `Requires=` would stop hotaru from starting at all, taking
the cooler and the API down because the lighting daemon is named differently.

**Started is not ready, and this is measured.** Even with correct ordering,
`openrgb-server` has been observed reaching `Started` and enumerating two
devices out of six on a cold boot, because OpenRGB detects devices once and USB
enumeration had not finished. Ordering can only wait for a unit to report
itself up; it cannot wait for the hardware to be there. Spec 033 exists because
that distinction was learned the hard way.

So the server is treated as a **resource that appears**, not a dependency that
is satisfied: connect with retry, and judge readiness by the device list rather
than by the socket accepting a connection. Where OpenRGB is installed but
nothing is running it, hotaru says so and offers to start it — detecting which
unit exists rather than assuming a name.

**Restoring is reconciliation, not a start-up step.** The service does not apply
state once at boot and consider itself done. It reconciles toward recorded state
as soon as the backend is reachable, and keeps reconciling — which matters more
at boot than anywhere else, because starting earlier means meeting the
enumeration race more often, not less. OpenRGB detects devices once at server
start, and a cold boot has been observed finding two of six. So a restore that
matches fewer devices than the recorded state names is an **unfinished restore**:
it retries with backoff, reports itself as incomplete rather than successful,
and completes silently when the rest appear.

**Measured on the first cold boot after this landed.** The machine came up, and
the journal is the argument for every paragraph above:

	13:29:26  Started hotaru lighting service
	13:29:26  no OpenRGB server at 127.0.0.1:6742 yet; lighting waits for one
	13:29:42  openrgb-server.service becomes active
	13:29:49  connected to the OpenRGB server (protocol 3)
	13:29:49  restore incomplete: 1 devices put back, still waiting for 5
	13:29:50  restore incomplete: 3 devices put back, still waiting for 3
	13:29:53  restored 6 devices to what they were showing

hotaru started **sixteen seconds before** the server it depends on, waited,
connected seven seconds after that unit reported itself active, and found
**one** device of six enumerated. Ordering after any OpenRGB unit would have
restored that one device and stopped. The full restore took twenty-seven
seconds from service start, and every device came back to the colour it was
showing before the reboot.

A fresh install with no recorded state still does nothing at boot, which is the
inert rule unchanged — there is simply nothing to restore.

### The desktop parts attach when the desktop appears

A service that starts before any session cannot register a KWin script at
start-up, because KWin is not there. That is not an obstacle; it is the correct
shape, and it fixes an old failure by accident.

The hotkey integration **watches the session bus for `org.kde.KWin`** and
installs the script when it appears — at login, and again whenever KWin
restarts. The Python installed the script once, at monitor start, which is why a
KWin restart silently took the shortcuts with it and left a program that
believed it had them. Watching the name owner makes the binding as durable as
the desktop rather than as durable as one moment in it.

The same applies to anything else session-shaped: it is an attachment to a
session that may come and go, never a precondition for starting.

### The service is the only actor; everything else asks it

The CLI and the GUI are clients. Not "may write directly when convenient" — the
service is the single actor on the hardware, and every workflow event, from
whatever source, enters through the same door and is serialised by the same
owner. Two callers cannot fight over a device because there is only ever one
caller. Parity stops being a discipline and becomes a shape: both shells call
the same API, and that API is the contract the parity test enumerates.

**The API is HTTP and JSON, served on a Unix domain socket in
`$XDG_RUNTIME_DIR`.** A microservice in shape — versioned paths, a request per
operation, an event stream for live state — over a transport scoped to one
user's session. There is **no TCP listener**: the socket is what makes it safe
to leave unauthenticated, because filesystem permissions already say "this
user's session and nothing else", which is exactly the audience. Adding a port
would mean adding authentication to go with it, for a program that changes
light colours on the machine its user is sitting at. OpenRGB's own SDK is the
cautionary example — no authentication at all, which is why hotaru reaches it
over loopback and says so in its README.

Being HTTP still pays: `curl --unix-socket` is a debugging session, any language
has a client, and if a listener is ever genuinely wanted the address is a
config line rather than a rewrite.

**A D-Bus object exists as well, and is deliberately thin.** Not a second API:
KWin's scripting API can only reach the outside world through `callDBus`, so a
hotkey physically cannot call anything else. That endpoint's whole job is to
receive "scene N was pressed" and hand it to the same service methods the HTTP
API calls. One flow, two doors, and the narrow door exists because KWin gives
no other.

Three things this buys that a library-in-each-shell would not:

- **The System view can stream.** Live device state over an event stream
  (`GET /v1/events`) rather than the GUI polling hardware it is not allowed to
  touch.
- **The preview lease has a natural lifetime.** It is held by an open stream, so
  a GUI that crashes drops the connection and the service restores desired state
  without anyone having to notice.
- **The future SDK is nearly free.** The Go monitor imports a client package
  that speaks this API; no second process opens a device, and "hotaru provides
  the AIO stats" becomes an HTTP call rather than a shared-memory problem.

It also changes what "no direct writes" means in practice. Once the service
exists, **a shell never writes a device**; it asks the service to. A write
command with the service down fails with one line saying so, the way `aio-scene`
did, rather than writing directly — two writers is the failure this whole
migration exists to end, and a convenience fallback would reintroduce it.
Diagnostics keep an explicit escape hatch: `--direct` performs read-only
queries against OpenRGB without the service, documented as "only while the
service is stopped".

**The service is baselined first, and no shell ever writes a device.** There is
no interim in which the CLI drives hardware directly and is converted later.
That ordering was considered and rejected: it means writing the CLI twice, and
the second version is written under the worst conditions — against an API whose
shape is being invented at the same moment, with a working direct path sitting
right there as the tempting shortcut.

Building the service first inverts every one of those problems. The API is
designed while it is the only thing that exists, so it is shaped by the domain
rather than by a CLI's existing structure. The CLI is written once, as a client,
and is automatically proof that the API is usable. And the single-writer
invariant is true from the first commit rather than being a property the project
converges on.

### Throwaway clients, early and often

The practical technique that makes service-first work: as each endpoint appears,
it is exercised by a **one-off client** — a `curl --unix-socket` line, or twenty
lines of Go — before any real consumer exists. Endpoints get used the day they
are written, by something with no investment in them.

This is how behaviour gets defined rather than assumed. Questions like "what
does applying to a device that vanished return?", "what arrives on the event
stream when a reconcile fires?", "what does health say with an empty server?"
are answered by asking the running service, at the point where changing the
answer is free. An endpoint nobody has called is a design, not an interface.

Those probes are kept: the ones that capture a decision become the worked
examples in the API documentation and the fixtures for its tests, so the
transcript that defined a behaviour is the same artefact that guards it.

### Shape

One binary. `hotaru serve` is what the unit runs; `hotaru light set …` is what a
person runs. A separate `hotarud` would duplicate flags, config loading and the
version stamp for no benefit, and the systemd unit does not care which form the
command takes.

## Addressing: devices, zones, segments

A colour per device is the coarsest thing lighting hardware can do, and it is
all the Python ever did — `set_color_argv(device, mode, rgb)`, one colour for
whatever that device is. But the Kraken's radiator fans daisy-chain into it as
one addressable zone, and a user who wants the top fan red and the bottom fan
blue is asking a reasonable question that the old model cannot express at all.

So the addressable unit is not the device. It is a **target**:

```
kraken                    the whole device, every zone
kraken/ring               one zone
kraken/ring[0:11]         an LED range within a zone
kraken/fan-top            a named segment, defined once in config
```

Whole-device is the shorthand for "every zone", not a separate concept — so
simple scenes stay simple and granular ones are the same mechanism, further
down.

### Segments are named, because indices are not an interface

`kraken/ring[12:23]` is a correct way to say "the middle fan" and a terrible way
to remember it. LED ordering is per device, per firmware, and occasionally per
how the cables were plugged in. So a user names ranges once, in the device rule:

```yaml
devices:
  - match: kraken
    segments:
      fan-top: {zone: ring, leds: [0, 11]}
      fan-mid: {zone: ring, leds: [12, 23]}
      fan-bot: {zone: ring, leds: [24, 35]}
      pump:    {zone: logo}
```

and scenes address `kraken/fan-top` thereafter. Naming is the moment the user
works out which LEDs are which fan, and it happens once rather than every time
they write a scene. `hotaru light probe` helps: it reports each device's zones
and LED counts, and can walk a zone one LED at a time so the user can watch
which light moves and name what they see.

### A scene is a list of assignments

```yaml
scenes:
  cold-top:
    - {target: kraken/fan-top, colour: "#1d55ff"}
    - {target: kraken/fan-mid, colour: "#3daee9"}
    - {target: kraken/fan-bot, colour: white}
    - {target: gpu,           colour: "#1d55ff"}
    lcd: dashboard
```

Order does not matter and later assignments to the same LEDs win, which makes
"everything blue, except the top fan" expressible as two lines rather than as an
enumeration.

### Assignments compose into one frame per device

This is the part that reaches into the architecture. A device is not written
once per assignment — the assignments for a device are **resolved into a single
frame** (the full colour list for that device) and that frame is what goes into
the device's mailbox.

Three reasons, all of which are faults in the naive version:

- **Partial writes race.** Three assignments sent as three writes can interleave
  with a reconcile or a second scene and leave a device showing halves of two
  scenes. One frame is atomic from the device's point of view.
- **Coalescing still works.** "Latest wins per device" only means anything if
  the unit is a complete device state. A mailbox holding the latest *fragment*
  would drop the other fragments of the same scene.
- **Desired state stays comparable.** Reconciliation asks "is the device showing
  what it should?", which is a question about a frame, not about the last
  instruction sent.

So desired state is: per device, a frame; plus the LCD mode. A whole-device
assignment fills the frame uniformly, which is why the simple case costs
nothing.

### Per-LED control needs a mode that supports it

Most devices expose per-LED colour only in Direct; several render Static as one
colour for the whole device, which is a silent downgrade rather than an error.
Mode resolution therefore takes the *granularity* into account: a frame with
more than one distinct colour resolves against modes that accept per-LED data,
and a device that has none is **reported, not approximated**.

What it must not do is average the colours, pick the first, or apply the frame's
dominant colour and call it success — the user asked for three fans in three
colours, and a device that cannot do that should say so in `hotaru light set`'s
per-device output and in the GUI, where the answer is visible while the scene is
being designed.

## Scenes

Scenes move whole, both halves, with every shipped default. A scene is a colour
and an LCD mode applied as a unit:

```yaml
scenes:
  red:     {colour: red,      lcd: dashboard}     # every device in scope
  liquid:  {colour: white,    lcd: liquid}
  dark:    {colour: "off",    lcd: liquid}
```

The short form above is the common case: one colour, every device in scope. The
long form is a list of assignments against targets — see "Addressing" — and the
two are the same model, since `{colour: red}` is "assign red to everything in
scope".

`colour` is a named colour, `#rrggbb`, `off`, or absent to leave lighting alone.
`lcd` is `dashboard`, `liquid`, a path to an image or GIF, or absent to leave the
screen alone. A scene with neither is rejected at load with a reason.

Scenes are **named rather than numbered**. The 1-9 / 11-19 numbering exists in
the Python because numpad keys are numbers and the two banks shared one D-Bus
method; a name survives a rebind, and slot numbers belong to the binding layer
(spec 006), which maps a key to a scene name.

The eighteen current defaults ship as hotaru's defaults, including the animation
bank and the colours derived from each animation — those were measured from the
frames, not chosen, and re-deriving them is work already done. The GIF paths
under `~/Pictures` stay user paths: hotaru ships the scene, not the media, and a
scene whose file is missing reports it and still applies its colour half.

Brightness stays out of a scene, as in the original: a shortcut that
unexpectedly dimmed a keyboard would be a surprise, and brightness is a standing
preference rather than part of a look.

## The GUI

Built in spec 004. Its layout is deliberately left open; its **purpose** is
settled here, because the purpose determines what the service interface must
carry, and that interface is designed in this spec and in 005.

The GUI is the program's control surface. Three jobs:

1. **Manage the service** — is it running, what does it see, what is it doing,
   make it reconcile now.
2. **Visualise, define and create scenes** — the reason the GUI exists. The
   editor works on a **staged** scene: a draft held in the editor, which the
   user may optionally push to the hardware to see it, and which becomes real
   only when saved.
3. **Bind and re-bind hotkeys** without hand-editing anything.

Plus a **System** view: a graphical representation of the RGB devices in the
machine with their status — what is present, what is in scope, what mode each is
in, which ones are being re-asserted. The thing the Python could only express as
a context menu of device names.

**This is the workflow the GUI exists for.** Granular addressing is precisely
what a config file is bad at and a picture is good at: the System view draws a
device's zones and segments, the user clicks the top fan and gives it a colour,
sees it on the hardware, names the segment they just worked out, and saves the
result as a scene. Writing `kraken/ring[12:23]` by hand is possible and nobody
wants to. The GUI is where a machine's addressable units become named,
visible things — and naming them is a one-time act the CLI can then use forever.

Five consequences follow, and they land on this spec rather than on 004:

**Service management is a first-class capability, so the CLI has it too.** The
parity rule is not "the GUI can do what the CLI can"; it is symmetrical. Status,
health, reconcile-now and reload are service operations, reachable from both
shells, and the machine-readable forms exist because a headless box over SSH is
the case with no GUI at all.

**Staging is the editing model, and it is three states, not two.** A scene
being edited is a *draft* in the GUI; nothing has been written anywhere.
Pushing it to the hardware is a *preview*: the devices show it, and it is still
not what the user wants — it is what they are looking at. *Saving* is what makes
it a scene. Dragging a colour slider therefore changes nothing until asked,
which is what makes a colour picker usable at all: the alternative writes the
hardware sixty times a second and records each one.

So the service interface carries `Preview`, `Apply` and revert-to-desired as
distinct operations rather than one write with a flag — the same distinction
spec 037 drew for the re-assert (`remember=False`, emitting no change event),
generalised. Three rules fall out of it, and each is a way the naive version
breaks:

- **A preview suspends reconciliation for the devices it touches.** Otherwise
  the reassert timer restores desired state underneath the user — on the G502,
  within 60 seconds — and the editor appears to lose its own colour for no
  visible reason. Suspension is scoped to the previewed devices and ends when
  the preview does.
- **A preview cannot outlive its client.** If the GUI is closed, crashes, or
  loses the bus while hardware is showing a draft, the service restores desired
  state on its own. A preview is held as a lease tied to the caller, not as a
  mode the service can be left stuck in.
- **A preview is visible as a preview.** The GUI says the hardware is showing an
  unsaved draft and offers the way back, because a user who closes the editor
  and later wonders why the mouse is green has been given a puzzle by the
  program.

**One writer owns the config file.** The GUI saving a scene while the service
persists desired state is two processes writing one file. The recommendation is
that the **service owns the file** and the shells ask it to persist, which also
means a saved scene takes effect without a reload. The alternative — shells
write, the service watches — is more moving parts for the same result, and
fynedesygn's `settings` writes a second after the last change, which is a race
worth not having. Staging makes this cheap either way: a save is one request
carrying a complete scene, not a stream of edits.

**Everything the GUI does is a request.** Staging, previewing, saving,
rebinding, reconciling — the editor holds a draft and the service does the work,
so the GUI never holds a device handle and a second GUI cannot contend with the
first. It also means the GUI is not privileged: anything it can do is an API
call the CLI can make too, which is the parity rule falling out of the
architecture rather than being enforced on top of it.

**Re-binding is a service operation, not a GUI one.** Changing a key means
re-installing the KWin script, which is the load/unload/`stop()`/unique-path
dance in "KDE hotkey ownership" — one process does that, and it is the service.
The GUI captures the key and asks. Two further problems belong to spec 004 and
are named here so they are not discovered late: **a key may already be claimed**
(KDE reports success and the key does nothing, which is the failure mode this
whole document circles), so a bind must verify the grab and report a conflict
by reading what holds the sequence; and **capturing a key combination inside a
Fyne window under Wayland** — including `Num+` keys — is not obviously
supported, so the fallback of choosing a modifier set and a key from controls
should be assumed until it is proven otherwise.

**The System view needs observed state, not just configuration.** Per device:
present or absent, in scope or not, the active mode as the device reports it,
the last colour applied and when, whether a rule re-asserts it, and whether the
last write landed. That extends requirement 13 — the snapshot is the raw truth
about the machine, and the view is a rendering of it, so the GUI never queries
hardware itself.

## Files on disk

YAML, `.yml`, through `fynedesygn`'s `settings` package with `settings/yamlcodec`
imported for its effect. YAML because these files are meant to be read and
hand-edited: device rules and scenes are full of names, ranges and comments
explaining which fan is which, and JSON is a poor host for all three.

Four files, and the split is not tidiness — each one has a different owner and a
different failure if that is confused:

| File | Owner | Holds |
|---|---|---|
| `$XDG_CONFIG_HOME/hotaru/hotaru.yml` | The user | Device rules, named segments, service preferences. hotaru reads it and never rewrites it |
| `$XDG_CONFIG_HOME/hotaru/scenes.yml` | The service, on request | Scenes. Machine-written, because the GUI is the scene editor |
| `$XDG_STATE_HOME/hotaru/state.yml` | The service | Desired state, learned mode fall-throughs, last applied. Never hand-edited |
| `$XDG_CONFIG_HOME/hotaru/gui.yml` | The GUI | Window geometry, colour scheme, fonts, last section, editor preferences |

### Why rules and scenes are separate files

**A YAML rewrite destroys comments.** Any encoder that serialises a decoded
document drops every comment and every bit of formatting the user put there.
That is fine for a file the machine owns and unacceptable for the one where a
user wrote `# the top fan is the one nearest the radiator inlet` — which is
exactly the comment that makes a segment table worth having.

So the file the user writes is never rewritten by the program, and the file the
program writes is one the user is not invited to decorate. Scene saving from the
GUI touches `scenes.yml` only. A user who prefers to hand-write scenes may still
do so; they simply inherit the machine's formatting the next time the GUI saves.

### Why desired state is not config at all

Desired state is a record of what the hardware should currently be showing. It
changes every time a scene is applied, it is meaningless on another machine, and
it belongs in `$XDG_STATE_HOME` rather than in a file anyone backs up as
configuration. Keeping it out of the config file is also what lets the
inert-on-install rule be simple: a fresh machine has no state file, so there is
nothing to assert.

### The GUI's file is the GUI's alone

The service never reads `gui.yml`, and the GUI never writes any of the other
three. Everything the GUI changes about hotaru's behaviour goes through the API
as a request — saving a scene, editing a rule, rebinding a key — so there is one
writer per file and no coordination problem to solve. What `gui.yml` holds is
view state: where the window was, which scheme, which section was open. Losing
it costs a user their window position and nothing else.

## Proposed layout

The system these pieces make up is drawn in
[docs/architecture.md](../docs/architecture.md), which is updated in the same
commit as any decision that changes it.

SDK candidates are marked `*`: they are the packages the future Go monitor would
import, so they carry the "written as if public" constraint above and stay free
of anything view-shaped.

```
cmd/hotaru/            CLI, and `hotaru serve` — the user service (cobra)
cmd/hotaru-gui/        Fyne GUI on fynedesygn (spec 004)
internal/openrgb/    * SDK client: connect, list, set mode/colour
internal/devices/    * Rules, scope matching, intent-to-mode resolution
internal/colour/     * Named colours and #rrggbb parsing
internal/cooler/     * liquidctl and OpenLinkHub readers (spec 002)
internal/queue/      * Serialised, prioritised, coalescing device access (002)
internal/lcd/        * set screen argv and the dashboard render (003)
internal/scene/      * Scene model, validation, shipped defaults (005)
internal/config/       Config file on fynedesygn's settings package
internal/service/    * The operations, in-process: validation, resolution,
                       health. The daemon's core; no transport, no view
internal/api/        * The HTTP/JSON contract: request and response types, the
                       route table, and the client every shell uses
internal/daemon/       The resident half: desired state, reconciliation loops,
                       the HTTP server, and later the thin D-Bus door (006)
internal/version/      Stamped by the Makefile from .tag
```

`internal/service` follows the terrariabonker/jira-viewer dual-shell pattern: one
view-neutral core, with shells as adapters, so a capability cannot exist in one
shell and not the other. The difference here is that the boundary is not
optional: the shells reach it only through `internal/api`'s client, across the
socket. That is the same boundary shape terrariabonker uses, with an HTTP socket
in place of sudo and for a different reason — not privilege, but single
ownership of the hardware.

## Requirements

1. A library core with no UI: connect to an OpenRGB server, list devices with
   their modes and zones, resolve an intent to a mode a given device supports,
   and apply a colour. Every consumer goes through it.
2. Devices are addressed by name. Indices are resolved per operation and never
   persisted.
3. An intent (`solid`, `off`, a named effect) resolves against each device's own
   reported mode list. A device that cannot express it is skipped with a reason,
   not failed over or forced.
4. Lighting addresses targets — a device, a zone, an LED range, or a named
   segment — not devices alone. Assignments for a device compose into one frame
   before any write, and a device that cannot express a multi-colour frame is
   reported rather than approximated.
5. Device rules — scope, direct-over-static, never-blanked, asserted brightness,
   reassert interval, named segments — are configuration matched on name, not
   compiled in. With no configuration at all, scope is every device present and
   every default is derived from what the hardware reports.
6. A fresh install is inert: with no recorded desired state, the service starts
   and discovers but writes to no device until asked, and reconciliation
   reasserts only what hotaru itself was told to set.
7. Every backend and capability is independently optional **at runtime**. An
   absent one is reported as absent and removed from the interface; it never
   fails an unrelated operation, and it never prevents the service from
   starting. Packaging is a separate claim: the Arch packages depend on all
   three backends, so a fresh install works without reading anything, while the
   program still degrades because daemons stop and other install routes exist.
8. Applying reports per device what happened: applied with which mode, skipped
   with a reason, or failed. An operation that changed nothing does not report
   success.
9. Health is distinguishable and reportable: server unreachable, up with no
   devices, up with no devices matching scope, healthy.
10. The CLI covers list, apply and health, with machine-readable output.
11. Writes coalesce per device so rapid repeats converge on the last request.
12. The CLI and the GUI expose the same capabilities, one for one. A capability
   reachable from one shell and not the other is a test failure.
13. Configuration is YAML on disk, split by owner: a user-owned rules file the
    program never rewrites, a machine-written scenes file, machine-owned state
    outside the config directory, and a GUI file the service never reads.
14. No credentials and no privileged operations. Network access is limited to
    the configured OpenRGB server and, from spec 002, the OpenLinkHub daemon —
    both localhost by default.
15. The migration contract above is binding on specs 002-006: the monitor's
    write-path removal is one commit at cutover, preceded by the release
    verification and the clearing of the stale key entries.
16. The service's snapshot carries observed per-device state — present, in
    scope, active mode as reported, last applied colour and when, whether a rule
    re-asserts it, whether the last write landed — so a view renders it rather
    than querying hardware. The cooler snapshot carries raw measured values — pump rpm, coolant and CPU
    temperature, and the staleness of each — rather than only the fields
    hotaru's own views need, so a consumer can compute its own alerting from it.
17. Service management — status, health, reconcile now, reload — is reachable
    from both shells, not only from the GUI, and has a machine-readable form.
18. The service is the only writer of hardware. A shell holds no device handle
    and has no direct-write path once the service exists.
19. Previewing, applying and reverting are distinct operations on the service
    interface. A preview never becomes desired state, and an unconfirmed
    preview leaves nothing behind.
20. JSON output is an interface, not a convenience. Its shape is stable enough
    for the monitor's future Go rewrite to consume `hotaru cooler status --json`
    as its telemetry source, and a change to it is a breaking change.
21. The CLI is an API client from its first commit. No shell ever contains a
    direct-write path, so none has to be removed later.
22. Every behaviour carried over from the Python cites the fact that makes it
    necessary, and every cited fact has a named test. A carried behaviour that
    can cite neither is removed rather than preserved.
23. The SDK-candidate packages are written as if already public: no global
    state, no output, no exits, errors returned, `context` on device and socket
    calls, and no CLI or GUI concept below `cmd/`.

## Acceptance Criteria

- [x] `go.mod` is `github.com/ushineko/hotaru`, Go 1.26.0, no `toolchain` line.
- [x] An OpenRGB SDK client connects to a configured host/port, lists devices
      with name, type, modes, active mode and zones, and sets a device's mode
      and colour, with no subprocess and no output parsing.
- [x] Duplicate device names from a rescanned server collapse to the first
      occurrence.
- [x] Devices are addressed by name throughout; no index is written to disk or
      carried between operations.
- [x] Mode resolution returns the device's own spelling of the chosen mode, or
      nothing when the device cannot express the intent, and the nothing case is
      a skip-with-reason at the call site rather than an error.
- [x] A target resolves as device, `device/zone`, `device/zone[a:b]` or
      `device/segment-name`, and a whole-device target is the same code path
      filling every zone.
- [x] Named segments are read from device rules and are what scenes reference;
      an unknown segment name is reported with the names that do exist, and the
      rest of the scene still applies.
- [x] Assignments compose into one frame per device before any write: a test
      asserts that a three-assignment scene produces exactly one write per
      device, and that a later assignment overrides an earlier one on the same
      LEDs.
- [x] A frame with more than one distinct colour resolves against a mode that
      accepts per-LED data; a device with no such mode is reported in the
      per-device output, and its colours are never averaged or reduced to one.
- [x] `hotaru light probe` reports zones and LED counts. **Walking a zone one
      LED at a time moved to spec 005**: it is a conversation, not a report, and
      belongs with the wizard that asks the questions.
- [x] A partial assignment composes onto what hotaru last wrote, falling back to
      what the device reports and then to black — never onto a device's claim
      when a better answer is remembered.
- [x] With no config file at all, every device OpenRGB reports is in scope, and
      a test with an invented device list asserts it.
- [x] Solid resolves static-first by default; a rule can invert it to
      direct-first; the shipped example config inverts it for Aura/Maximus and
      is not loaded unless the user adopts it.
- [x] A write is read back: a device that accepts a mode without taking it falls
      through to the next candidate, and the outcome is remembered for the
      session. A test drives a fake device that lies about a mode.
- [x] `hotaru light probe` reports what each present device advertises, what
      actually took, and the rules it would suggest — and writes nothing unless
      asked.
- [x] A fresh install writes to no device: a test asserts that a service started
      with empty state performs no write, and that reconciliation with nothing
      recorded is a no-op rather than an assertion of a default.
- [x] The second-machine test passes on hardware the author does not own, with
      no hand-written configuration. It is run before the version that claims
      lighting support is tagged.
- [x] A rule can mark a device never-blanked: it is skipped for `off` and still
      receives colour scenes.
- [x] A rule can assert a brightness level on every write.
- [x] Config is YAML (`.yml`) read through fynedesygn `settings` with
      `settings/yamlcodec`, from `$XDG_CONFIG_HOME/hotaru/`, seeded per key on
      first run, and a user-edited key is never overwritten.
- [x] `hotaru.yml` is never written by the program: a test asserts that a round
      trip through load-and-save leaves a user's file, comments included, byte
      for byte unchanged — because it is never saved at all.
- [x] Desired state is written to `$XDG_STATE_HOME/hotaru/`, not to the config
      directory, and a machine with no state file writes to no device.
- [x] A malformed rule costs that entry only: it is reported with what is wrong
      and skipped, and the rest of the file still loads.
- [x] `hotaru light list` prints devices with their modes, marking the active
      one and which are in scope.
- [x] `hotaru light set <colour> [--devices ...]` applies a colour, printing one
      line per device: mode used, skipped with reason, or failed.
- [x] `hotaru light health` distinguishes server unreachable, up with no
      devices, up with no in-scope devices, and healthy, exiting non-zero for
      the first three, and reports the negotiated protocol version.
- [x] Each unhealthy state names what to do about it, and distinguishes "OpenRGB
      is not installed" from "installed but not running" from "running but
      enumerated nothing" — three different remedies, not one error.
- [x] Any remedy hotaru can perform — starting the OpenRGB server, enabling its
      own user service, restarting a server that enumerated a partial list — is
      offered explicitly and performed only on an answer. A test asserts that no
      such action happens without one.
- [x] `--json` on every listing and health command emits a stable shape.
- [x] With the server unreachable, every command fails in under a second with
      one line naming the address it tried. Nothing hangs.
- [x] Per-device coalescing: rapid repeated writes to one device converge on the
      last, verified with a fake client.
> **Preview moved to spec 004.** The three criteria below are unchanged and
> unmet; they are listed there instead, because preview only means something
> once scenes exist to preview and the editor that needs it is built on them.
> The machinery they depend on — desired state, reconciliation, per-device
> queues — is finished here, so spec 004 adds semantics rather than mechanism.
>
> - The service interface distinguishes preview from apply, and a test asserts
>   that a preview followed by a revert leaves desired state untouched.
> - A preview suspends reconciliation for the devices it covers and resumes it
>   when the preview ends.
> - A preview is a lease bound to its caller: a caller going away restores
>   desired state without having said so.
- [x] `hotaru serve` listens on a Unix socket in `$XDG_RUNTIME_DIR`, serves
      `/v1`, and survives every backend being absent.
- [x] The user unit has no `graphical-session.target` dependency, and the
      service starts and serves with no session at all — verified by starting it
      with no desktop running.
- [x] hotaru reports when lingering is not enabled, explains that boot-time
      restore needs it, and offers to enable it. It never enables it silently,
      and the package never enables it.
- [x] A restore that reaches fewer devices than the recorded state names is
      reported as incomplete, retries with backoff, and completes without
      further instruction when the devices appear.
- [x] The unit declares no `After=`, `Requires=` or `Wants=` on any OpenRGB
      unit: the server is a resource that appears, and neither its absence nor
      its name can prevent hotaru from starting.
- [x] Readiness is judged by the device list, not by the socket accepting — a
      server that answers while reporting fewer devices than recorded state
      names is "not ready yet", not "healthy".
- [x] Where OpenRGB is installed but not running, hotaru detects which unit
      exists (system `openrgb.service`, a user unit, or neither) and offers to
      start it rather than naming one.
- [x] The CLI depends on `internal/api`'s client and on no device package — a
      test asserts the import graph, so a direct-write path cannot appear by
      accident.
- [x] Every `/v1` endpoint has a recorded one-off client transcript — the
      request, the response, and what it establishes — kept as the worked
      example in the API documentation and reused as the test fixture.
- [x] With the service not running, a CLI write fails with one line saying so
      and a non-zero exit; `--direct` performs read-only queries only.
- [x] Every operation on `internal/service` is reachable from `cmd/hotaru`, and
      a test enumerates the service surface and fails on one the CLI cannot
      reach. The same test covers the GUI when spec 004 lands.
- [x] Tests run headless with no OpenRGB server and no hardware, against a fake
      client, covering mode resolution for facts 1, 3 and 4, duplicate
      collapsing, config merge and coalescing.
- [x] Tests include machine profiles that are not this desk: one device, no
      devices, only unknown mode names, an OpenRGB that is up but empty. Each
      asserts what a stranger sees, not what this author sees.
- [x] Every absent-backend path is covered: server unreachable, server empty,
      and (from 002) no liquidctl, no cooler, no LCD, no OpenLinkHub. In each
      case unrelated operations still work and the service stays up.
- [x] At least one test exercises a real OpenRGB server when one is reachable
      and skips cleanly when it is not — the integration boundary is the wire
      protocol, which a fake cannot fail the way a real server can.
- [x] No test is a translation of a Python test. Each is written against the Go
      structure, and the Python suite is mined for hardware assertions only.
- [x] Each device quirk carried from the Python has a test named for it —
      `TestGPURejectsStatic`, `TestAuraNeedsDirect`, `TestKeychronNeverBlanks` —
      so a hardware or upstream change fails one named test rather than
      silently changing behaviour.
- [x] No package reimplements the bounded-drop queue: per-device serialisation
      is a goroutine with a single-slot mailbox, and a test asserts that N rapid
      requests to one device produce one write of the last value, with nothing
      dropped silently.
- [x] The service listens on a Unix socket only; a test asserts no listener is
      created on any network address.
- [x] No SDK-candidate package holds mutable package-level state, prints, or
      calls `os.Exit`; a test asserts it rather than a review catching it.
- [x] Every device and socket call takes a `context.Context` and honours
      cancellation, including the connect.
- [x] `hotaru light list --json` and `light health --json` shapes are documented
      in the README as an interface, with the note that they are consumed by
      other programs and changing them is breaking.
- [x] `go test ./...` passes with no hardware, no OpenRGB, no liquidctl and no
      root, because that is what lets a PKGBUILD's `check()` run it.
- [x] `make test`, `make lint` and `govulncheck ./...` pass.
- [x] README covers what it is, what it needs (a running OpenRGB server; from
      002, liquidctl), the commands, and the config file with example rules.
- [x] The migration contract, the cutover order and the KDE hotkey rules are
      carried into `docs/migration.md` so specs 002-006 do not have to rediscover
      them.

## Risks & Assumptions

- **An unauthenticated local API is a deliberate choice**, and the Unix socket
  is what earns it: the boundary is the filesystem, the audience is one user's
  session, and the socket is created with permissions that say so. This is the
  reason there is no port — a listener would need authentication to be defensible,
  and the feature does not justify the mechanism.
- **The LCD accepts a file path from a client**, so the service reads whatever
  path a scene names. That is a local file read performed by the user's own
  session on the user's own behalf, but it is the one place an API request turns
  into filesystem access, and it deserves saying: paths are used as given, not
  expanded from untrusted templates, and a path that is not an image fails
  rather than being passed to a subprocess as an argument that might be read as
  a flag.
- **A resident service is a new always-on process** holding a socket to OpenRGB
  and invoking liquidctl on a timer. It restarts on failure, and it must degrade
  rather than spin: a downed OpenRGB produces one warning, not one per
  reconciliation tick (spec 037 made that an explicit criterion after the
  60 s timer proved noisy).
- **Reconciliation intervals are guesses against unmeasured hardware.** 60 s for
  the mouse sits under its ~5-minute sleep timeout with margin; the real wake
  behaviour was never characterised. The interval is per rule and in config for
  exactly that reason.
- **Reconciliation can fight another controller.** If OpenLinkHub or OpenRGB's
  own GUI is also driving a device, hotaru re-asserting every 60 s turns a
  one-off disagreement into a visible oscillation. The MM700 is the known case
  — it is managed by both OpenLinkHub and OpenRGB — and the existing remedy is
  to exclude it there so OpenRGB owns it.
- **The SDK is a direction, not a commitment.** Writing the core as if public
  costs little; promoting it out of `internal/` before the monitor's rewrite
  exists would create an API with no consumer to shape it. The risk being
  managed is the opposite one — a core so entangled with the CLI that promotion
  means a rewrite — and the constraints in "Later" are what manage it.
- **Over-fitting is the likeliest way this program is bad for someone else**,
  and it is invisible from here: every test passes on the machine that has the
  hardware. The countermeasures are the invented machine profiles, the
  default-to-everything scope, and — the only one that is real evidence — the
  second-machine test on hardware the author does not own.
- **Coexisting with an OpenRGB that something else manages.** On the second
  machine OpenRGB already holds the user's own configuration. hotaru must be a
  well-behaved second client of it: it does not save profiles, does not assume
  it started the server, and does not assert anything it was not asked for. The
  inert-on-install rule is what makes that true rather than hoped for.
- **Read-back-and-fall-through is more device traffic** than trusting a write,
  and on a device whose active mode is slow to report it costs a round trip per
  write. It is worth it: it is the mechanism that replaces this desk's quirk
  table with something a stranger's hardware can drive.
- **Rearchitecting costs more than translating, and hides different bugs.** A
  faithful port would carry the Python's faults intact but would at least fail
  the same way it does today. Rebuilding means the failures will be new ones.
  The mitigation is that the *facts* are not being rederived — only the
  structure — and that each fact arrives with a test.
- **This is a large migration.** Eighteen specs' worth of hard-won behaviour
  moves across a language boundary. The roadmap exists so the monitor is not
  broken before hotaru can replace it; the cutover in 005 is the only
  irreversible step, and it is a revert away from returning.
- **OpenRGB protocol compatibility.** Speaking the SDK binds hotaru to a
  protocol version rather than a CLI's output format. A client that lags a
  server release is a failure mode the subprocess did not have. Health reports
  the negotiated version; the integration test catches drift.
- **Third-party client vs hand-rolled.** `github.com/csutorasa/go-openrgb-sdk`
  (v1.0.1, Feb 2026, MIT) is the most recently maintained of four Go clients.
  Taking it follows the standing "prefer a library over hand-rolled
  infrastructure" decision; the protocol is small enough to vendor if it goes
  unmaintained, and the `internal/openrgb` interface keeps that swappable.
- **liquidctl remains a runtime dependency** (spec 002), invoked as an explicit
  argv subprocess. It is Python, it is the reference implementation for this
  cooler, and `set_screen` is marked Unstable upstream. hotaru is a Go program
  that shells to Python, which is worth stating plainly rather than discovering.
- **Detection still happens once.** A correctly ordered OpenRGB server still
  will not notice a device plugged in later. Health makes it visible; the remedy
  remains restarting the unit.
- **The dashboard render must match what is there now closely enough** that the
  screen does not become a downgrade. It is a 640x640 layout with fonts; Go's
  text rendering is not Qt's, and pixel parity is not the goal — legibility at a
  glance is.
- **Two readers on one hidraw node, deliberately.** After cutover the monitor
  keeps polling `liquidctl status` every 5 s while hotaru reads and writes. This
  is accepted as a temporary state on the maintainer's call, and it is worth
  naming what is being accepted: this machine has a documented history of hidraw
  contention (logid, solaar and `battery_reader` sharing a node produced 25 s
  stalls), and `set_screen` is Unstable upstream with known bucket-switch
  failures under repeated writes. The symptoms to watch for are stalled polls
  and LCD writes that report success and do not land. If it does bite, the
  remedy is to bring the monitor's telemetry forward onto `hotaru cooler status
  --json` sooner rather than to re-split ownership.
- **Concurrent writers during development.** Before cutover the monitor still
  pushes the dashboard; the working rule is to stop it before exercising
  hotaru's writes.
- **Rollback**: hotaru is additive until the cutover commit. After it, rollback
  is reverting that commit in the monitor's repository and bouncing it — the
  monitor's code is unchanged underneath, so it resumes.
- **Security**: no credentials, no privileged operations. Network access is the
  configured OpenRGB server and the OpenLinkHub daemon, both localhost by
  default. The OpenRGB SDK has no authentication, which the README must say
  plainly for anyone pointing it at a non-local host.

## Alternatives Considered

- Considered porting the subprocess OpenRGB design as-is; rejected because the
  parsers, the silent 9-second fallback and the name-length rule are artefacts
  of the CLI and vanish with it.
- Considered leaving cooler telemetry and the LCD in the monitor and taking only
  lighting; rejected by the ownership decision, and independently unworkable:
  the dashboard is a render of the telemetry, so splitting them puts two
  processes on one hidraw node.
- Considered reimplementing the Kraken's HID protocol in Go to drop liquidctl;
  rejected as a large reverse-engineering project with an upstream that already
  works, and `nzxt-kraken3` not matching this device is a warning about how much
  is model-specific.
- Considered numbered scene slots; rejected in favour of names, with slots
  pushed to the binding layer where the numbers come from.
- Considered the XDG `org.freedesktop.portal.GlobalShortcuts` portal instead of
  KWin scripting; not rejected, but not chosen. It is the portable answer and
  would drop the whole load/unload/object-id hazard, at the cost of a mechanism
  nothing on this desk has proven, and of bindings configured in System Settings
  rather than in hotaru. Worth a spike in spec 006; the KWin path is known to
  work.
- Considered keeping a lighting fallback in the monitor; rejected — two programs
  that can both drive the devices and both claim the keys is the failure this
  migration exists to end.

## Not in this spec

Specs 002-006 as tabled above. Also out of scope throughout: effects beyond
selecting a device's own named effect modes, fan and pump duty control (the
firmware silently discards those writes — see the runbook), and packaging
(PKGBUILD, desktop entry, the systemd user unit's new home).

## Open questions for review

1. ~~Does the monitor keep any AIO display?~~ **Decided**: it keeps its
   read-only telemetry display, reading from its own sources for now, and
   becomes a consumer of `hotaru cooler status --json` when it is rewritten in
   Go. hotaru's `glance` panel (spec 004) is built anyway — two views of the same
   cooler is the accepted cost of not blocking one program on the other.
2. ~~Where does pump-failure alerting live?~~ **Decided**: it stays in the
   monitor. **An alert is owned where it is visible.** The monitor is the tray
   widget the user actually sees, so it owns the alert; hotaru owning an alert
   nobody is looking at would be ownership in name only. It moves down into
   hotaru at the Go rewrite, with everything else.

   Two consequences carried forward. The removal in this migration leaves the
   alert and the telemetry it reads intact — the alert is not a write and is not
   part of the cutover. And hotaru's snapshot must carry the raw values the
   alert is computed from (pump rpm, coolant temperature, their staleness), not
   only a rendered alert state, so the move is a change of source rather than a
   change of shape.

   The daemon-mode question that this left open is now settled on its own
   merits — see "hotaru runs as a user service" — and the alert moving later
   does not depend on it.
3. **Client dependency or vendored protocol** — see Risks.
4. ~~Who writes the config file once the GUI can save scenes?~~ **Decided**:
   the service, and never the GUI. The GUI has its own file for its own view
   state, and reaches everything else through the API. See "Files on disk".
5. ~~Config format.~~ **Decided**: YAML, `.yml`, via `settings/yamlcodec`.
6. **Where does the mapping wizard write what it learns?** Segments are rules,
   and `hotaru.yml` is the user's file, which the program never rewrites — so
   the wizard cannot simply save into it. Three options: print the YAML for the
   user to paste, which is honest and makes a GUI wizard end in a copy-paste;
   write a machine-owned `learned.yml` in the config directory that is merged
   underneath the user's rules, with the user's winning; or relax the
   never-write rule for a file the user was explicitly editing through the GUI.
   The middle one is the recommendation: it keeps the promise about the
   hand-written file exactly, and it lets the GUI save a mapping without asking
   someone to edit YAML — which is the whole reason the wizard exists.
6. **Who edits the bindings?** The Python hardcoded `Ctrl+Alt+Num+N` from a
   template. hotaru could keep that, or make the binding part of each scene's
   config entry — more flexible, and one more thing that can claim a taken key.
