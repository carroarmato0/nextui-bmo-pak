#!/bin/sh
set -e

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
cd "$SCRIPT_DIR/.."

if [ "${1:-}" = "--help" ] || [ "${1:-}" = "-h" ]; then
    cat <<'EOF'
Usage: deploy-mods.sh [<sd-card-path>]

Deploy every example mod under examples/mods/ to a connected device over ADB,
or copy them to a mounted SD card path. Each mod becomes selectable under
Settings -> MOD. Scales automatically as more example mods are added.

Environment:
  DEPLOY_PLATFORM=tg5040|tg5050   Target platform userdata dir (default: tg5040)
EOF
    exit 0
fi

PLATFORM="${DEPLOY_PLATFORM:-tg5040}"
SRC_ROOT="examples/mods"
MODS_SUBPATH=".userdata/$PLATFORM/BMO/mods"

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

SD_PATH="${1:-}"
if [ -n "$SD_PATH" ]; then
    DEST="$SD_PATH/$MODS_SUBPATH"
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

if ! command -v adb >/dev/null 2>&1; then
    echo "ERROR: adb not found. Install android-tools (or android-platform-tools)." >&2
    exit 1
fi

DEVICE="$(adb devices | awk 'NR==2 {print $1}')"
if [ -z "$DEVICE" ]; then
    echo "ERROR: no ADB device connected. Check USB cable." >&2
    exit 1
fi

DEST="/mnt/SDCARD/$MODS_SUBPATH"
echo "==> Deploying mods to $DEVICE:$DEST"
adb shell "mkdir -p $DEST"
for name in $mods; do
    adb push "$SRC_ROOT/$name" "$DEST/" >/dev/null
    echo "  $name"
done
echo "Done. Select one under Settings -> MOD."
