#!/bin/sh
set -e

PAK_DIR="$(cd "$(dirname "$0")" && pwd)"
PLATFORM="${BMO_PLATFORM:-}"
if [ -z "$PLATFORM" ]; then
    if {
        [ -r /proc/device-tree/compatible ] && tr -d '\000' </proc/device-tree/compatible | grep -Eqi 'tg5050|smart pro s';
    } || {
        [ -r /proc/device-tree/model ] && tr -d '\000' </proc/device-tree/model | grep -Eqi 'tg5050|smart pro s';
    } || grep -qi "TG5050" /proc/cpuinfo 2>/dev/null; then
        PLATFORM="tg5050"
    else
        PLATFORM="tg5040"
    fi
fi

BASE_USERDATA="/mnt/SDCARD/.userdata"
BMO_DATA_ROOT="$BASE_USERDATA/$PLATFORM"
export BMO_PLATFORM="$PLATFORM"
export BMO_DATA_ROOT="$BMO_DATA_ROOT"
export HOME="$BMO_DATA_ROOT/BMO"
export PATH="$PAK_DIR:$PATH"
mkdir -p "$HOME" "$BMO_DATA_ROOT/logs"
# The device has no system CA certificate store; point Go's TLS stack at the
# bundled cert file so HTTPS calls to OpenAI and other providers work.
export SSL_CERT_FILE="$PAK_DIR/assets/ca-certificates.crt"

# SDL2 library resolution: prefer the device-native SDL2 (tuned for the
# device's display backend — EGL/pvrsrvkm on Smart Pro, fbdev on Brick) over
# the bundled LoveRetro fallback.
NATIVE_SDL_LIB=""
for _d in /usr/trimui/lib /usr/miyoo/lib /usr/lib /usr/local/lib; do
    if [ -f "$_d/libSDL2-2.0.so.0" ]; then
        NATIVE_SDL_LIB="$_d"
        break
    fi
done
unset _d
BUNDLED_SDL_LIB="$PAK_DIR/lib/$PLATFORM"
export LD_LIBRARY_PATH="${NATIVE_SDL_LIB:+$NATIVE_SDL_LIB:}$BUNDLED_SDL_LIB${LD_LIBRARY_PATH:+:$LD_LIBRARY_PATH}"

cd "$PAK_DIR"
exec "$PAK_DIR/bin/$PLATFORM/bmo-pak" "$@"
