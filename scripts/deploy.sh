#!/bin/sh
set -e

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
cd "$SCRIPT_DIR/.."

if [ "${1:-}" = "--help" ] || [ "${1:-}" = "-h" ]; then
    cat <<'EOF'
Usage: deploy.sh [<sd-card-path>]

Deploy the already-built BMO Pak to a connected device over ADB,
or copy it to a mounted SD card path.

Environment:
  DEPLOY_PLATFORM=tg5040|tg5050   Target platform directory (default: tg5040)
EOF
    exit 0
fi

PLATFORM="${DEPLOY_PLATFORM:-tg5040}"
PAK_SRC="dist/all/Tools/$PLATFORM/BMO.pak"

if [ ! -d "$PAK_SRC" ]; then
    echo "ERROR: $PAK_SRC not found. Run scripts/release.sh first." >&2
    exit 1
fi

SD_PATH="${1:-}"
if [ -n "$SD_PATH" ]; then
    DEST="$SD_PATH/Tools/$PLATFORM/BMO.pak"
    echo "==> Copying to SD card: $DEST"
    rm -rf "$DEST"
    mkdir -p "$(dirname "$DEST")"
    cp -R "$PAK_SRC" "$DEST"
    echo "Done."
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

DEST="/mnt/SDCARD/Tools/$PLATFORM/BMO.pak"
echo "==> Deploying to $DEVICE:$DEST"
adb shell "mkdir -p $DEST"
adb push "$PAK_SRC/." "$DEST/"
echo "Done."
