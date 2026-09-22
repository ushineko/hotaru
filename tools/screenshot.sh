#!/usr/bin/env bash
# Capture hotaru's window for the gallery, on KDE/Wayland.
#
# Ported from fynedesygn's tools/screenshot.sh (same author, MIT), which was
# ported from angou's. The differences are hotaru's: the window is a client of
# a running service, so a capture shows this machine's real devices, scenes and
# pictures -- and two of the states worth showing are reached by clicking
# rather than by a flag, which is what --current is for.
#
# Three things make this less trivial than "take a screenshot":
#
#   1. The active window is almost never the one we want. Refreshing these
#      means a terminal driving the capture, so whatever has focus is the
#      terminal. The window is raised first and found by *pid as well as
#      class*, because the developer's own copy of hotaru-gui is usually
#      already running and a class search returns that one first.
#   2. When a dialog is open the dialog *is* the active window, so an
#      active-window grab returns the dialog alone on a transparent
#      background. Those shots pass --with-dialog: capture the desktop, crop
#      to the window's geometry.
#   3. The window reads settings from $XDG_CONFIG_HOME. HOME and the XDG
#      config/data directories are redirected to a throwaway directory so a
#      capture cannot pick up this desk's window size or navigation shape --
#      but XDG_RUNTIME_DIR is deliberately left alone, because that is where
#      both the Wayland socket and hotaru's own socket live. The service is a
#      separate process with the real HOME, so the scenes and pictures in the
#      images are real.
#
# Requires kdotool, spectacle, and python3 with Pillow.
set -euo pipefail

REPO_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
CLASS="${HOTARU_CLASS:-io.github.ushineko.hotaru}"
BIN="${HOTARU_GUI:-$REPO_DIR/bin/hotaru-gui}"
SCHEME_DEFAULT="Breeze Dark"
HOME_DIR=""
SOCKET=""

usage() {
    cat <<'USAGE'
usage: tools/screenshot.sh [--bin PATH] [--class APPID] [--scheme NAME]
                           [--with-dialog] --section NAME <output.png>
       tools/screenshot.sh --current [--with-dialog] <output.png>
       tools/screenshot.sh --all

  --section NAME  which section to open on: a section, or one of Create's
                  parts by name (Scenes, Pictures, Screen)
  --current       capture the hotaru-gui window that is already open, as it
                  is. For the states a flag cannot reach -- the scene editor
                  with a light selected, a chooser mid-dialog -- which
                  somebody sets up by hand and this then grabs.
  --with-dialog   a dialog is open: capture the desktop and crop, rather than
                  grabbing the active window (which would be the dialog alone)
  --scheme NAME   colour scheme for this run; not saved over the user's choice
  --bin PATH      the window to start; default ./bin/hotaru-gui ($HOTARU_GUI)
  --socket PATH   the service to talk to. For capturing against a throwaway
                  service -- one with a library of stock wallpapers rather
                  than somebody's own photographs, which is what the Pictures
                  image wants when the images are published
  --class APPID   the window class to find; default io.github.ushineko.hotaru
  --home DIR      the throwaway HOME to run under; created when absent
  --all           refresh the flag-reachable set into docs/img/

The service must be running: the window is a client, and a capture of "the
service is not running" is a screenshot of a machine nobody has.

docs/gallery.md holds the images, what each is of, and when to refresh them.
The alt text there is the only description a screen-reader user gets, and a
stale one is worse than none: check it still matches before committing.
USAGE
}

with_dialog=0
current=0
section=""
scheme=""
out=""
all=0
while [ $# -gt 0 ]; do
    case "$1" in
        --class) shift; CLASS="${1:-}" ;;
        --bin) shift; BIN="${1:-}" ;;
        --with-dialog) with_dialog=1 ;;
        --current) current=1 ;;
        --section) shift; section="${1:-}" ;;
        --scheme) shift; scheme="${1:-}" ;;
        --home) shift; HOME_DIR="${1:-}" ;;
        --socket) shift; SOCKET="${1:-}" ;;
        --all) all=1 ;;
        -h|--help) usage; exit 0 ;;
        -*) echo "unknown option: $1" >&2; usage >&2; exit 2 ;;
        *) out="$1" ;;
    esac
    shift
done

for tool in kdotool spectacle python3; do
    command -v "$tool" >/dev/null || { echo "$tool is not installed" >&2; exit 1; }
done
python3 -c "import PIL" 2>/dev/null || { echo "python3 Pillow is not installed" >&2; exit 1; }

made_home=0
if [ -z "$HOME_DIR" ]; then
    HOME_DIR=$(mktemp -d)
    made_home=1
fi
mkdir -p "$HOME_DIR/.config" "$HOME_DIR/.local/share"
cleanup() { [ "$made_home" -eq 1 ] && rm -rf "$HOME_DIR"; return 0; }
trap cleanup EXIT

