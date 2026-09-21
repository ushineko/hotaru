# Packaging files

What a distribution package installs, kept here so it can be read without
unpacking one. The shape and the dependency argument are in
[docs/packaging.md](../docs/packaging.md); spec 007 builds the PKGBUILD itself.

| File | Installed as |
|---|---|
| `PKGBUILD` | not installed — the AUR package, kept with what it packages |
| `hotaru.service` | `/usr/lib/systemd/user/hotaru.service` |
| `60-hotaru.rules` | `/usr/lib/udev/rules.d/60-hotaru.rules` |
| `io.github.ushineko.hotaru.desktop` | `/usr/share/applications/io.github.ushineko.hotaru.desktop` |
| `hotaru.svg` | `/usr/share/icons/hicolor/scalable/apps/hotaru.svg` |
| `openrgb-enumeration.conf.example` | `/usr/share/doc/hotaru/openrgb-enumeration.conf.example` |

The udev rule is the one that is easy to leave out and impossible to notice:
without it hotaru finds the cooler and cannot open it, so lighting works while
telemetry and the screen do not. It came from the `liquidctl` package on the
machine this was written on, and hotaru no longer depends on that (spec 012).

`.SRCINFO` is **not** kept here. It carries a checksum for a tarball that does
not exist until the tag does; it is generated in the AUR repository with
`makepkg --printsrcinfo > .SRCINFO`, in the same commit as any PKGBUILD
change, because the AUR reads it rather than the PKGBUILD.

At release: `updpkgsums` fills the checksum, `namcap` runs on the built
package, and then the copy goes to the AUR.

The unit is **not enabled** by the package, per Arch policy. After installing:

```console
systemctl --user enable --now hotaru
loginctl enable-linger $USER      # so lighting comes back at boot, not at login
```

Neither command is run for you. Starting a daemon and turning on a user manager
at boot are the user's decisions, and hotaru's job is to say which command would
help — `hotaru light health` names the one you need.
