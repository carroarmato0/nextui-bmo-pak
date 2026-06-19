#!/usr/bin/env bash
# Fetch the pinned onnxruntime shared library (linux-aarch64) and openWakeWord
# ONNX models used by the on-device wake-word detector. Artifacts land in the
# gitignored third_party/wakeword/ cache and are bundled into the pak by
# scripts/release.sh. Re-running is cheap: existing files are left in place.
#
# Versions and rationale: docs/superpowers/2026-06-19-p2.0-wakeword-feasibility-findings.md
set -euo pipefail

ORT_VERSION="1.26.0"
OWW_VERSION="v0.5.1"

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
DEST="$ROOT/third_party/wakeword"
MODELS="$DEST/models"
mkdir -p "$DEST" "$MODELS"

ORT_TGZ="onnxruntime-linux-aarch64-${ORT_VERSION}.tgz"
ORT_URL="https://github.com/microsoft/onnxruntime/releases/download/v${ORT_VERSION}/${ORT_TGZ}"
OWW_BASE="https://github.com/dscripka/openWakeWord/releases/download/${OWW_VERSION}"

fetch() { # url dest
    if [ -f "$2" ]; then
        echo "have $(basename "$2")"
        return
    fi
    echo "fetch $(basename "$2")"
    curl -fsSL -o "$2" "$1"
}

# onnxruntime aarch64 shared library
if [ ! -f "$DEST/libonnxruntime.so" ]; then
    tmp="$(mktemp -d)"
    echo "fetch ${ORT_TGZ}"
    curl -fsSL -o "$tmp/ort.tgz" "$ORT_URL"
    tar xzf "$tmp/ort.tgz" -C "$tmp"
    so="$(find "$tmp" -name 'libonnxruntime.so.*' ! -name '*.so' | head -1)"
    cp "$so" "$DEST/libonnxruntime.so"
    rm -rf "$tmp"
    echo "have libonnxruntime.so"
fi

# openWakeWord shared pipeline + a stock wake classifier (hey_jarvis) used as
# the "Hey BMO" placeholder until a dedicated model is trained.
fetch "$OWW_BASE/melspectrogram.onnx" "$MODELS/melspectrogram.onnx"
fetch "$OWW_BASE/embedding_model.onnx" "$MODELS/embedding_model.onnx"
fetch "$OWW_BASE/hey_jarvis_v0.1.onnx" "$MODELS/hey_bmo.onnx"

echo "wake-word assets ready in $DEST"
