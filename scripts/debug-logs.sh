#!/bin/sh
set -e

if [ "${1:-}" = "--help" ] || [ "${1:-}" = "-h" ]; then
    cat <<'EOF'
Usage: debug-logs.sh [-n <lines>]

Tail BMO's log file on a connected device over ADB (follows new output;
Ctrl-C to stop).

Options:
  -n <lines>   Number of trailing lines to print initially (default: 50)

Environment:
  DEPLOY_PLATFORM=tg5040|tg5050   Target platform directory (default: tg5040)
EOF
    exit 0
fi

LINES=50
if [ "${1:-}" = "-n" ]; then
    LINES="${2:?-n requires a line count}"
fi

PLATFORM="${DEPLOY_PLATFORM:-tg5040}"
LOG_PATH="/mnt/SDCARD/.userdata/$PLATFORM/logs/BMO.txt"

if ! adb get-state >/dev/null 2>&1; then
    echo "ERROR: no device connected over ADB." >&2
    exit 1
fi

if ! adb shell "[ -f '$LOG_PATH' ]"; then
    echo "ERROR: $LOG_PATH not found on device. Has BMO been launched?" >&2
    exit 1
fi

echo "Tailing $LOG_PATH (Ctrl-C to stop)..." >&2
exec adb shell "tail -n $LINES -f '$LOG_PATH'"
