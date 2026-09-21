# hotaru architecture

The shape of the system as decided so far. The source of record for *why* any of
it is this way is `specs/001-scope-migration-and-lighting-core.md`; this page is
the picture, and it is updated in the same commit as any decision that changes
it.

## The system

```mermaid
%%{init: {"theme":"base","themeVariables":{
  "background":"#202326",
  "primaryColor":"#292c30","primaryTextColor":"#fcfcfc","primaryBorderColor":"#3c4045",
  "secondaryColor":"#1d1f22","tertiaryColor":"#141618",
  "lineColor":"#a1a9b1","textColor":"#fcfcfc","titleColor":"#fcfcfc",
  "clusterBkg":"#141618","clusterBorder":"#3c4045",
  "edgeLabelBackground":"#202326",
  "nodeTextColor":"#fcfcfc",
  "fontFamily":"Noto Sans, Segoe UI, sans-serif","fontSize":"14px"
}}}%%
flowchart TB
    subgraph inputs["Ways in"]
        KEY["KDE global shortcut<br/>Ctrl+Alt+Num N"]
        GUI["hotaru-gui<br/>Fyne on fynedesygn"]
        CLI["hotaru CLI"]
        DEV["throwaway clients<br/>curl · 20 lines of Go"]
    end

    KWIN["KWin script<br/>registerShortcut + callDBus<br/>installed by the service"]

    subgraph svc["hotaru serve — the only actor on the hardware"]
        DBUS["D-Bus object<br/>thin: hotkey activations only"]
        API["HTTP/JSON API<br/>unix socket in XDG_RUNTIME_DIR<br/>/v1/... + /v1/events stream"]
        CORE["Service core<br/>operations, validation, health"]
        STATE["Desired state<br/>one frame per device + LCD<br/>preview leases held apart"]
        RECON["Reconcilers<br/>reassert per device rule<br/>dashboard render + push gate<br/>LCD keepalive"]
        MBOX["Per-device mailboxes<br/>one goroutine each<br/>single slot: latest frame wins"]
        CFG["Config + state<br/>hotaru.yml (user, read-only)<br/>scenes.yml · state.yml"]
    end

    subgraph backends["Backends"]
        ORGB["OpenRGB server<br/>SDK protocol, TCP 6742<br/>systemd --user, enumeration gate"]
        COOL["Cooler driver · in hotaru<br/>/dev/hidraw + usbfs<br/>NZXT protocol, no cgo"]
        HWMON["Kernel sensors<br/>/sys/class/hwmon by label<br/>nvidia-smi where there is none"]
    end

    subgraph hw["Hardware"]
        LIT["Lit devices<br/>Kraken · GPU · Aura · MM700 · G502 · Keychron"]
        LCD["Kraken LCD<br/>640x640, GIF only"]
        TELEM["Cooler telemetry<br/>coolant · pump · fans"]
        TEMP["CPU package · GPU"]
    end

    PBM["peripheral-battery-monitor<br/>batteries, bandwidth,<br/>read-only AIO display + pump alert"]
    FUTURE["Go monitor rewrite<br/>imports the client package"]

    KEY --> KWIN
    KWIN -->|"Apply scene N"| DBUS
    GUI -->|"stage · preview · save · bind"| API
    CLI -->|"list · set · probe · health"| API
    DEV -.->|"curl --unix-socket<br/>one-off clients, early"| API

    DBUS --> CORE
    API --> CORE
    CORE --> STATE
    CORE --> CFG
    STATE --> RECON
    RECON --> MBOX
    CORE --> MBOX

    MBOX -->|"mode + frame<br/>per zone / per LED"| ORGB
    RECON -->|"dashboard frames<br/>hash gate + size floor"| COOL
    CORE -->|"status, coalesced"| COOL
    CORE -->|"package + card temps"| HWMON

    ORGB --> LIT
    COOL --> LCD
    COOL --> TELEM
    HWMON --> TEMP

    PBM -.->|"temporary: its own liquidctl<br/>until the Go rewrite"| TELEM
    FUTURE -.->|"GET /v1/cooling<br/>no device handle"| API

    classDef svcnode fill:#292c30,stroke:#3daee9,color:#fcfcfc
    classDef backend fill:#1d1f22,stroke:#3c4045,color:#fcfcfc
    classDef hwnode fill:#141618,stroke:#3c4045,color:#a1a9b1
    classDef future fill:#1d1f22,stroke:#a1a9b1,stroke-dasharray:5 5,color:#a1a9b1
    classDef entry fill:#292c30,stroke:#3c4045,color:#fcfcfc

    class DBUS,API,CORE,STATE,RECON,MBOX,CFG svcnode
    class ORGB,COOL,HWMON backend
    class LIT,LCD,TELEM,TEMP hwnode
    class KEY,GUI,CLI,KWIN entry
    class DEV future
    class FUTURE,PBM future
```

