# Packaging and publishing

Decided early, built late. Nothing here is implemented until hotaru works; it is
written now because the dependency question has an answer that follows from the
architecture, and discovering that at packaging time usually means discovering
it wrong.

The source of record for the behaviour these packages assume is
`specs/001-scope-migration-and-lighting-core.md`, in particular "Someone else's
machine".

## The rule that decides everything else

**hotaru has no hard dependency on any backend.** OpenRGB, liquidctl and
OpenLinkHub are each optional at runtime, and a machine with none of them still
installs, starts and serves. So in packaging terms they are `optdepends`, never
`depends`, and each one says what is lost without it.

Making them `depends` would be the easy mistake: it would pull a Python
toolchain and a lighting daemon onto a machine that wanted the CLI, and it would
quietly contradict the program's own behaviour. A package's dependency list is a
claim about what the program needs, and hotaru's claim is "almost nothing".

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
depends=('glibc')
optdepends=('openrgb: RGB lighting control'
            'liquidctl: cooler telemetry and LCD control'
            'openlinkhub: CPU package temperature in the cooler snapshot')

# hotaru-gui
depends=('hotaru' 'libglvnd' 'libx11' 'libxcursor' 'libxrandr'
         'libxinerama' 'libxi' 'libxxf86vm' 'libxkbcommon' 'wayland')

makedepends=('go')
```

`kwin` is deliberately *not* listed, even as an optdepend: the hotkey
integration is a Plasma convenience, and on every other desktop the CLI is the
binding mechanism. A package that suggested KWin would imply hotaru wants it.

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
