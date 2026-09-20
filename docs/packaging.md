# Packaging and publishing

Decided early, built late. Nothing here is implemented until hotaru works; it is
written now because the dependency question has an answer that follows from the
architecture, and discovering that at packaging time usually means discovering
it wrong.

The source of record for the behaviour these packages assume is
`specs/001-scope-migration-and-lighting-core.md`, in particular "Someone else's
machine".

## The rule that decides everything else

**The backends are hard dependencies. Runtime optionality is a separate claim.**

These are two different questions and it is worth keeping them apart:

- *What must be installed for `pacman -S hotaru` to give someone a working
  program?* All three backends. A user cannot be expected to know which pieces
  of hotaru come from which daemon — that is internal detail — so the package
  decides for them rather than making the choice a prerequisite for the program
  working.
- *What must be present for hotaru to keep running?* Nothing. A daemon can be
  stopped, a device can vanish, and a user who installed with `go install` has
  no package manager in the picture at all. The program degrades in every one of
  those cases, and that requirement is untouched by this.

So `depends` expresses the supported install, not a runtime precondition, and
`optdepends` is not used for the backends at all. The graceful-degradation rules
in spec 001 stand exactly as written; they are about a machine's state, not
about a package's manifest.

## Split: `hotaru` and `hotaru-gui`

One PKGBUILD, two packages.

`hotaru` — the CLI and the service. Pure Go with no OpenGL, no X11 and no
Wayland, so it installs on a headless box, in a container, or on a server whose
only lighting is a fan someone forgot about.

`hotaru-gui` — the Fyne program, which needs the graphics stack. It depends on
`hotaru` for the service and the shared docs.

The split falls out of the architecture: the GUI is a client, so it is genuinely
separable, and the heavy dependencies are all on the client side.

## Dependencies, named

Verified against this machine's repositories:

| Package | Repo | Role |
|---|---|---|
| `openrgb` | extra | Lighting. Without it, lighting is absent |
| `liquidctl` | extra | Cooler telemetry and the LCD. Without it, both are absent |
| `openlinkhub` | AUR | CPU package temperature only. Without it, that one field is missing |
| `go` | extra | Build only |
| `libglvnd`, `libx11`, `libxcursor`, `libxrandr`, `libxinerama`, `libxi`, `libxxf86vm`, `libxkbcommon`, `wayland` | extra | The GUI's runtime graphics stack |

```
# hotaru
depends=('glibc' 'openrgb' 'liquidctl' 'openlinkhub')

# hotaru-gui
depends=('hotaru' 'libglvnd' 'libx11' 'libxcursor' 'libxrandr'
         'libxinerama' 'libxi' 'libxxf86vm' 'libxkbcommon' 'wayland')

makedepends=('go')
```

Two consequences to accept knowingly:

- **`openlinkhub` is in the AUR, not the official repositories.** An AUR package
  may depend on another, and helpers resolve it, but it means every hotaru
  install builds and runs a Corsair-device daemon for one field of the snapshot.
- **`liquidctl` brings Python** — `python`, `python-pillow`, `python-pyusb`,
  `i2c-tools` — onto a machine that may have no liquid cooler, and `openrgb`
  brings `qt6-base`, which on a Plasma desktop is already installed.

The alternative was `optdepends`, and it was rejected for a better reason than
install size: **`optdepends` asks the user a question they have no way to
answer.** Choosing correctly from that list means already knowing that lighting
comes from OpenRGB, that the cooler's screen is liquidctl's, and that one field
of the telemetry comes from a third daemon — that is, knowing how hotaru is put
together internally. Nobody installing a program to make their fans blue should
have to read an architecture document first, and a user who guesses wrong gets a
program that looks broken and has no way to tell that it is not.

Hard dependencies move that knowledge into the package, where it belongs. The
cost is paid once, in disk space, by the packager's decision rather than by a
stranger's debugging.

The place a user *can* reason about all this is
[docs/hardware.md](hardware.md), which says what each backend contributes and
what hardware it covers. "Why does this install a Corsair daemon?" has an answer
there, in terms of devices rather than of package manifests.

