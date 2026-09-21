# Spec 007: packaging and release

**Issue**: [#7](https://github.com/ushineko/hotaru/issues/7)

## Status: COMPLETE

## Context

[docs/packaging.md](../docs/packaging.md) decided the shape a year of specs
ago -- deliberately, because the dependency question follows from the
architecture and discovering it at packaging time usually means discovering it
wrong. This spec builds what that document describes, and is the last one
before the first tagged release.

Nothing here re-argues the decisions. What follows is what was made, and the
three places the document left open.

### What the package installs

One PKGBUILD, two packages. `hotaru` is the CLI and the service: pure Go, no
OpenGL, no X11, so it installs on a headless box. `hotaru-gui` is the Fyne
window, which needs the graphics stack and depends on `hotaru` for the service
it is a client of.

- `/usr/bin/hotaru`, `/usr/bin/hotaru-gui`
- `/usr/lib/systemd/user/hotaru.service` -- not enabled
- `/usr/lib/udev/rules.d/60-hotaru.rules`
- `/usr/share/applications/io.github.ushineko.hotaru.desktop` and
  `/usr/share/icons/hicolor/scalable/apps/hotaru.svg`
- `/usr/share/doc/hotaru/hotaru.yml.example` and
  `/usr/share/doc/hotaru/openrgb-enumeration.conf.example`
- `/usr/share/licenses/<pkg>/LICENSE`

### The udev rule is the part that is easy to leave out

hotaru opens two nodes to reach the cooler, and neither needs root: logind
puts an ACL on a device tagged `uaccess` for whoever is logged in. On the
machine this was written on, that tag came from the `liquidctl` package --
which hotaru dropped in spec 012.

So a machine that never had liquidctl has no rule, and hotaru finds the cooler
and cannot open it: lighting works, telemetry and the screen do not, and the
reason is invisible. That is the "someone else's machine" failure spec 001
exists to prevent, and it could not have shown up here, because this machine
has had liquidctl installed all along. The package ships its own rule.

The vendor is matched rather than the product, because a rule listing products
would have to be edited in step with `internal/cooler/discover.go`, and a
`uaccess` tag on a device hotaru does not drive costs nothing.

### The open questions, answered

**A separate AUR package for the window, or a split package?** Split, as the
document assumed. One source tree, one build, two packages: a second AUR
repository would duplicate the PKGBUILD for a binary built from the same tree
in the same run, and the two would drift.

**A udev note for OpenRGB's own permissions?** No. The `openrgb` package
handles its devices, and a second package with opinions about them is how two
rules end up disagreeing.

**Where the PKGBUILD lives.** In this repository, under `packaging/`, so it is
versioned with the thing it packages and a change to what is installed and a
change to the installer are one commit. The AUR repositories take a copy;
`.SRCINFO` is generated there, in the same commit as any PKGBUILD change,
because the AUR reads it rather than the PKGBUILD and a stale one is the
commonest way a package breaks. It is deliberately **not** kept here: it
carries a checksum for a tarball that does not exist until the tag does.

### The first machine found it in an hour

The package installed, the service started, and the first thing anybody did
on the second machine failed:

	hotaru: no image library: make the image directory:
	mkdir /home/.../.local/share/hotaru: read-only file system

`ProtectHome=read-only` makes `$HOME` read-only to the service, and a path
named in `ReadWritePaths` is only punched through **if it already exists** --
systemd cannot bind-mount a directory that is not there. The dashes in front
of those three paths stop the unit failing to start when they are missing,
which is what they were for; what nobody noticed is that they also leave the
service unable to create them.

So on a fresh install hotaru could not make its own directories: no pictures,
and on a machine that had never run it before, no saved rules or scenes
either. The development machine had made all three long before it was ever
packaged, which is why the unit had been right here for weeks.

The fix is `ExecStartPre=+/usr/bin/install -d -m 0700 ...`: `+` runs a command
outside the namespacing, which is the one place a directory the sandbox needs
can still be made. 0700 rather than the umask, because that is the mode hotaru
gives them itself.

**This is spec 001's argument, arriving on schedule.** The packaging could not
be verified on the machine that wrote it, and the first machine that had never
run hotaru found the fault within an hour of installing.

The service also keeps *why* there is no library now, rather than logging it
at startup and answering "the image library is unavailable on this machine"
for the rest of the day. The reason is the half that tells somebody what to
do.

## Requirements

**R1. One PKGBUILD, two packages**, the window's dependencies confined to the
window's package.

**R2. The backends are `depends`**, not `optdepends`: the package decides what
a working install needs rather than asking somebody to know which capability
comes from which daemon.

**R3. The cooler is reachable without root**, by a rule the package ships.

**R4. Nothing is enabled or started** by installing.

**R5. Examples are examples**: the rules file and the OpenRGB drop-in install
under `/usr/share/doc`, never as active configuration.

**R6. `check()` runs the suite**, which needs no hardware, no OpenRGB, no
display and no root.

**R7. The service can make its own directories**, on a machine where none of
them exist yet.

**R8. The release flow is the ushineko one**: changelog heading dated, README
**Version** line matched, `govulncheck` clean, the README commit on `main`
before the tag, and a GitHub Release for every tag whose notes are that
version's changelog entry verbatim.

## Acceptance Criteria

- [x] AC1. `makepkg --printsrcinfo` parses the PKGBUILD and names both
      packages with their own dependencies.
- [x] AC2. The window's graphics dependencies appear only in `hotaru-gui`.
- [x] AC3. The udev rule tags both interfaces of the supported vendor.
- [x] AC4. The unit is installed and not enabled, and the README says which
      two commands enable it and why neither is run for you.
- [x] AC5. The desktop entry is named for the Fyne app ID and carries
      `StartupWMClass`.
- [x] AC6. `go test ./...` passes with no hardware present, which is what
      `check()` runs.
- [x] AC7. The README has an Install section: the AUR packages, `go install`,
      and the first-run steps.
- [x] AC8. Verified by building the package from a pushed tag and installing
      it on a second machine: the service runs from the unit, drives nine
      devices, and a picture added there makes a scene. **It found a bug in
      an hour** -- see "The first machine found it in an hour".
- [x] AC9. A fresh install creates `~/.config/hotaru`,
      `~/.local/state/hotaru` and `~/.local/share/hotaru` at 0700, and a
      service that cannot says why rather than only that it cannot.
- [ ] AC10. The udev rule verified on a machine with a supported cooler and
      no liquidctl. Neither machine here is both.

## Risks & Assumptions

- **AC8 is open until there is a tag**, because the source array names a
  tarball that does not exist until then, and the checksum with it. The
  PKGBUILD is written; `updpkgsums` fills the checksum at release, and the
  install is the last check before the AUR upload.
- **`namcap` is not installed on this machine**, so the package has not been
  linted. It runs before the AUR upload, which is where the document already
  put it.
- **The second machine is the real test.** Everything above is reasoning about
  a machine whose udev rules this developer has never seen.
- **Rollback** is not pushing the AUR update; the tag and the GitHub Release
  stand on their own, and `go install` is unaffected by any of it.
