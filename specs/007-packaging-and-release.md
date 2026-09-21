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

**R7. The release flow is the ushineko one**: changelog heading dated, README
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
- [ ] AC8. Verified by building the package from a pushed tag and installing
      it on a machine that has never had liquidctl.

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
