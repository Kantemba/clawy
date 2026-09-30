#!/bin/bash
# Build one Clawy executable with the web console and CLI embedded.
set -euo pipefail

case "${1:-}" in
    --skip-frontend) export SKIP_WEB_BUILD=1; shift ;;
    -h|--help)
        echo "Usage: $0 [--skip-frontend]"
        echo "Builds one clawy executable, including the web console."
        echo "--skip-frontend reuses an already-built embedded UI."
        exit 0
        ;;
esac
if [ "$#" -ne 0 ]; then
    echo "Usage: $0 [--skip-frontend]" >&2
    exit 1
fi

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"
GO_BUILD_TAGS=${GO_BUILD_TAGS:-goolm,stdjson}
PLATFORM="$(go env GOHOSTOS)"
ARCH="$(go env GOHOSTARCH)"
EXT=
if [ "$PLATFORM" = "windows" ]; then EXT=.exe; fi

CONFIG_PKG=github.com/Kantemba/clawy/pkg/config
VERSION="$(git describe --tags --always --dirty 2>/dev/null || echo dev)"
GIT_COMMIT="$(git rev-parse --short=8 HEAD 2>/dev/null || echo dev)"
BUILD_TIME="$(date +%FT%T%z)"
GO_VERSION="$(go env GOVERSION)"
LDFLAGS="-X ${CONFIG_PKG}.Version=${VERSION} \
-X ${CONFIG_PKG}.GitCommit=${GIT_COMMIT} \
-X ${CONFIG_PKG}.BuildTime=${BUILD_TIME} \
-X ${CONFIG_PKG}.GoVersion=${GO_VERSION} -s -w"

bash scripts/build-web.sh

echo "Building clawy${EXT} for ${PLATFORM}/${ARCH} (CLI + web console)..."
echo "  Version: ${VERSION}"
echo "  Tags:    ${GO_BUILD_TAGS}"
mkdir -p build
SOURCE="build/clawy-${PLATFORM}-${ARCH}${EXT}"
TARGET="build/clawy${EXT}"
CGO_ENABLED=0 GOOS="${PLATFORM}" GOARCH="${ARCH}" \
    go build -tags "${GO_BUILD_TAGS}" -ldflags "${LDFLAGS}" \
    -o "$SOURCE" ./cmd/clawy

if ! cp "$SOURCE" "$TARGET"; then
    echo "Cannot replace $TARGET. Stop any running Clawy processes, then rerun this build." >&2
    echo "Windows locks active executables; you may still be running the old binary." >&2
    echo "Newly compiled binary: $SOURCE" >&2
    exit 1
fi

echo "Build complete: $TARGET (no separate launcher needed)"
echo "Run: ./$TARGET start"
