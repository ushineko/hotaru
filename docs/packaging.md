# Packaging and publishing

Decided early, built late. Nobody implements anything here until hotaru works.
This page exists now because the dependency question has an answer that
follows from the architecture, and a packager who first asks it at packaging
time usually answers it wrong.

The source of record for the behaviour these packages assume is
`specs/001-scope-migration-and-lighting-core.md`, in particular "Someone else's
machine".

## The rule that decides everything else

**The backends are hard dependencies. Runtime optionality is a separate claim.**

These are two different questions, and keeping them apart matters:

- *What must be installed for `pacman -S hotaru` to give somebody a working
  program?* All three backends. A user cannot know which pieces of hotaru come
  from which daemon, because that is internal detail. The package decides for
  them, rather than making the choice a prerequisite for a working program.
- *What must be present for hotaru to keep running?* Nothing. A daemon can
  stop, a device can vanish, and a user who installed with `go install` has no
  package manager at all. The program degrades in every one of those cases,
  and this page does not change that requirement.

So `depends` states the supported install rather than a runtime precondition,
and the backends use no `optdepends`. The graceful-degradation rules in spec
001 stand as written. They describe a machine's state, not a package's
manifest.

## Split: `hotaru` and `hotaru-gui`

One PKGBUILD, two packages.

`hotaru` holds the CLI and the service. It is pure Go, with no OpenGL, no X11
and no Wayland, so it installs on a headless box, in a container, or on a
server whose only lighting is a fan somebody forgot about.

`hotaru-gui` holds the Fyne program, which needs the graphics stack. It
depends on `hotaru` for the service and the shared docs. `make gui` builds it
separately from the service, for the reason it ships separately: cgo and
OpenGL on one side, and a binary that runs on a headless box on the other.

The architecture produces the split. The window is a client, so it separates
cleanly, and every heavy dependency sits on the client side.

### The window's icon takes three things, not one

A Fyne window on KDE under Wayland shows its icon only when all three of these
agree. Two of the three correct shows nothing:

1. **The icon compiled in** (`shell.Options.Icon`). This is the in-app icon,
   and the X11 window icon.
2. **A desktop entry named for the app_id**:
   `io.github.ushineko.hotaru.desktop`, because the compositor resolves the
   *titlebar* icon by matching the app_id to a desktop file of that name. Its
   `Icon=hotaru` then resolves through the icon theme, which is why the SVG
   installs to `hicolor/scalable/apps/hotaru.svg`.
3. **`StartupWMClass=io.github.ushineko.hotaru`** in that entry, which is what
   the *task manager* matches on.

**Then the trap.** With all three correct, the taskbar can still be blank.
plasmashell caches "no icon" for an app_id it has already failed to resolve,
so every run made while the entry was missing poisons that cache.
`kbuildsycoca6` does not clear it. `systemctl --user restart
plasma-plasmashell` clears it, and so does the next login.

A sibling project learned this at the cost of an afternoon. The afternoon went
on concluding that the configuration was wrong, when it was correct and
cached.

## Dependencies, named

Verified against this machine's repositories:

| Package | Repo | Role |
|---|---|---|
| `openrgb` | extra | Lighting. Without it, lighting is absent |
| `go` | extra | Build only |
| `libglvnd`, `libx11`, `libxcursor`, `libxrandr`, `libxinerama`, `libxi`, `libxxf86vm`, `libxkbcommon`, `wayland` | extra | The GUI's runtime graphics stack |

```
# hotaru
depends=('glibc' 'openrgb')
optdepends=('nvidia-utils: GPU temperature on the dashboard')

# hotaru-gui
depends=('hotaru' 'libglvnd' 'libx11' 'libxcursor' 'libxrandr'
         'libxinerama' 'libxi' 'libxxf86vm' 'libxkbcommon' 'wayland')

makedepends=('go')
```

**`liquidctl` and `openlinkhub` were here and are not any more.** hotaru reads
the cooler and drives its screen itself, over `/dev/hidraw*` and usbfs, and it
reads every temperature the Corsair daemon used to supply from
`/sys/class/hwmon`. See
[spec 012](../specs/012-the-cooler-without-liquidctl.md). That removes Python
from machines that may have no liquid cooler at all: `python`,
`python-pillow`, `python-pyusb` and `i2c-tools`. It also removes an AUR daemon
that was installed for one field of the snapshot.

