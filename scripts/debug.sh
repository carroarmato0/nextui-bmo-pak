#!/bin/sh
# On-device profiling helper for bmo-pak over ADB.
#
# Workflow:
#   ./scripts/debug.sh profile          # enable cpu+mem+sample flags
#   # launch BMO via NextUI, exercise workloads, then exit BMO gracefully
#   ./scripts/debug.sh pull-profile     # fetch profiles + CSV to ./debug-profiles/
#   ./scripts/debug.sh profile-restore  # remove flags
#   go tool pprof bin/$PLATFORM/bmo-pak debug-profiles/bmo-cpu.prof
set -e

PLATFORM="${BMO_PLATFORM:-tg5040}"
PAK_DEST="/mnt/SDCARD/Tools/$PLATFORM/BMO.pak"
DEV_TMP="/tmp"
PROF_DIR="$(pwd)/debug-profiles"

CPU_PROF="$DEV_TMP/bmo-cpu.prof"
MEM_PROF="$DEV_TMP/bmo-mem.prof"
SAMPLE_CSV="$DEV_TMP/bmo-perf-sample.csv"

usage() {
    cat <<EOF
Usage: $0 <command>

  profile          Enable CPU+memory+RSS-sample flags; launch via NextUI to record
  profile-cpu      Enable CPU-only profiling
  profile-mem      Enable heap-only profiling
  profile-sample   Enable RSS/CPU CSV sampling only
  profile-live     Enable live pprof (HTTP :6060 via ADB forward)
  profile-restore  Remove profiling flags (restores normal launch)
  pull-profile     Pull recorded profiles + CSV to ./debug-profiles/

Platform defaults to tg5040; override with BMO_PLATFORM=tg5050.
EOF
}

write_flags() {
    adb shell "printf '%s' '$1' > $PAK_DEST/.profile-flags"
    echo "==> wrote flags: $1"
    echo "    Now launch BMO via NextUI, exercise workloads, then exit BMO."
    echo "    Then: $0 pull-profile && $0 profile-restore"
}

case "${1:-}" in
    profile)
        write_flags "-cpuprofile $CPU_PROF -memprofile $MEM_PROF -perfsample $SAMPLE_CSV"
        ;;
    profile-cpu)
        write_flags "-cpuprofile $CPU_PROF"
        ;;
    profile-mem)
        write_flags "-memprofile $MEM_PROF"
        ;;
    profile-sample)
        write_flags "-perfsample $SAMPLE_CSV"
        ;;
    profile-live)
        adb shell "printf '%s' '-pprof :6060' > $PAK_DEST/.profile-flags"
        adb forward tcp:6060 tcp:6060
        echo "==> live pprof enabled; forwarded localhost:6060 -> device:6060"
        echo "    Launch BMO via NextUI, then from the host:"
        echo "      go tool pprof 'http://localhost:6060/debug/pprof/profile?seconds=30'"
        echo "      go tool pprof http://localhost:6060/debug/pprof/heap"
        echo "    Then: $0 profile-restore"
        ;;
    profile-restore)
        adb shell "rm -f $PAK_DEST/.profile-flags"
        adb forward --remove tcp:6060 2>/dev/null || true
        echo "==> removed .profile-flags and any :6060 forward"
        ;;
    pull-profile)
        mkdir -p "$PROF_DIR"
        for f in "$CPU_PROF" "$MEM_PROF" "$SAMPLE_CSV"; do
            if adb shell "[ -f $f ] && echo yes" | grep -q yes; then
                adb pull "$f" "$PROF_DIR/"
            fi
        done
        echo "==> pulled available profiles to $PROF_DIR"
        ls -la "$PROF_DIR"
        ;;
    *)
        usage
        exit 1
        ;;
esac
