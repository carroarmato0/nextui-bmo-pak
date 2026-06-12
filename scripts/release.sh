#!/bin/sh
set -e

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
cd "$SCRIPT_DIR/.."

if [ "${1:-}" = "--help" ] || [ "${1:-}" = "-h" ]; then
    cat <<'EOF'
Usage: release.sh [--runtime docker|podman]

Run tests, cross-compile the BMO binary for the tg5040/tg5050 family using
the LoveRetro platform toolchain containers, and assemble dist/BMO.pak plus
dist/all/Tools/<platform>/BMO.pak.

Options:
  --runtime docker|podman   Override container runtime (default: auto-detect, prefers podman)

Environment:
  CONTAINER_RUNTIME=docker|podman   Alternative to --runtime
EOF
    exit 0
fi

detect_runtime() {
    case "${CONTAINER_RUNTIME:-}" in
        docker|podman) echo "$CONTAINER_RUNTIME"; return ;;
    esac
    if command -v podman >/dev/null 2>&1; then echo "podman"
    elif command -v docker >/dev/null 2>&1; then echo "docker"
    else echo ""; fi
}

RUNTIME_OVERRIDE=""
while [ $# -gt 0 ]; do
    case "$1" in
        --runtime) RUNTIME_OVERRIDE="$2"; shift 2 ;;
        *) echo "Unknown option: $1" >&2; exit 1 ;;
    esac
done

RUNTIME="${RUNTIME_OVERRIDE:-$(detect_runtime)}"
if [ -z "$RUNTIME" ]; then
    echo "ERROR: docker or podman required for cross-compilation." >&2
    exit 1
fi

if ! command -v zip >/dev/null 2>&1; then
    echo "ERROR: zip is required to create the release archive." >&2
    exit 1
fi

CACHE_DIR="$(pwd)/.go_cache"
mkdir -p "$CACHE_DIR"
GIT_COMMIT=$(git rev-parse --short HEAD 2>/dev/null || echo "unknown")
if [ "$GIT_COMMIT" != "unknown" ] && ! git diff --quiet 2>/dev/null; then
    GIT_COMMIT="${GIT_COMMIT}-dirty"
fi

DEV_IMAGE="bmo-pak-dev"
ensure_dev_image() {
    $RUNTIME image inspect "$DEV_IMAGE" >/dev/null 2>&1 || \
        $RUNTIME build -t "$DEV_IMAGE" -f docker/Dockerfile.dev .
}

ensure_platform_image() {
    platform="$1"
    tag="bmo-pak-${platform}-dev"
    $RUNTIME image inspect "$tag" >/dev/null 2>&1 || \
        $RUNTIME build -t "$tag" --build-arg "PLATFORM=$platform" \
            -f docker/Dockerfile.platform .
}

echo "==> Running tests..."
ensure_dev_image
$RUNTIME run --rm \
    -v "$(pwd):/workspace" \
    -v "$CACHE_DIR:/go" \
    -w /workspace \
    -e IN_CONTAINER=1 \
    -e GOCACHE=/go/build-cache \
    "$DEV_IMAGE" \
    go test ./...

rm -rf dist bin
mkdir -p bin/tg5040 bin/tg5050
mkdir -p dist/BMO.pak/assets
mkdir -p dist/all/Tools/tg5040/BMO.pak/assets
mkdir -p dist/all/Tools/tg5050/BMO.pak/assets

build_platform() {
    platform="$1"
    ensure_platform_image "$platform"
    $RUNTIME run --rm \
        -v "$(pwd):/workspace" \
        -v "$CACHE_DIR:/go" \
        -w /workspace \
        -e IN_CONTAINER=1 \
        -e GOCACHE=/go/build-cache \
        -e GIT_COMMIT="$GIT_COMMIT" \
        "bmo-pak-${platform}-dev" \
        sh -c "
            mkdir -p bin/${platform} lib/${platform}
            CGO_ENABLED=1 GOOS=linux GOARCH=arm64 \
                go build -a -tags netgo -buildvcs=false \
                -o bin/${platform}/bmo-pak ./cmd/bmo-pak
            echo 'Built: bin/${platform}/bmo-pak'
            SDL2_SO=\$(ls \"\$SYSROOT/usr/lib\"/libSDL2-2.0.so.0.* 2>/dev/null | grep -v '\.so\$' | head -1)
            if [ -n \"\$SDL2_SO\" ]; then
                cp \"\$SDL2_SO\" lib/${platform}/libSDL2-2.0.so.0
                echo 'Bundled: lib/${platform}/libSDL2-2.0.so.0'
            fi
        "
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
    if [ -d "lib/$platform" ] && [ "$(ls -A "lib/$platform" 2>/dev/null)" ]; then
        mkdir -p "$dest/lib/$platform"
        cp "lib/$platform"/libSDL2*.so* "$dest/lib/$platform/" 2>/dev/null || true
    fi
}

echo "==> Assembling release directories..."
copy_pak tg5040 dist/BMO.pak
mkdir -p dist/BMO.pak/bin/tg5050
cp bin/tg5050/bmo-pak dist/BMO.pak/bin/tg5050/bmo-pak
if [ -d lib/tg5050 ] && [ "$(ls -A lib/tg5050 2>/dev/null)" ]; then
    mkdir -p dist/BMO.pak/lib/tg5050
    cp lib/tg5050/libSDL2*.so* dist/BMO.pak/lib/tg5050/ 2>/dev/null || true
fi
copy_pak tg5040 dist/all/Tools/tg5040/BMO.pak
copy_pak tg5050 dist/all/Tools/tg5050/BMO.pak

echo "==> Creating release archive..."
rm -f dist/BMO.pak.zip
(
    cd dist/BMO.pak
    zip -qr ../BMO.pak.zip .
)

echo "==> Release artifacts:"
find dist -maxdepth 5 \( -name 'BMO.pak.zip' -o -name 'bmo-pak' -o -name '*.so*' \) | sort
