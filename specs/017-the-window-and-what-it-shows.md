# Spec 017: the window, and what it shows

**Issue**: [#5](https://github.com/ushineko/hotaru/issues/5)

## Status: COMPLETE

## Context

Everything the GUI needs from the service now exists. Scenes are named and
stored, previews are leases that end with their holder, the keys are bound
through a door the service owns, and health, cooling and the device list have
been answerable since spec 001. What is missing is the picture.

**Granular addressing is what a configuration file is bad at and a picture is
good at.** `kraken/ring[12:23]` is a thing somebody can type and nobody wants
to; it is also a thing that has to be *worked out* first, by lighting LEDs and
looking at the case. The wizard does that by asking questions in a terminal.
The GUI is where it becomes pointing at a fan.

That is spec 018's job. This spec builds the window it will live in, and the
view it will be an editor for.

### Read-only, on purpose

Nothing in this spec writes to a device. Not because writing is hard -- the API
takes care of that -- but because a control surface that cannot yet show the
machine honestly has no business changing it, and every failure in this project
so far has been a write that reported success while doing something else.

So: the skeleton, service management, a System view that draws what is there,
and the cooler's numbers beside them. Then an editor, on a window that already
works.

### What the System view is for

The Python could express a machine's lighting as a context menu of device
names. That is the whole distance being travelled: a menu says *NZXT Kraken
2024 ELITE Series RGB*, and a picture says *this device has two zones, the ring
is twenty-four lights and the fans are twenty-four more, both are in scope, it
is in Direct, and the ring is showing this purple right now*.

Every fact in that sentence is already on the wire. The view is an arrangement
of `GET /v1/devices`, and its value is entirely in being a picture rather than
a table.

### There is no glance panel

Spec 001 listed one -- a small always-on-top window for the cooler, the second
window archetype -- and this spec built it before somebody asked the obvious
question: **that is the monitor's job.**

`peripheral-battery-monitor` is the always-on-desktop widget on this machine.
It shows batteries, bandwidth and the cooler already, it is the thing somebody
glances at, and its Go rewrite is meant to become a consumer of this API. Two
always-on-top cooling panels on one desktop is a duplication nobody asked for
and one of them would go.

So the cooler's numbers are a **section in the window**, beside the devices and
the service, where a control surface wants them. The desktop panel stays where
it already is.

### The sections say nothing they do not have to

fynedesygn's `Heading` takes a title and a blurb, and every section in every
program built on it has one. These do not. A line under "The service"
explaining what a service is tells somebody looking at their own machine
something they knew before they opened the window, and it is read once per
glance while competing with the controls under it.

The rule is the design system's own -- one short statement, purpose first,
no second clause explaining the first -- and it was broken three times writing
this before being applied. The facts are the content.

### It polls, and says that it does

`/v1/events` is in the architecture and is not built. Until it is, the window
polls -- on a timer, only while it is on screen, and stopping when it is not.
This is stated rather than hidden because a GUI that polls a service that polls
hardware is two intervals somebody will eventually have to reason about.

### One writer owns the files

The service owns `hotaru.yml` (the user's, never rewritten), `scenes.yml` and
the desired state. The GUI owns nothing but its own view state: window
geometry, colour scheme, the section it was last on. **The service never reads
the GUI's file and the GUI never writes the service's.** Spec 001 settled this;
it is repeated here because the editor in spec 018 is exactly where somebody
would be tempted to break it.

## Requirements

**R1. The GUI is a client.** Every fact it shows and every action it offers is
an API request over the socket. It holds no device handle, imports nothing that
can reach hardware, and a test says so the way the CLI's does.

**R2. It is `hotaru-gui`, built on fynedesygn's `shell`.** Sections down the
side, one content scroller, a status bar, and the shell's own settings store
for view state in `gui.yml`.

**R3. Service management.** Whether the service is reachable, what it reports
through `health`, what it can see, and a way to make it reconcile now. A
service that is not running is the ordinary first-run case and says what to do
about it rather than showing an error.

**R4. A System view that draws the machine.** Every device, its zones as
proportional blocks, the colours each is currently showing, whether it is in
scope, what mode it is in, whether it is being re-asserted, and whether
somebody is holding a preview on it.

**R5. The cooler is a section, not a panel.** Coolant, pump and fans beside the
devices and the service. The always-on-desktop display is
`peripheral-battery-monitor`'s, and duplicating it here would give the machine
two of them.

**R6. Nothing here writes to a device.** Reconcile is the one operation that
changes hardware and it asks the service to restore what was already wanted --
it introduces no new state.

**R7. It polls only while on screen**, and stops when the window is not
showing. A background window that keeps a service busy is a background window
somebody will close for the wrong reason.

**R7a. No exposition.** A section is its name and its facts. The house style
for comments and specs is expository; the interface is not.

**R8. The tests run headless**, through the Fyne test driver, with no display
and no service.

## Acceptance Criteria

- [x] AC1. `hotaru-gui` opens, with Service, System and Cooling sections.
- [x] AC2. The GUI package imports nothing that can reach a device, asserted by
      a test over the import graph.
- [x] AC3. With no service running, the window opens and says so, with the
      command that starts it.
- [x] AC4. The Service section shows health, the device count in scope, the
      rules file in use, and reconciles on request.
- [x] AC5. The System view draws each device with its zones proportional to
      their LED counts, in the colours the device reports.
- [x] AC6. A device out of scope is drawn as out of scope rather than omitted.
- [x] AC7. A device under a preview is marked, with who holds it.
- [x] AC8. The Cooling section shows coolant, pump and fans, and says there is
      no cooler when there is not one -- as a fact rather than as a failure,
      because most machines are that machine.
- [x] AC9. Polling stops when the window is off screen.
- [x] AC10. The GUI's settings file holds view state only, and the service's
      files are untouched by it.
- [x] AC11. Tests run headless with no display and no socket.
- [x] AC12. Verified on the development machine: the window draws its six
      devices with their real colours and the cooler's numbers match `hotaru
      cooling`.

## Verified on hardware

Development machine, Plasma on Wayland, with somebody looking at the window.

It draws the six devices with the colours on the desk, the cooler's numbers
match `hotaru cooling`, and `gui.yml` appeared in hotaru's configuration
directory holding a navigation split and nothing else -- `hotaru.yml` and
`scenes.yml` keeping the timestamps they had before the window ever ran, which
is the one-writer rule holding rather than being asserted.

Two things came from somebody seeing it rather than from a test. The glance
panel was built, shown, and removed: it had a titlebar Plasma would not give
up without a window rule, and the question that followed -- *why do we need
this, the monitor does it* -- was the better one. And the section blurbs went,
because prose under a heading is read once per glance and competes with what it
sits above.

## Risks & Assumptions

- **Fyne needs cgo and a graphics stack**, which the service deliberately does
  not. That is why `hotaru-gui` is a separate binary and a separate package in
  `docs/packaging.md`; a machine that installs only `hotaru` is unaffected.
- **Polling is a placeholder for an event stream.** The interval is a guess
  until somebody watches it; the mitigation is that it stops when the window is
  not on screen, so the cost is bounded by somebody looking at it.
- **Drawing is a first attempt.** A device's zones are a row of blocks here;
  what the case actually looks like is not something the protocol reports, and
  guessing at physical arrangement is the over-fitting this project avoids.
- **Rollback** is not shipping the binary: nothing in the service changes.

## Alternatives Considered

Considered making this an editor from the start; rejected because the editor
needs a window that is known to show the truth, and this project's whole
history is writes that looked fine and were not.

Considered a TUI rather than a GUI; rejected because the point is a picture, and
the CLI already covers the terminal case completely.

Considered the glance panel spec 001 asked for, and built it: a frameless
always-on-top card of coolant, pump and fans. Removed before it shipped.
`peripheral-battery-monitor` is that window on this desktop and its Go rewrite
consumes this API, so hotaru would have been shipping a second one. The window
archetype is right and the program carrying it is the monitor.
