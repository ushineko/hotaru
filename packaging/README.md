# Packaging files

What a distribution package installs, kept here so it can be read without
unpacking one. The shape and the dependency argument are in
[docs/packaging.md](../docs/packaging.md); spec 007 builds the PKGBUILD itself.

| File | Installed as |
|---|---|
| `hotaru.service` | `/usr/lib/systemd/user/hotaru.service` |

The unit is **not enabled** by the package, per Arch policy. After installing:

```console
systemctl --user enable --now hotaru
loginctl enable-linger $USER      # so lighting comes back at boot, not at login
```

Neither command is run for you. Starting a daemon and turning on a user manager
at boot are the user's decisions, and hotaru's job is to say which command would
help — `hotaru light health` names the one you need.
