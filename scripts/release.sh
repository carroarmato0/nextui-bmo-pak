#!/bin/sh
set -e

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
cd "$SCRIPT_DIR/.."

if [ "${1:-}" = "--help" ] || [ "${1:-}" = "-h" ]; then
    cat <<'EOF'
Usage: release.sh [--runtime docker|podman]

Run tests, cross-compile the BMO binary for the tg5040/tg5050 family using
the LoveRetro platform toolchain containers, and assemble the release archives.

Output in dist/:
  BMO.pak.zip   Single combined pak (both platform binaries inside) zipped at
                top level. This is the Pak Store release_filename; a user
                extracts it into Tools/<platform>/ on the SD card.
  BMO.pakz      Multi-device bundle: the Tools/<platform>/BMO.pak tree (one
                directory per platform) zipped with the Tools/ prefix, for
                manual SD-card-root install / Pak Store recognition.

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
# Build version (shown on the About screen). Priority:
#   1. "<sha>-dirty" when the working tree has uncommitted changes
#   2. the exact git tag on HEAD, if any
#   3. the short commit SHA otherwise
GIT_COMMIT=$(git rev-parse --short HEAD 2>/dev/null || echo "unknown")
GIT_TAG=$(git describe --tags --exact-match HEAD 2>/dev/null || true)
if [ "$GIT_COMMIT" = "unknown" ]; then
    VERSION="unknown"
elif [ -n "$(git status --porcelain 2>/dev/null)" ]; then
    VERSION="${GIT_COMMIT}-dirty"
elif [ -n "$GIT_TAG" ]; then
    VERSION="$GIT_TAG"
else
    VERSION="$GIT_COMMIT"
fi
echo "==> Build version: $VERSION"

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
        -e VERSION="$VERSION" \
        "bmo-pak-${platform}-dev" \
        sh -c "
            mkdir -p bin/${platform} lib/${platform}
            CGO_ENABLED=1 GOOS=linux GOARCH=arm64 \
                go build -a -tags netgo -buildvcs=false \
                -ldflags \"-X github.com/carroarmato0/nextui-bmo/internal/buildinfo.Version=\$VERSION\" \
                -o bin/${platform}/bmo-pak ./cmd/bmo-pak
            echo 'Built: bin/${platform}/bmo-pak'
            SDL2_SO=\$(ls \"\$SYSROOT/usr/lib\"/libSDL2-2.0.so.0.* 2>/dev/null | grep -v '\.so\$' | head -1)
            if [ -n \"\$SDL2_SO\" ]; then
                cp \"\$SDL2_SO\" lib/${platform}/libSDL2-2.0.so.0
                echo 'Bundled: lib/${platform}/libSDL2-2.0.so.0'
            fi
        "
}

echo "==> Fetching wake-word assets (onnxruntime + openWakeWord models)..."
"$(dirname "$0")/fetch-wakeword-assets.sh"

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
    # Wake-word: bundle the onnxruntime shared library (aarch64, same for both
    # platforms) and the openWakeWord models fetched by fetch-wakeword-assets.sh.
    if [ -f third_party/wakeword/libonnxruntime.so ]; then
        mkdir -p "$dest/lib/$platform" "$dest/assets/wakeword"
        cp third_party/wakeword/libonnxruntime.so "$dest/lib/$platform/libonnxruntime.so"
        cp third_party/wakeword/models/*.onnx "$dest/assets/wakeword/"
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
# Wake-word onnxruntime library for the tg5050 binary in the combined pak.
if [ -f third_party/wakeword/libonnxruntime.so ]; then
    mkdir -p dist/BMO.pak/lib/tg5050
    cp third_party/wakeword/libonnxruntime.so dist/BMO.pak/lib/tg5050/libonnxruntime.so
fi
copy_pak tg5040 dist/all/Tools/tg5040/BMO.pak
copy_pak tg5050 dist/all/Tools/tg5050/BMO.pak

echo "==> Creating release archives..."
rm -f dist/BMO.pak.zip dist/BMO.pakz

# Single combined pak zipped at top level (Pak Store release_filename).
# Contains both platform binaries + libs so one zip installs on any device.
(
    cd dist/BMO.pak
    zip -qr ../BMO.pak.zip .
) &
pid_zip=$!

# Multi-device bundle (.pakz): the Tools/<platform>/BMO.pak tree zipped with
# the Tools/ prefix, one directory per platform, for SD-card-root install.
(
    cd dist/all
    zip -qr ../BMO.pakz Tools
) &
pid_pakz=$!

wait $pid_zip
wait $pid_pakz

# Package each example mod as its own distributable archive. The zip contains a
# top-level <name>/ directory, so a user unzips it straight into their device's
# BMO/mods/ folder (matching the install flow in docs/MODDING.md).
if [ -d examples/mods ]; then
    echo "==> Packaging example mods..."
    mkdir -p dist/mods
    for moddir in examples/mods/*/; do
        [ -d "$moddir" ] || continue
        modname=$(basename "$moddir")
        rm -f "dist/mods/$modname.zip"
        (
            cd examples/mods
            zip -qr "../../dist/mods/$modname.zip" "$modname"
        )
        echo "Packaged: dist/mods/$modname.zip"
    done
fi

echo "==> Release artifacts:"
find dist -maxdepth 5 \( -name '*.zip' -o -name '*.pakz' -o -name 'bmo-pak' -o -name '*.so*' \) | sort
