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
        LQC["liquidctl<br/>explicit argv subprocess"]
        OLH["OpenLinkHub<br/>localhost HTTP"]
    end

    subgraph hw["Hardware"]
        LIT["Lit devices<br/>Kraken · GPU · Aura · MM700 · G502 · Keychron"]
        LCD["Kraken LCD<br/>640x640, GIF only"]
        TELEM["Cooler telemetry<br/>coolant · pump · fans · CPU pkg"]
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
    MBOX -->|"set screen"| LQC
    CORE -->|"status poll"| LQC
    CORE -->|"CPU package temp"| OLH

    ORGB --> LIT
    LQC --> LCD
    LQC --> TELEM
    OLH --> TELEM

    PBM -.->|"temporary: its own reads<br/>until the Go rewrite"| LQC
    PBM -.-> OLH
    FUTURE -.->|"GET /v1/cooler<br/>no device handle"| API

    classDef svcnode fill:#292c30,stroke:#3daee9,color:#fcfcfc
    classDef backend fill:#1d1f22,stroke:#3c4045,color:#fcfcfc
    classDef hwnode fill:#141618,stroke:#3c4045,color:#a1a9b1
    classDef future fill:#1d1f22,stroke:#a1a9b1,stroke-dasharray:5 5,color:#a1a9b1
    classDef entry fill:#292c30,stroke:#3c4045,color:#fcfcfc

    class DBUS,API,CORE,STATE,RECON,MBOX,CFG svcnode
    class ORGB,LQC,OLH backend
    class LIT,LCD,TELEM hwnode
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
- **Two backends, one device.** The Kraken's lighting is OpenRGB's; its LCD and
  telemetry are liquidctl's. liquidctl exposes no colour channels for this
  model at all.
- **One writer per file.** Rules are the user's and are never rewritten;
  scenes are machine-written because the GUI edits them; desired state lives
  outside the config directory entirely; and the GUI's own file holds nothing
  but view state. YAML throughout, which is why the file the user comments is
  not one the program ever serialises back.
- **Every backend is optional.** OpenRGB, liquidctl and OpenLinkHub are three
  independent legs; any of them missing removes its capabilities from the API
  and the GUI without failing the others or stopping the service. The KWin
  script is a KDE convenience — elsewhere the CLI is the binding mechanism.
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