Colours are Breeze Dark, the same palette `fynedesygn` ships as its default
scheme, so the diagram matches the program it describes. It renders dark
whatever the viewer's theme is — deliberate, since that is how the work is read.

**Solid** is the architecture after cutover. **Dashed** is either temporary (the
monitor reading the cooler for itself until its Go rewrite) or not yet built
(the CLI's direct path exists only until the service does; the Go monitor is a
direction, not a commitment).

## Boot and readiness

The service starts with the machine. Nothing below waits for a login, and
nothing treats "the unit started" as "the hardware is there".

```mermaid
%%{init: {"theme":"base","themeVariables":{
  "background":"#202326",
  "primaryColor":"#292c30","primaryTextColor":"#fcfcfc","primaryBorderColor":"#3c4045",
  "secondaryColor":"#1d1f22","tertiaryColor":"#141618",
  "lineColor":"#a1a9b1","textColor":"#fcfcfc","titleColor":"#fcfcfc",
  "clusterBkg":"#141618","clusterBorder":"#3c4045",
  "edgeLabelBackground":"#202326","nodeTextColor":"#fcfcfc",
  "fontFamily":"Noto Sans, Segoe UI, sans-serif","fontSize":"14px"
}}}%%
flowchart TB
    BOOT(["boot · user manager, lingering"]) --> SERVE

    SERVE["<b>Serving</b><br/>API up from this moment —<br/>no state below ever blocks it"]
    SERVE -->|"no recorded state"| INERT["<b>Inert</b><br/>nothing to restore,<br/>nothing written"]
    SERVE -->|"state recorded"| WAIT

    WAIT["<b>Waiting for OpenRGB</b><br/>health: server unreachable"]
    WAIT -->|"no server · retry with backoff"| WAIT
    WAIT -->|"connected, device list short"| PART
    WAIT -->|"connected, every recorded<br/>device present"| REST

    PART["<b>Partial</b><br/>fewer devices than recorded —<br/>reported incomplete, never done"]
    PART -->|"retry with backoff"| PART
    PART -->|"the rest enumerate"| REST
    PART -->|"user restarts the server"| REST

    REST["<b>Restoring</b><br/>one frame per device,<br/>read back to confirm"] --> STEADY

    STEADY["<b>Reconciling</b><br/>steady state"]
    STEADY -->|"re-assert per device rule"| STEADY
    STEADY -->|"server went away"| WAIT
    INERT -->|"first scene applied by hand"| REST

    classDef ok fill:#292c30,stroke:#3daee9,color:#fcfcfc
    classDef wait fill:#1d1f22,stroke:#f67400,color:#fcfcfc
    classDef idle fill:#1d1f22,stroke:#3c4045,color:#a1a9b1
    classDef entry fill:#141618,stroke:#3c4045,color:#a1a9b1
    class SERVE,REST,STEADY ok
    class WAIT,PART wait
    class INERT idle
    class BOOT entry
```

**Why there is no "wait for OpenRGB" edge at the start.** hotaru declares no
`After=` or `Requires=` on any OpenRGB unit. There is no single unit to name —
the `openrgb` package ships a system one, this machine runs a user one, other
people start it by hand — and ordering would not help regardless: a cold boot
has been observed reaching `Started` with two devices of six enumerated, because
OpenRGB detects once and USB enumeration had not finished. Readiness is judged
by the device list, which is why **Partial** is a state of its own rather than a
successful restore with fewer lights than expected.

## Session attachment

The desktop is not a precondition for anything. It is something that shows up,
possibly more than once.

