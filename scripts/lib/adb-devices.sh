# shellcheck shell=sh
# adb-devices.sh — shared helpers for detecting and selecting connected
# NextUI handhelds over ADB. Sourced by deploy.sh and deploy-mods.sh.
#
# A "device record" is a single tab-separated line:  <serial>\t<platform>\t<label>
# e.g.  4c00...2229d<TAB>tg5040<TAB>TrimUI Brick (tg5040)
#
# Detection mirrors the pak's own logic (launch.sh / hardware.DetectPlatform):
#   * platform comes from /proc/device-tree/{model,compatible} + /proc/cpuinfo;
#     the Allwinner sun50iw10 SoC used by every tg5040 unit is treated as tg5040.
#   * within tg5040, Brick (1024x768) and Smart Pro (1280x720) are told apart by
#     framebuffer width, since their device-tree model is identical ("sun50iw10").

TAB="$(printf '\t')"

require_adb() {
    if ! command -v adb >/dev/null 2>&1; then
        echo "ERROR: adb not found. Install android-tools (or android-platform-tools)." >&2
        return 1
    fi
}

# adb_ready_serials: echo the serial of every device in the "device" state, one
# per line. Devices in any other state (unauthorized, offline, ...) are noted on
# stderr and skipped.
adb_ready_serials() {
    adb devices | awk 'NR>1 && NF>=2 && $2!="device" {
        print "WARN: ignoring " $1 " (" $2 ")" > "/dev/stderr"
    }
    NR>1 && $2=="device" {print $1}'
}

# model_to_platform "<blob>": map a blob of device-tree model/compatible/cpuinfo
# text to a platform id. Echoes tg5040, tg5050, or "" (empty = unrecognized).
# Mirrors hardware.DetectPlatformFromMetadata; "SMART PRO S" must win over the
# "SMART PRO" substring, so the tg5050 case is tested first.
model_to_platform() {
    hay=$(printf '%s' "$1" | tr '[:lower:]' '[:upper:]')
    case "$hay" in
        *TG5050*|*"SMART PRO S"*)                       echo "tg5050" ;;
        *TG5040*|*BRICK*|*"SMART PRO"*|*SUN50IW10*)     echo "tg5040" ;;
        *)                                              echo "" ;;
    esac
}

# device_name <platform> <fb_width>: friendly model name. Within tg5040 the
# framebuffer width disambiguates Brick from Smart Pro.
device_name() {
    case "$1" in
        tg5050) echo "TrimUI Smart Pro S" ;;
        tg5040)
            case "$2" in
                1024) echo "TrimUI Brick" ;;
                1280) echo "TrimUI Smart Pro" ;;
                *)    echo "TrimUI (tg5040)" ;;
            esac ;;
        *) echo "Unknown device" ;;
    esac
}

# detect_device <serial>: echo one device record for a supported handheld, or
# return 1 if the device is not a recognized BMO target. One adb round trip.
detect_device() {
    serial=$1
    blob=$(adb -s "$serial" shell '
        tr -d "\000" </proc/device-tree/model 2>/dev/null; echo;
        tr -d "\000" </proc/device-tree/compatible 2>/dev/null; echo;
        grep -i "tg5050\|smart pro s" /proc/cpuinfo 2>/dev/null;
        echo "FBMODES:$(cat /sys/class/graphics/fb0/modes 2>/dev/null)"
    ' 2>/dev/null)

    platform=$(model_to_platform "$blob")
    [ -n "$platform" ] || return 1

    width=$(printf '%s' "$blob" | sed -n 's/.*FBMODES:[^0-9]*\([0-9]\{3,\}\)x.*/\1/p' | head -1)
    label="$(device_name "$platform" "$width") ($platform)"
    printf '%s%s%s%s%s\n' "$serial" "$TAB" "$platform" "$TAB" "$label"
}

# print_supported_devices: echo a device record for every connected, recognized
# handheld. Unrecognized-but-connected devices are noted on stderr.
print_supported_devices() {
    for serial in $(adb_ready_serials); do
        if rec=$(detect_device "$serial"); then
            printf '%s\n' "$rec"
        else
            echo "WARN: $serial is connected but not a recognized BMO device; skipping." >&2
        fi
    done
}

# choose_targets "<records>": given the newline-separated device records, echo
# the subset to act on. Honors DEVICE_FILTER (--device) and SELECT_ALL (--all);
# falls back to an interactive menu on a TTY when several devices are present,
# or to "all" when non-interactive. Returns 1 on no/ambiguous match.
choose_targets() {
    list=$(printf '%s\n' "$1" | sed '/^$/d')
    count=$(printf '%s\n' "$list" | sed '/^$/d' | wc -l | tr -d ' ')
    if [ "$count" -eq 0 ]; then
        echo "ERROR: no supported devices detected. Check the USB cable / authorization." >&2
        return 1
    fi

    if [ -n "${DEVICE_FILTER:-}" ]; then
        f=$(printf '%s' "$DEVICE_FILTER" | tr '[:upper:]' '[:lower:]')
        matches=""
        while IFS="$TAB" read -r serial platform label; do
            [ -n "$serial" ] || continue
            sl=$(printf '%s' "$serial" | tr '[:upper:]' '[:lower:]')
            ll=$(printf '%s' "$label" | tr '[:upper:]' '[:lower:]')
            if [ "$sl" = "$f" ]; then
                matches="$serial$TAB$platform$TAB$label"
                break
            fi
            case "$ll" in
                *"$f"*) matches="${matches:+$matches
}$serial$TAB$platform$TAB$label" ;;
            esac
        done <<EOF
$list
EOF
        mc=$(printf '%s\n' "$matches" | sed '/^$/d' | wc -l | tr -d ' ')
        if [ "$mc" -eq 0 ]; then
            echo "ERROR: --device '$DEVICE_FILTER' matched no connected device." >&2
            return 1
        fi
        if [ "$mc" -gt 1 ]; then
            echo "ERROR: --device '$DEVICE_FILTER' is ambiguous; matches:" >&2
            printf '%s\n' "$matches" | sed '/^$/d; s/^/  /' >&2
            return 1
        fi
        printf '%s\n' "$matches" | sed '/^$/d'
        return 0
    fi

    if [ "${SELECT_ALL:-0}" = "1" ] || [ "$count" -eq 1 ]; then
        printf '%s\n' "$list"
        return 0
    fi

    # Several devices, no explicit choice.
    if [ -t 0 ] && [ -r /dev/tty ]; then
        {
            echo "Multiple devices detected:"
            i=0
            while IFS="$TAB" read -r serial platform label; do
                [ -n "$serial" ] || continue
                i=$((i + 1))
                printf '  %d) %-26s [%s]\n' "$i" "$label" "$serial"
            done <<EOF
$list
EOF
            echo "  a) All devices"
            printf 'Select [a]: '
        } >/dev/tty
        read -r choice </dev/tty
        case "$choice" in
            ""|a|A|all|ALL) printf '%s\n' "$list" ;;
            *[!0-9]*)
                echo "ERROR: invalid selection '$choice'." >&2
                return 1 ;;
            *)
                sel=$(printf '%s\n' "$list" | sed -n "${choice}p")
                if [ -z "$sel" ]; then
                    echo "ERROR: selection '$choice' out of range." >&2
                    return 1
                fi
                printf '%s\n' "$sel" ;;
        esac
        return 0
    fi

    # Non-interactive with several devices: default to all.
    printf '%s\n' "$list"
}
