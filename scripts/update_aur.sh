#!/bin/bash
#
# Publish the PKGBUILD in packaging/ to the AUR.
#
# The same job clockwork-orange does from CI, done from here because hotaru
# has no CI yet. It is the copy that matters: the AUR reads .SRCINFO rather
# than the PKGBUILD, so the two are regenerated and committed together -- a
# stale .SRCINFO is the commonest way an AUR package breaks.
#
# The version comes from .tag, which is the same file the Makefile stamps the
# binaries from, so a package can never claim a version the binary does not.
#
# Usage: scripts/update_aur.sh [commit message]

set -euo pipefail

PKGNAME='hotaru'
AUR_URL="ssh://aur@aur.archlinux.org/${PKGNAME}.git"

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
clone="${root}/.aur-repo"

version="$(tr -d 'v[:space:]' < "${root}/.tag")"
message="${1:-Update to ${version}}"

echo "==> ${PKGNAME} ${version}"

# The tag has to exist: the PKGBUILD's source is the release tarball, and its
# checksum is of a file GitHub only makes once the tag is pushed.
if ! git -C "$root" rev-parse "v${version}" >/dev/null 2>&1; then
	echo "no tag v${version}: tag and push before publishing" >&2
	exit 1
fi

if [ -d "$clone" ]; then
	git -C "$clone" fetch origin
	git -C "$clone" reset --hard origin/master
else
	git clone "$AUR_URL" "$clone"
fi

cp "${root}/packaging/PKGBUILD" "$clone/"
sed -i "s/^pkgver=.*/pkgver=${version}/" "$clone/PKGBUILD"

# Regenerated rather than copied, because it is derived and the AUR believes
# it over the PKGBUILD.
(cd "$clone" && makepkg --printsrcinfo > .SRCINFO)

git -C "$clone" add PKGBUILD .SRCINFO
if git -C "$clone" diff --cached --quiet; then
	echo "==> nothing to publish"
	exit 0
fi

git -C "$clone" --no-pager diff --cached --stat
git -C "$clone" commit -m "$message"
git -C "$clone" push origin HEAD:master

echo "==> https://aur.archlinux.org/packages/${PKGNAME}"
