#!/bin/sh
set -e

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
cd "$SCRIPT_DIR/.."
. "$SCRIPT_DIR/lib/adb-devices.sh"

show_help() {
    cat <<'EOF'
Usage: deploy.sh [--all] [--device <serial|name>] [<sd-card-path>]

Deploy the already-built BMO Pak to connected device(s) over ADB, or copy it to
a mounted SD card path.

Over ADB, every connected & recognized handheld is detected and the correct
per-device build is pushed (a Brick and a Smart Pro plugged in together each get
their own platform's pak in one run).

Selection (ADB mode):
  default              Deploy to ALL detected supported devices.
  --device <s|name>    Deploy to one device, matched by serial or by a substring
                       of its label (e.g. "brick", "smart pro").
  --all                Deploy to all devices (skip the interactive menu).
  (When run in a terminal with several devices and no flag, a menu is shown.)

Positional:
  <sd-card-path>       Copy to <path>/Tools/<platform>/BMO.pak instead of ADB.

Environment:
  DEPLOY_PLATFORM=tg5040|tg5050   Force the platform (overrides per-device
                                  detection; required for SD-card mode).
EOF
}

PLATFORM="${DEPLOY_PLATFORM:-tg5040}"

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

pak_src() { echo "dist/all/Tools/$1/BMO.pak"; }

# --- SD-card mode (no ADB; uses DEPLOY_PLATFORM) -----------------------------
if [ -n "$SD_PATH" ]; then
    SRC="$(pak_src "$PLATFORM")"
    if [ ! -d "$SRC" ]; then
        echo "ERROR: $SRC not found. Run scripts/release.sh first." >&2
        exit 1
    fi
    DEST="$SD_PATH/Tools/$PLATFORM/BMO.pak"
    echo "==> Copying to SD card: $DEST"
    rm -rf "$DEST"
    mkdir -p "$(dirname "$DEST")"
    cp -R "$SRC" "$DEST"
    echo "Done."
    exit 0
fi

# --- ADB mode ----------------------------------------------------------------
require_adb || exit 1

targets=$(choose_targets "$(print_supported_devices)") || exit 1

deployed=0
skipped=0
# Read targets on FD 3 so adb (which consumes stdin) can't eat the loop's input.
while IFS="$TAB" read -r serial platform label <&3; do
    [ -n "$serial" ] || continue
    plat="${DEPLOY_PLATFORM:-$platform}"
    SRC="$(pak_src "$plat")"
    if [ ! -d "$SRC" ]; then
        echo "WARN: $label: $SRC missing (run scripts/release.sh); skipping." >&2
        skipped=$((skipped + 1))
        continue
    fi
    DEST="/mnt/SDCARD/Tools/$plat/BMO.pak"
    echo "==> $label [$serial] -> $DEST"
    adb -s "$serial" shell "mkdir -p $DEST" </dev/null
    adb -s "$serial" push "$SRC/." "$DEST/" </dev/null
    deployed=$((deployed + 1))
done 3<<EOF
$targets
EOF

if [ "$skipped" -gt 0 ]; then
    echo "Done. Deployed to $deployed device(s); skipped $skipped."
else
    echo "Done. Deployed to $deployed device(s)."
fi
[ "$deployed" -gt 0 ] || exit 1