`kwin` is deliberately *not* listed, in either form: the hotkey integration is a
Plasma convenience, and on every other desktop the CLI is the binding mechanism.
A package that named KWin would imply hotaru wants it.

## Build

Arch's Go packaging conventions, with the version stamped from the tag the way
terrariabonker does it:

```bash
export CGO_CPPFLAGS="${CPPFLAGS}" CGO_CFLAGS="${CFLAGS}"
export CGO_CXXFLAGS="${CXXFLAGS}" CGO_LDFLAGS="${LDFLAGS}"
export GOFLAGS="-buildmode=pie -trimpath -mod=readonly -modcacherw"
go build -ldflags "-linkmode=external -X ${_module}/internal/version.Version=${pkgver}" ./cmd/hotaru
```

`check()` runs `go test ./...`, which is meaningful because the suite is headless
and needs no hardware, no OpenRGB and no root — that is a packaging property as
much as a testing one.

## What gets installed

- `/usr/bin/hotaru`, `/usr/bin/hotaru-gui`
- `/usr/lib/systemd/user/hotaru.service` — **not enabled**, per Arch policy. The
  README says `systemctl --user enable --now hotaru`
- `/usr/share/applications/<app-id>.desktop` and the icon, named to match the
  Fyne app ID with `StartupWMClass` set, because KDE on Wayland matches the
  window to the entry that way and a mismatch silently gives a generic icon
- `/usr/share/doc/hotaru/hotaru.yml.example` — the example rules file for this
  desk, in the same YAML the program reads,
  and an example systemd drop-in for gating OpenRGB's start on device
  enumeration. **Examples, not defaults**: the enumeration gate exists because
  of one machine's boot ordering, and shipping it as active configuration would
  be over-fitting by packaging
- `/usr/share/licenses/hotaru/LICENSE`

The service unit carries no `Requires` on OpenRGB. It starts whether or not
anything is there to talk to, because that is what the program does.

## The three AUR packages

The name is free — the AUR has no `hotaru` (checked).

| Package | Source | For |
|---|---|---|
| `hotaru` | The release tarball, built from source | The default |
| `hotaru-bin` | The release binaries | People who do not want a Go toolchain |
| `hotaru-git` | `main` | Testing before a release |

Each is its own `aur.archlinux.org` git repository, and `.SRCINFO` is
regenerated with `makepkg --printsrcinfo > .SRCINFO` in the same commit as any
PKGBUILD change — a stale `.SRCINFO` is the most common way an AUR package
breaks, because the site reads it rather than the PKGBUILD.

`namcap` runs on the built package before upload.

## Publishing

The ushineko release convention, unchanged:

1. `### Unreleased` in the README changelog becomes `### X.Y.Z (YYYY-MM-DD)`,
   and the **Version** line is set to match.
2. `go test`, `make lint` and `govulncheck ./...` pass.
3. The README commit lands on `main`, then the tag `vX.Y.Z` is pushed.
4. **Every tag gets a GitHub Release**, titled `vX.Y.Z`, notes being that
   version's changelog entry verbatim:
   `gh release create vX.Y.Z --title vX.Y.Z --notes-file <entry>`.
   A bare tag is invisible: it is not in the Releases feed, nobody can watch it,
   and a consumer deciding whether to bump has to read a diff.
5. The AUR packages are updated and their `.SRCINFO` regenerated.

Off Arch, `go install github.com/ushineko/hotaru/cmd/hotaru@latest` works for the
CLI, and the release carries built binaries. Flatpak and AppImage are not
planned: a program whose job is to reach the user's own hardware and session bus
is a poor fit for a sandbox, and saying so once is better than half-supporting
it.

## Open

- Whether `hotaru-gui` is worth a separate AUR package or is better as a split
  package of the same PKGBUILD only. The latter is simpler and is the current
  assumption.
- Whether to ship a `sysusers`/`udev` note for OpenRGB's device permissions, or
  leave that entirely to the `openrgb` package, which already handles it.