`nvidia-utils` is the one `optdepends`, and it earns the label. A machine with
no NVIDIA card has no GPU temperature, which is a missing number rather than a
broken program. That is what `optdepends` is for, and it is what separates
this case from the backends below.

### Device permissions are part of the package

hotaru opens two nodes to reach the cooler: a `/dev/hidraw*` node for status
and control, and `/dev/bus/usb/BBB/DDD` for the screen's bulk endpoint.
Neither needs root, because `systemd-logind` puts an ACL on them for the
logged-in user. It does so **only if a udev rule tags the device `uaccess`**.

On the development machine that rule came from the `liquidctl` package, which
hotaru no longer depends on. A machine that has never had liquidctl installed
has no such rule, so hotaru finds the cooler and cannot open it. Lighting
works, telemetry and the screen do not, and nothing on screen says why.

That is the "someone else's machine" failure spec 001 exists to prevent, and
it could not have appeared here: this machine has had liquidctl installed all
along.

So **the package ships its own rule**, naming the devices hotaru supports:

```
# /usr/lib/udev/rules.d/60-hotaru.rules
KERNEL=="hidraw*", SUBSYSTEMS=="usb", ATTRS{idVendor}=="1e71", TAG+="uaccess"
SUBSYSTEM=="usb", ATTRS{idVendor}=="1e71", TAG+="uaccess"
```

Two rules, because the two interfaces appear differently. One appears as a
hidraw character device, and one as the USB device node itself.

`hotaru light health` reports a device it can see and cannot open as exactly
that, rather than as missing hardware. "Permission denied on /dev/hidraw7" is
a sentence somebody can act on. "No cooler found" is not.

The alternative was `optdepends`, and install size is not why this page
rejects it. **`optdepends` asks the user a question they cannot answer.**
Choosing correctly from that list means knowing already that lighting comes
from OpenRGB, which means knowing how hotaru is built internally. Nobody who
installs a program to make their fans blue should read an architecture
document first. A user who guesses wrong gets a program that looks broken, and
nothing tells them that it is not.

Hard dependencies move that knowledge into the package, where it belongs. The
packager pays the cost once, in disk space, instead of a stranger paying it in
debugging.

A user *can* reason about all this from [docs/hardware.md](hardware.md), which
says what each backend contributes and what hardware it covers. "Why does this
install a Corsair daemon?" has an answer there, in devices rather than in
package manifests.

The PKGBUILD lists `kwin` in neither form, deliberately. The hotkey
integration is a Plasma convenience, and on every other desktop the CLI is the
binding mechanism. A package that named KWin would imply that hotaru wants
it.

## Build

Arch's Go packaging conventions, with the version stamped from the tag the way
terrariabonker does it:

```bash
export CGO_CPPFLAGS="${CPPFLAGS}" CGO_CFLAGS="${CFLAGS}"
export CGO_CXXFLAGS="${CXXFLAGS}" CGO_LDFLAGS="${LDFLAGS}"
export GOFLAGS="-buildmode=pie -trimpath -mod=readonly -modcacherw"
go build -ldflags "-linkmode=external -X ${_module}/internal/version.Version=${pkgver}" ./cmd/hotaru
```

`check()` runs `go test ./...`. That check means something because the suite
is headless and needs no hardware, no OpenRGB and no root, which is a
packaging property as much as a testing one.

## What gets installed

- `/usr/bin/hotaru`, `/usr/bin/hotaru-gui`
- `/usr/share/applications/io.github.ushineko.hotaru.desktop`
- `/usr/share/icons/hicolor/scalable/apps/hotaru.svg`
- `/usr/lib/udev/rules.d/60-hotaru.rules`. The `uaccess` tags that let the
  service open the cooler without root. Without the rule, hotaru sees the
  device and cannot talk to it. See "Device permissions are part of the
  package" above.
- `/usr/lib/systemd/user/hotaru.service`. **Not enabled**, per Arch policy.
  The README tells the user to run `systemctl --user enable --now hotaru`. The
  unit is in [`packaging/`](../packaging/), already written
- `/usr/share/applications/<app-id>.desktop` and the icon. Both are named to
  match the Fyne app ID, with `StartupWMClass` set, because KDE on Wayland
  matches the window to the entry that way. A mismatch gives a generic icon
  and reports nothing
