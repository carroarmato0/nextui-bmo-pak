#!/bin/sh
set -e

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
cd "$SCRIPT_DIR/.."

if [ "${1:-}" = "--help" ] || [ "${1:-}" = "-h" ]; then
    cat <<'EOF'
Usage: release.sh

Run tests, cross-compile the BMO binary for the tg5040/tg5050 family,
and assemble dist/BMO.pak plus dist/all/Tools/<platform>/BMO.pak.
EOF
    exit 0
fi

if ! command -v zip >/dev/null 2>&1; then
    echo "ERROR: zip is required to create the release archive." >&2
    exit 1
fi

if ! command -v go >/dev/null 2>&1; then
    echo "ERROR: go is required to build the Pak." >&2
    exit 1
fi

echo "==> Running tests..."
go test ./...

rm -rf dist bin
mkdir -p bin/tg5040 bin/tg5050
mkdir -p dist/BMO.pak/assets
mkdir -p dist/all/Tools/tg5040/BMO.pak/assets
mkdir -p dist/all/Tools/tg5050/BMO.pak/assets

build_platform() {
    platform="$1"
    GOOS=linux GOARCH=arm64 CGO_ENABLED=0         go build -trimpath -buildvcs=false -o "bin/$platform/bmo-pak" ./cmd/bmo-pak
}

echo "==> Building tg5040..."
build_platform tg5040

echo "==> Building tg5050..."
build_platform tg5050

copy_pak() {
    platform="$1"
    dest="$2"
    mkdir -p "$dest/bin/$platform" "$dest/assets"
    cp launch.sh "$dest/launch.sh"
    cp pak.json "$dest/pak.json"
    cp "bin/$platform/bmo-pak" "$dest/bin/$platform/bmo-pak"
    cp -R assets/. "$dest/assets/" 2>/dev/null || true
}

echo "==> Assembling release directories..."
copy_pak tg5040 dist/BMO.pak
mkdir -p dist/BMO.pak/bin/tg5050
cp bin/tg5050/bmo-pak dist/BMO.pak/bin/tg5050/bmo-pak
copy_pak tg5040 dist/all/Tools/tg5040/BMO.pak
copy_pak tg5050 dist/all/Tools/tg5050/BMO.pak

echo "==> Creating release archive..."
rm -f dist/BMO.pak.zip
(
    cd dist/BMO.pak
    zip -qr ../BMO.pak.zip .
)

echo "==> Release artifacts:"
find dist -maxdepth 4 \( -name 'BMO.pak.zip' -o -name 'BMO.pak' -o -name 'bmo-pak' \) | sort
