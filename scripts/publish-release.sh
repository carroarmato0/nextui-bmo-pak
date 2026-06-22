#!/bin/sh
set -e

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
cd "$SCRIPT_DIR/.."

if [ "${1:-}" = "--help" ] || [ "${1:-}" = "-h" ]; then
    cat <<'EOF'
Usage: publish-release.sh [tag] [--title "<title>"]

Publish (or update) the GitHub release for a tag, attaching every artifact built
by release.sh: the pak bundles AND every example mod archive. The mods are always
part of the release — a missing dist/mods/ is a hard error, so a release can
never silently ship without them.

Artifacts (must already exist — run scripts/release.sh first):
  dist/BMO.pak.zip   single combined pak
  dist/BMO.pakz      multi-device bundle
  dist/mods/*.zip    one archive per example mod

Arguments:
  tag                 release tag (default: the exact git tag on HEAD)

Options:
  --title "<title>"   release title for a NEW release (default: "BMO <tag>")

For a new release the notes are taken from the matching changelog entry in
pak.json. Re-running for an existing release only re-uploads the artifacts
(gh upload --clobber) and leaves its title/notes untouched.
EOF
    exit 0
fi

TAG=""
TITLE=""
while [ $# -gt 0 ]; do
    case "$1" in
        --title) TITLE="$2"; shift 2 ;;
        -*) echo "Unknown option: $1" >&2; exit 1 ;;
        *)
            if [ -z "$TAG" ]; then TAG="$1"; shift
            else echo "Unexpected argument: $1" >&2; exit 1; fi
            ;;
    esac
done

if ! command -v gh >/dev/null 2>&1; then
    echo "ERROR: gh (GitHub CLI) is required." >&2
    exit 1
fi

if [ -z "$TAG" ]; then
    TAG=$(git describe --tags --exact-match HEAD 2>/dev/null || true)
fi
if [ -z "$TAG" ]; then
    echo "ERROR: no tag given and HEAD is not exactly tagged." >&2
    echo "       Pass a tag, e.g. publish-release.sh v1.0.1" >&2
    exit 1
fi
[ -n "$TITLE" ] || TITLE="BMO $TAG"

# Required pak bundles.
ASSETS=""
for f in dist/BMO.pak.zip dist/BMO.pakz; do
    if [ ! -f "$f" ]; then
        echo "ERROR: missing $f — run scripts/release.sh first." >&2
        exit 1
    fi
    ASSETS="$ASSETS $f"
done

# Every example mod archive. This is the whole point of the script: mods are
# always attached. No mod archives is a hard error so a release never ships
# without them.
modcount=0
for f in dist/mods/*.zip; do
    [ -f "$f" ] || continue
    ASSETS="$ASSETS $f"
    modcount=$((modcount + 1))
done
if [ "$modcount" -eq 0 ]; then
    echo "ERROR: no mod archives in dist/mods/ — run scripts/release.sh first." >&2
    exit 1
fi
echo "==> Attaching $modcount mod archive(s) to $TAG."

# An existing release may carry a hand-written title/notes; only refresh its
# assets so a re-run can add the mods without clobbering that.
if gh release view "$TAG" >/dev/null 2>&1; then
    echo "==> Updating assets on existing release $TAG"
    # shellcheck disable=SC2086
    gh release upload "$TAG" $ASSETS --clobber
    echo "==> Done. Updated $TAG with assets:$ASSETS"
    exit 0
fi

# New release: derive notes from pak.json's changelog entry for this tag, plus
# install instructions and (best-effort) a compare link to the previous tag.
CHANGELOG=$(jq -r --arg v "$TAG" '.changelog[$v] // empty' pak.json)
REPO=$(gh repo view --json nameWithOwner -q .nameWithOwner 2>/dev/null || true)
PREV_TAG=$(git describe --tags --abbrev=0 "$TAG^" 2>/dev/null || true)
NOTES_FILE=$(mktemp)
{
    if [ -n "$CHANGELOG" ]; then
        printf '## What'\''s new\n\n%s\n\n' "$CHANGELOG"
    fi
    echo "## Install"
    echo
    echo "- **Single device:** download \`BMO.pak.zip\` and extract it into \`Tools/<platform>/\` on your SD card."
    echo "- **Multi-device bundle:** download \`BMO.pakz\` (both tg5040 and tg5050 builds)."
    echo "- **Mods:** download any \`<mod>.zip\` and unzip it into your device's \`BMO/mods/\` folder."
    if [ -n "$PREV_TAG" ] && [ -n "$REPO" ]; then
        printf '\n**Full changelog:** https://github.com/%s/compare/%s...%s\n' "$REPO" "$PREV_TAG" "$TAG"
    fi
} > "$NOTES_FILE"

echo "==> Creating release $TAG"
# shellcheck disable=SC2086
gh release create "$TAG" $ASSETS --title "$TITLE" --notes-file "$NOTES_FILE"
rm -f "$NOTES_FILE"
echo "==> Done. Published $TAG with assets:$ASSETS"
