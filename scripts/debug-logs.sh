#!/bin/sh
set -e

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
. "$SCRIPT_DIR/lib/adb-devices.sh"

show_help() {
    cat <<'EOF'
Usage: debug-logs.sh [-n <lines>] [--all] [--device <serial|name>]

Tail BMO's log file on connected device(s) over ADB (follows new output;
Ctrl-C to stop).

With a single device it tails directly. With several connected devices it asks
which to follow; choosing "all" opens a tmux session with one split pane per
device (or, without tmux, interleaves label-prefixed lines in this terminal).

Options:
  -n <lines>           Trailing lines to print initially (default: 50)
  --device <s|name>    Follow one device by serial or label substring
                       (e.g. "brick", "smart pro").
  --all                Follow every detected device (split view).

Environment:
  DEPLOY_PLATFORM=tg5040|tg5050   Force the platform (overrides per-device
                                  detection of the log path).
EOF
}

LINES=50
DEVICE_FILTER=""
SELECT_ALL=0
while [ $# -gt 0 ]; do
    case "$1" in
        -h|--help)   show_help; exit 0 ;;
        -n)          shift; LINES="${1:?-n requires a line count}" ;;
        --all)       SELECT_ALL=1 ;;
        --device)    shift; DEVICE_FILTER="$1" ;;
        --device=*)  DEVICE_FILTER="${1#--device=}" ;;
        -*)          echo "ERROR: unknown option '$1' (see --help)." >&2; exit 1 ;;
        *)           echo "ERROR: unexpected argument '$1' (see --help)." >&2; exit 1 ;;
    esac
    shift
done

export DEVICE_FILTER SELECT_ALL

log_path_for() { echo "/mnt/SDCARD/.userdata/$1/logs/BMO.txt"; }

require_adb || exit 1

targets=$(choose_targets "$(print_supported_devices)") || exit 1

# Keep only devices that actually have a BMO log; resolve each device's log path
# (honoring a DEPLOY_PLATFORM override). Record format: serial\tlabel\tlogpath.
ready=""
while IFS="$TAB" read -r serial platform label <&3; do
    [ -n "$serial" ] || continue
    plat="${DEPLOY_PLATFORM:-$platform}"
    log="$(log_path_for "$plat")"
    if adb -s "$serial" shell "[ -f '$log' ]" </dev/null >/dev/null 2>&1; then
        ready="${ready:+$ready
}$serial$TAB$label$TAB$log"
    else
        echo "WARN: $label [$serial]: $log not found (has BMO been launched?); skipping." >&2
    fi
done 3<<EOF
$targets
EOF

count=$(printf '%s\n' "$ready" | sed '/^$/d' | wc -l | tr -d ' ')
if [ "$count" -eq 0 ]; then
    echo "ERROR: no selected device has a BMO log yet." >&2
    exit 1
fi

# --- Single device: tail directly -------------------------------------------
if [ "$count" -eq 1 ]; then
    serial=$(printf '%s\n' "$ready" | sed '/^$/d' | cut -f1)
    label=$(printf '%s\n' "$ready"  | sed '/^$/d' | cut -f2)
    log=$(printf '%s\n' "$ready"    | sed '/^$/d' | cut -f3)
    echo "Tailing $label [$serial]: $log (Ctrl-C to stop)..." >&2
    exec adb -s "$serial" shell "tail -n $LINES -f '$log'"
fi

# --- Several devices: tmux split panes ---------------------------------------
if command -v tmux >/dev/null 2>&1; then
    session="bmo-logs"
    tmux kill-session -t "$session" 2>/dev/null || true
    tmux set -g pane-border-status top 2>/dev/null || true
    first=1
    while IFS="$TAB" read -r serial label log <&3; do
        [ -n "$serial" ] || continue
        pane_cmd="adb -s $serial shell tail -n $LINES -f $log"
        if [ "$first" -eq 1 ]; then
            tmux new-session -d -s "$session" -n logs "$pane_cmd"
            first=0
        else
            tmux split-window -t "$session" "$pane_cmd"
            tmux select-layout -t "$session" tiled >/dev/null
        fi
        tmux select-pane -t "$session" -T "$label [$serial]" 2>/dev/null || true
    done 3<<EOF
$ready
EOF
    echo "Opening tmux session '$session' with $count panes (Ctrl-C a pane to stop; 'tmux kill-session -t $session' to close)." >&2
    exec tmux attach -t "$session"
fi

# --- Fallback without tmux: interleave label-prefixed lines ------------------
echo "tmux not found; interleaving $count logs in this terminal (Ctrl-C to stop all)." >&2
pids=""
# shellcheck disable=SC2064
trap 'kill $pids 2>/dev/null' INT TERM EXIT
while IFS="$TAB" read -r serial label log <&3; do
    [ -n "$serial" ] || continue
    adb -s "$serial" shell "tail -n $LINES -f $log" </dev/null 2>&1 \
        | sed "s/^/[$label] /" &
    pids="$pids $!"
done 3<<EOF
$ready
EOF
wait