- `/usr/share/doc/hotaru/hotaru.yml.example`. The example rules file for this
  desk, in the same YAML the program reads. It ships with an example systemd
  drop-in that gates OpenRGB's start on device enumeration. **Examples, not
  defaults**: the enumeration gate exists because of one machine's boot
  ordering, and shipping it as active configuration would fit the packaging to
  that machine
- `/usr/share/licenses/hotaru/LICENSE`

The service unit carries **no ordering or dependency on OpenRGB at all**:
no `After=`, and no `Wants=`. No single unit exists to name. The `openrgb`
package ships a system unit, this developer's machine runs a user unit, and
other people start the server by hand. Ordering would not help in any case,
because OpenRGB reaching `Started` does not mean its devices are enumerated.
hotaru retries, and judges readiness by the device list. It starts whether or
not anything is there to talk to.

The unit is also **not tied to a desktop session**. It uses
`WantedBy=default.target` and no `graphical-session.target`, so hotaru
restores the lighting at boot rather than at login. That needs `loginctl
enable-linger` for the user, and the package does **not** run it. Starting a
user manager at boot is the user's decision, so hotaru offers the command
rather than a post-install script that runs it.

## The three AUR packages

The name is free. The AUR has no `hotaru`, which somebody checked.

| Package | Source | For | |
|---|---|---|---|
| `hotaru` | The release tarball, built from source | The default | **published** at 0.1.0, and building `hotaru-gui` as a split package |
| `hotaru-bin` | The release binaries | People who do not want a Go toolchain | not yet |
| `hotaru-git` | `main` | Testing before a release | not yet |

`scripts/update_aur.sh` publishes the first of them. It takes the version from
`.tag`, refuses to run before that tag is pushed, and regenerates `.SRCINFO`
rather than copying it. The other two follow the same shape when they are
worth having: `-bin` when the release carries binaries for more than one
architecture, and `-git` when somebody other than the author wants to track
`main`.

Each one is its own `aur.archlinux.org` git repository. Regenerate `.SRCINFO`
with `makepkg --printsrcinfo > .SRCINFO` in the same commit as any PKGBUILD
change. A stale `.SRCINFO` is the most common way an AUR package breaks,
because the site reads it rather than the PKGBUILD.

`namcap` runs on the built package before upload.

## Publishing

The ushineko release convention, unchanged:

1. One `chore: release X.Y.Z` commit sets the version in **three** places:
   `### Unreleased` in the README changelog becomes `### X.Y.Z (YYYY-MM-DD)`
   with the **Version** line to match, `.tag` becomes `X.Y.Z`, and the
   PKGBUILD's `pkgver` does too with its `sha256sums` back to `SKIP` until
   the tarball exists.

   All three, because something different reads each one and none of them
   checks the others. The Makefile stamps the binary from `.tag`,
   `scripts/update_aur.sh` takes its version from `.tag`, and the AUR builds
   from `pkgver`. A release that moves the README alone tags a commit whose
   binaries report the *previous* version. That is what v0.1.6 did on its
   first attempt, and it is why this step names three files.
2. `go test`, `make lint` and `govulncheck ./...` pass.
3. The README commit lands on `main`, then the tag `vX.Y.Z` is pushed.
4. **Every tag gets a GitHub Release**, titled `vX.Y.Z`, with that version's
   changelog entry as its notes, word for word:
   `gh release create vX.Y.Z --title vX.Y.Z --notes-file <entry>`.
   A bare tag is invisible. It does not appear in the Releases feed, nobody
   can watch it, and a consumer deciding whether to upgrade has to read a
   diff.
5. The AUR packages are updated and their `.SRCINFO` regenerated.

Away from Arch, `go install github.com/ushineko/hotaru/cmd/hotaru@latest`
installs the CLI, and the release carries built binaries. Flatpak and AppImage
are not planned. A program whose job is to reach the user's own hardware and
session bus fits a sandbox badly, and saying so once is better than
half-supporting it.

## Settled

Building the package answered both of the questions this document left open.
See [spec 007](../specs/007-packaging-and-release.md).

- **`hotaru-gui` is a split package** of the same PKGBUILD, as this page
  assumed. A second AUR repository would duplicate a PKGBUILD for a binary
  built from the same tree in the same run, and the two copies would drift.
- **OpenRGB's device permissions stay with the `openrgb` package.** A second
  package with opinions about another package's devices is how two udev rules
  come to disagree. hotaru ships a rule for the cooler it drives itself, and
  nothing else.

The PKGBUILD is in [`packaging/`](../packaging/), versioned with what it
packages.