# grab raises a window, waits for it to actually have focus, and captures it.
# Shared by both modes: what differs is whose window it is.
grab() {
    local wid="$1" dest="$2"

    # Activating is asynchronous and `spectacle -a` grabs whatever is active
    # at the moment it fires. Activate, confirm, then grab.
    local active="" tries=0
    while [ "$tries" -lt 12 ]; do
        timeout 10 kdotool windowactivate "$wid" >/dev/null 2>&1 || true
        sleep 0.5
        active=$(timeout 10 kdotool getactivewindow 2>/dev/null || true)
        [ "$active" = "$wid" ] && break
        tries=$((tries + 1))
    done
    [ "$active" = "$wid" ] || { echo "could not focus the window (active=$active want=$wid)" >&2; return 1; }
    # Long enough for the window's poll to land. It asks the service every
    # two seconds, and a capture taken before the first answer shows a status
    # bar reading "0 of 0 devices" under a window full of devices -- which
    # looks like a bug in the program rather than in the screenshot.
    sleep "${SETTLE:-4.0}"

    rm -f "$dest"
    if [ "$with_dialog" -eq 0 ]; then
        # -S drops the compositor's drop shadow, which pads the image unevenly
        timeout 30 spectacle -a -b -n -S -o "$dest" >/dev/null 2>&1 || true
        sleep 1.5
    else
        local tmp; tmp=$(mktemp --suffix=.png)
        timeout 30 spectacle -f -b -n -o "$tmp" >/dev/null 2>&1 || true
        sleep 1.5
        local geo; geo=$(timeout 10 kdotool getwindowgeometry "$wid")
        python3 "${REPO_DIR}/tools/crop.py" "$tmp" "$dest" \
            "$(printf '%s' "$geo" | awk '/Position/{print $2}')" \
            "$(printf '%s' "$geo" | awk '/Geometry/{print $2}')"
        rm -f "$tmp"
    fi

    [ -s "$dest" ] || { echo "capture produced nothing" >&2; return 1; }

    # A last check on the geometry: an image wildly wider or taller than the
    # window we asked for is not a screenshot of it.
    local geo_check; geo_check=$(timeout 10 kdotool getwindowgeometry "$wid" 2>/dev/null || true)
    python3 - "$dest" "$(printf '%s' "$geo_check" | awk '/Geometry/{print $2}')" <<'PY'
import os, sys
from PIL import Image

path, dim = sys.argv[1], sys.argv[2] if len(sys.argv) > 2 else ""
im = Image.open(path)
if dim and "x" in dim:
    w, h = (float(v) for v in dim.split("x"))
    want, got = w / h, im.width / im.height
    if abs(want - got) / want > 0.05:
        sys.exit("captured %dx%d, but the window is %gx%g -- wrong window grabbed"
                 % (im.width, im.height, w, h))
print("  %s  %dx%d  %.0fK" % (os.path.basename(path), im.width, im.height,
                              os.path.getsize(path) / 1024))
PY
}

# capture starts a window on the requested section, grabs it, and stops it
# again. Fresh per shot rather than reusing one window, so each image is
# independent of whatever the previous one left selected.
capture() {
    local sect="$1" dest="$2"

    HOME="$HOME_DIR" XDG_CONFIG_HOME="$HOME_DIR/.config" XDG_DATA_HOME="$HOME_DIR/.local/share" \
        "$BIN" ${sect:+--section "$sect"} ${scheme:+--scheme "$scheme"} \
        ${SOCKET:+--socket "$SOCKET"} >/dev/null 2>&1 &
    local pid=$!
    # shellcheck disable=SC2064  # pid is captured deliberately, at trap-set time
    trap "kill $pid 2>/dev/null || true; wait $pid 2>/dev/null || true" RETURN

    # By pid as well as class: this desk usually has hotaru-gui open already,
    # and a class search returns that one first -- every image would be of it.
    local wid="" waited=0 w
    while [ "$waited" -lt 40 ]; do
        for w in $(timeout 10 kdotool search --class "$CLASS" 2>/dev/null || true); do
            if [ "$(timeout 10 kdotool getwindowpid "$w" 2>/dev/null || true)" = "$pid" ]; then
                wid=$w
                break
            fi
        done
        [ -n "$wid" ] && break
        sleep 0.25
        waited=$((waited + 1))
    done
    [ -n "$wid" ] || { echo "the window never appeared (no window of class $CLASS with pid $pid)" >&2; return 1; }

    grab "$wid" "$dest"
}

if [ "$current" -eq 1 ]; then
    [ -n "$out" ] || { usage >&2; exit 2; }
    wid=$(timeout 10 kdotool search --class "$CLASS" 2>/dev/null | head -1 || true)
    [ -n "$wid" ] || { echo "no hotaru-gui window is open" >&2; exit 1; }
    grab "$wid" "$out"
    exit 0
fi

[ -x "$BIN" ] || { echo "$BIN not found or not executable (run make gui, or set --bin)" >&2; exit 1; }

if [ "$all" -eq 1 ]; then
    # A scheme unless one was asked for, so two refreshes on two machines
    # produce the same colours.
    : "${scheme:=$SCHEME_DEFAULT}"
    mkdir -p "${REPO_DIR}/docs/img"
    for s in Service System Scenes Pictures Screen Appearance About; do
        low=$(printf '%s' "$s" | tr '[:upper:]' '[:lower:]')
        capture "$s" "${REPO_DIR}/docs/img/window-${low}.png"
    done
    echo
    echo "The editor and the dialogs are not in this set: they are reached by"
    echo "clicking. Open one and run --current, and see docs/gallery.md."
    exit 0
fi

[ -n "$out" ] || { usage >&2; exit 2; }
[ -n "$section" ] || { echo "--section is required (or use --all, or --current)" >&2; exit 2; }
capture "$section" "$out"
