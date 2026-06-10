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

cd "$PAK_DIR"
exec "$PAK_DIR/bin/$PLATFORM/bmo-pak" "$@"
