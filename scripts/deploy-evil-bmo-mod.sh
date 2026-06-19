#!/usr/bin/env bash
# Stage the tracked Evil BMO mod into dist/ and push it to the device.
# Usage: scripts/deploy-evil-bmo-mod.sh   (run from repo root)
set -euo pipefail

SRC="examples/mods/evil-bmo"
STAGE="dist/mods/evil-bmo"
DEVICE_DIR="/mnt/SDCARD/.userdata/tg5040/BMO/mods"

# Copy the mod's runtime assets into the staging dir. (The example dir is now
# data-only; its validation test lives in internal/examplemods.)
rm -rf "$STAGE"
mkdir -p "$STAGE/faces"
cp "$SRC/mod.json" "$SRC/persona.txt" "$SRC/voice.txt" "$SRC/quotes.txt" "$STAGE/"
cp "$SRC"/faces/*.svg "$STAGE/faces/"

# Voice clips (hello/goodbye/etc) are optional; copy them only if present.
if compgen -G "$SRC/audio/*.pcm" > /dev/null; then
  mkdir -p "$STAGE/audio"
  cp "$SRC"/audio/*.pcm "$STAGE/audio/"
fi

echo "Staged to $STAGE:"
find "$STAGE" -type f | sort

adb push "$STAGE" "$DEVICE_DIR/"
echo "Pushed to $DEVICE_DIR/evil-bmo — select it under Settings → MOD."
