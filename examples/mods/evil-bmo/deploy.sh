#!/usr/bin/env bash
# Stage the tracked Evil BMO mod into dist/ and push it to the device.
# Usage: examples/mods/evil-bmo/deploy.sh   (run from repo root)
set -euo pipefail

SRC="examples/mods/evil-bmo"
STAGE="dist/mods/evil-bmo"
DEVICE_DIR="/mnt/SDCARD/.userdata/tg5040/BMO/mods"

# Copy only the runtime assets (exclude dev-only files: tests, docs, this script).
rm -rf "$STAGE"
mkdir -p "$STAGE/faces"
cp "$SRC/mod.json" "$SRC/persona.txt" "$SRC/voice.txt" "$SRC/quotes.txt" "$STAGE/"
cp "$SRC"/faces/*.svg "$STAGE/faces/"

echo "Staged to $STAGE:"
find "$STAGE" -type f | sort

adb push "$STAGE" "$DEVICE_DIR/"
echo "Pushed to $DEVICE_DIR/evil-bmo — select it under Settings → MOD."
