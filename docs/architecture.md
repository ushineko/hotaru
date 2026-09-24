# hotaru architecture

The shape of the system as decided so far. The source of record for *why* any
of it is this way is `specs/001-scope-migration-and-lighting-core.md`. This
page is the picture, and it changes in the same commit as any decision that
changes the system.

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

The colours are Breeze Dark, the palette `fynedesygn` ships as its default
scheme, so the diagram matches the program it describes. It renders dark
whatever theme the viewer uses. That is deliberate, because that is how the
work is read.

**Solid** is the architecture after the cutover. **Dashed** is one of two
things. It is temporary, like the monitor reading the cooler for itself until
its Go rewrite. Or it is not built yet: the CLI's direct path exists only
until the service does, and the Go monitor is a direction rather than a
commitment.

## Boot and readiness

The service starts with the machine. Nothing below waits for a login, and
nothing reads "the unit started" as "the hardware is there".

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
`After=` or `Requires=` on any OpenRGB unit. No single unit exists to name: the
`openrgb` package ships a system unit, this machine runs a user unit, and
other people start the server by hand. Ordering would not help in any case. A
cold boot has reached `Started` with two devices of six enumerated, because
OpenRGB detects devices once and USB enumeration had not finished. hotaru
judges readiness by the device list instead. That is why **Partial** is a
state of its own, rather than a successful restore with fewer lights than
expected.

## Session attachment

The desktop is a precondition for nothing. It appears, and it may appear more
than once.

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

hotaru watches the bus name rather than installing once at start-up. That
makes the bindings as durable as the desktop, rather than as durable as one
moment in it. The old implementation installed at program start, so a KWin
restart took the shortcuts and left a program that believed it still held
them.

## What the picture is asserting

- **One actor, from the first commit.** Every write to every device leaves
  through the per-device mailboxes inside the service. No shell holds a device
  handle. The CLI is an API client from the day it exists, so no direct-write
  path has to be removed later. Two callers cannot compete for a device,
  because only one caller exists.
- **Two doors, one flow.** The HTTP API is the interface. The D-Bus object
  exists only because KWin scripting can reach nothing else. Both end in the
  same service core.
- **Desired state is separate from what is on the hardware.** Reconcilers
  close the gap on a timer, because some devices do not hold what they are
  told. The wireless G502 restores its onboard state on wake, and the LCD
  drops a static image within seconds. hotaru holds a preview apart from
  desired state, so that nobody mistakes a preview for an instruction.
- **Latest wins per device, and the unit is a frame.** Lighting addresses
  targets: a device, a zone, an LED range or a named segment. The assignments
  for one device compose into one complete frame before anything is written.
  That makes a write atomic from the device's point of view, and it lets a
  single-slot mailbox coalesce writes without dropping half a scene.
- **Two paths, one device.** The Kraken's lighting belongs to OpenRGB. Its
  telemetry and its screen are hotaru's own, over `/dev/hidraw` and usbfs. The
  hardware sets that split rather than a preference. OpenRGB exposes the
  colour channels and nothing else, and liquidctl, which used to fill the gap,
  exposes no colour channels for this model at all. Reading the cooler
  directly removed a Python interpreter and a subprocess per reading from the
  service, and took a reading from 105 ms to about two (spec 012).
- **One writer per file.** The rules belong to the user, and hotaru never
  rewrites them. hotaru writes the scenes, because the window edits them.
  Desired state lives outside the config directory. The window's own file
  holds view state and nothing else. Every file is YAML, which is why the file
  the user comments is never one the program writes back.
- **The service starts at boot, not at login.** It is a user unit with no
  desktop dependency, and it restores recorded state by reconciling toward it.
  The OpenRGB server is a resource that appears, rather than a unit to order
  after. Started is not ready, as a cold boot that found two devices of six
  showed. The desktop pieces, such as the KWin script, attach when the session
  appears and attach again when it restarts.
- **Every backend is optional.** OpenRGB, the cooler and the kernel's sensors
  are independent legs. A missing leg removes its capabilities from the API
  and the window, and the others continue. The service keeps running. A
  machine with no liquid cooler is the ordinary case rather than an error, and
  it reports that through the same route a reading would take. The KWin script
  is a KDE convenience, and away from Plasma the CLI is the binding mechanism.
- **The monitor is a consumer, eventually.** It keeps its own reads for now,
  which is an accepted and temporary overlap. It becomes an API client when
  somebody rewrites it in Go.

## Keeping it current

Update this diagram in the commit that changes the decision, rather than
afterwards. It is a `mermaid` block in Markdown, so it renders in Zettlr and
on GitHub without a build step. The block's `init` directive pins the theme,
so editing the diagram does not mean choosing colours again. Once the Go
project exists, the diagram moves to the fynedesygn convention: a `.mmd`
source that `mmdc` renders to PNG under `go generate`, with the PNG committed.
The gallery and the docs pane can then show it too.
