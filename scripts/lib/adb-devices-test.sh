#!/bin/sh
# Self-test for the pure (no-adb) helpers in adb-devices.sh.
# Run: sh scripts/lib/adb-devices-test.sh
set -e

DIR="$(cd "$(dirname "$0")" && pwd)"
. "$DIR/adb-devices.sh"

fail=0
check() {
    desc=$1 got=$2 want=$3
    if [ "$got" = "$want" ]; then
        echo "ok   - $desc"
    else
        echo "FAIL - $desc: got '$got' want '$want'"
        fail=1
    fi
}

# model_to_platform: real strings seen on devices + the marker keywords.
check "tg5040 from sun50iw10 compatible" \
    "$(model_to_platform 'sun50iw10 allwinner,a133arm,sun50iw10p1')" tg5040
check "tg5040 from BRICK keyword" \
    "$(model_to_platform 'TrimUI Brick')" tg5040
check "tg5040 from SMART PRO keyword" \
    "$(model_to_platform 'TrimUI Smart Pro')" tg5040
check "tg5050 wins over SMART PRO substring" \
    "$(model_to_platform 'TrimUI Smart Pro S')" tg5050
check "tg5050 from TG5050 keyword" \
    "$(model_to_platform 'foo TG5050 bar')" tg5050
check "unrecognized -> empty" \
    "$(model_to_platform 'Pixel 7 google,panther')" ""

# device_name: framebuffer width disambiguates within tg5040.
check "Brick from 1024 width"      "$(device_name tg5040 1024)" "TrimUI Brick"
check "Smart Pro from 1280 width"  "$(device_name tg5040 1280)" "TrimUI Smart Pro"
check "tg5040 unknown width"       "$(device_name tg5040 '')"   "TrimUI (tg5040)"
check "Smart Pro S from tg5050"    "$(device_name tg5050 1280)" "TrimUI Smart Pro S"

if [ "$fail" -ne 0 ]; then
    echo "FAILED"
    exit 1
fi
echo "All passed."
