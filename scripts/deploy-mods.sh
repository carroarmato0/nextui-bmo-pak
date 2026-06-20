#!/bin/sh
set -e

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
cd "$SCRIPT_DIR/.."
. "$SCRIPT_DIR/lib/adb-devices.sh"

show_help() {
    cat <<'EOF'
Usage: deploy-mods.sh [--all] [--device <serial|name>] [<sd-card-path>]

Deploy every example mod under examples/mods/ to connected device(s) over ADB,
or copy them to a mounted SD card path. Each mod becomes selectable under
Settings -> MOD. Scales automatically as more example mods are added.

Over ADB, every connected & recognized handheld is detected and the mods are
pushed into that device's own platform userdata dir.

Selection (ADB mode):
  default              Deploy to ALL detected supported devices.
  --device <s|name>    Deploy to one device, matched by serial or by a substring
                       of its label (e.g. "brick", "smart pro").
  --all                Deploy to all devices (skip the interactive menu).
  (When run in a terminal with several devices and no flag, a menu is shown.)

Positional:
  <sd-card-path>       Copy to <path>/.userdata/<platform>/BMO/mods instead.

Environment:
  DEPLOY_PLATFORM=tg5040|tg5050   Force the platform (overrides per-device
                                  detection; required for SD-card mode).
EOF
}

PLATFORM="${DEPLOY_PLATFORM:-tg5040}"
SRC_ROOT="examples/mods"

if [ ! -d "$SRC_ROOT" ]; then
    echo "ERROR: $SRC_ROOT not found (run from the repo root)." >&2
    exit 1
fi

# A mod is any immediate subdirectory of examples/mods/.
mods=""
for moddir in "$SRC_ROOT"/*/; do
    [ -d "$moddir" ] || continue
    mods="$mods $(basename "$moddir")"
done
if [ -z "$mods" ]; then
    echo "ERROR: no mods found under $SRC_ROOT/." >&2
    exit 1
fi

DEVICE_FILTER=""
SELECT_ALL=0
SD_PATH=""
while [ $# -gt 0 ]; do
    case "$1" in
        -h|--help)   show_help; exit 0 ;;
        --all)       SELECT_ALL=1 ;;
        --device)    shift; DEVICE_FILTER="$1" ;;
        --device=*)  DEVICE_FILTER="${1#--device=}" ;;
        -*)          echo "ERROR: unknown option '$1' (see --help)." >&2; exit 1 ;;
        *)           SD_PATH="$1" ;;
    esac
    shift
done

mods_subpath() { echo ".userdata/$1/BMO/mods"; }

# --- SD-card mode (no ADB; uses DEPLOY_PLATFORM) -----------------------------
if [ -n "$SD_PATH" ]; then
    DEST="$SD_PATH/$(mods_subpath "$PLATFORM")"
    echo "==> Copying mods to SD card: $DEST"
    mkdir -p "$DEST"
    for name in $mods; do
        rm -rf "$DEST/$name"
        cp -R "$SRC_ROOT/$name" "$DEST/$name"
        echo "  $name"
    done
    echo "Done. Select one under Settings -> MOD."
    exit 0
fi

# --- ADB mode ----------------------------------------------------------------
require_adb || exit 1

targets=$(choose_targets "$(print_supported_devices)") || exit 1

deployed=0
# Read targets on FD 3 so adb (which consumes stdin) can't eat the loop's input.
while IFS="$TAB" read -r serial platform label <&3; do
    [ -n "$serial" ] || continue
    plat="${DEPLOY_PLATFORM:-$platform}"
    DEST="/mnt/SDCARD/$(mods_subpath "$plat")"
    echo "==> $label [$serial] -> $DEST"
    adb -s "$serial" shell "mkdir -p $DEST" </dev/null
    for name in $mods; do
        adb -s "$serial" push "$SRC_ROOT/$name" "$DEST/" </dev/null >/dev/null
        echo "  $name"
    done
    deployed=$((deployed + 1))
done 3<<EOF
$targets
EOF

echo "Done. Deployed mods to $deployed device(s). Select one under Settings -> MOD."
[ "$deployed" -gt 0 ] || exit 1