```mermaid
%%{init: {"theme":"base","themeVariables":{
  "background":"#202326",
  "primaryColor":"#292c30","primaryTextColor":"#fcfcfc","primaryBorderColor":"#3c4045",
  "secondaryColor":"#1d1f22","tertiaryColor":"#141618",
  "lineColor":"#a1a9b1","textColor":"#fcfcfc","titleColor":"#fcfcfc",
  "clusterBkg":"#141618","clusterBorder":"#3c4045",
  "edgeLabelBackground":"#202326","nodeTextColor":"#fcfcfc",
  "fontFamily":"Noto Sans, Segoe UI, sans-serif","fontSize":"14px"
}}}%%
flowchart LR
    NOSESS["<b>No session</b><br/>service running,<br/>no desktop"]
    BOUND["<b>Bound</b><br/>KWin script installed,<br/>D-Bus door registered"]

    NOSESS -->|"org.kde.KWin appears on the bus"| BOUND
    BOUND -->|"KWin vanishes: logout, crash, restart"| NOSESS
    BOUND -->|"reinstall on every reappearance"| BOUND

    classDef a fill:#292c30,stroke:#3daee9,color:#fcfcfc
    classDef b fill:#1d1f22,stroke:#3c4045,color:#a1a9b1
    class BOUND a
    class NOSESS b
```

Watching the bus name rather than installing once at start-up is what makes the
bindings as durable as the desktop instead of as durable as one moment in it —
the old implementation installed at program start, so a KWin restart silently
took the shortcuts and left a program that believed it still had them.

## What the picture is asserting

- **One actor, from the first commit.** Every write to every device leaves
  through the per-device mailboxes inside the service. No shell holds a device
  handle — the CLI is an API client from the day it exists, so there is no
  direct-write path to remove later. Two callers cannot fight over a device
  because there is only ever one caller.
- **Two doors, one flow.** The HTTP API is the interface; the D-Bus object
  exists only because KWin scripting can reach nothing else. Both terminate in
  the same service core.
- **Desired state is separate from what is on the hardware.** Reconcilers close
  the gap on a timer, because some devices do not hold what they are told (the
  wireless G502 restores onboard state on wake) and the LCD drops a static image
  within seconds. A preview is held apart from desired state so it cannot be
  mistaken for an instruction.
- **Latest wins per device, and the unit is a frame.** Lighting addresses
  targets — device, zone, LED range or named segment — and the assignments for a
  device compose into one complete frame before anything is written. That is
  what makes a write atomic from the device's point of view, and what lets a
  single-slot mailbox coalesce without dropping half a scene.
- **Two paths, one device.** The Kraken's lighting is OpenRGB's; its telemetry
  and its screen are hotaru's own, spoken to over `/dev/hidraw` and usbfs. The
  split is the hardware's rather than a preference: OpenRGB exposes the colour
  channels and nothing else, and liquidctl — which used to fill the gap —
  exposes no colour channels for this model at all. Doing the cooler directly
  took a Python interpreter and a subprocess per reading out of the service,
  and a reading from 105 ms to about two (spec 012).
- **One writer per file.** Rules are the user's and are never rewritten;
  scenes are machine-written because the GUI edits them; desired state lives
  outside the config directory entirely; and the GUI's own file holds nothing
  but view state. YAML throughout, which is why the file the user comments is
  not one the program ever serialises back.
- **The service starts at boot, not at login.** A user unit with no desktop
  dependency, restoring recorded state by reconciling toward it. The OpenRGB
  server is a resource that appears rather than a unit to order after — started
  is not ready, as a cold boot finding two devices of six demonstrated. Desktop
  pieces such as the KWin script attach when the session shows up and reattach
  when it restarts.
- **Every backend is optional.** OpenRGB, the cooler and the kernel's sensors
  are independent legs; any of them missing removes its capabilities from the
  API and the GUI without failing the others or stopping the service. A machine
  with no liquid cooler is the ordinary case, not an error, and says so through
  the same route a reading would take. The KWin script is a KDE convenience —
  elsewhere the CLI is the binding mechanism.
- **The monitor is a consumer, eventually.** It keeps its own reads for now —
  an accepted, temporary overlap — and becomes an API client when it is
  rewritten in Go.

## Keeping it current

Update this diagram in the commit that changes the decision, not afterwards. It
is a `mermaid` block in Markdown so it renders in Zettlr and on GitHub without a
build step; the theme is pinned in the block's `init` directive, so editing the
diagram does not mean re-choosing colours. Once the Go project exists, it moves to the fynedesygn convention —
a `.mmd` source rendered to PNG by `mmdc` under `go generate`, with the PNG
committed — so the gallery and the docs pane can show it too.
