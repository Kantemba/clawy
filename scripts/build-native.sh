#!/bin/bash
# Build the clawy binary natively (no Docker / no cross-compile).
# Mirrors `make build`.

set -e

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

BINARY_NAME=clawy
BUILD_DIR=build
CMD_DIR=cmd/${BINARY_NAME}
CONFIG_PKG=github.com/Kantemba/clawy/pkg/config
GO_BUILD_TAGS=${GO_BUILD_TAGS:-goolm,stdjson}
EXT=
PLATFORM="$(go env GOHOSTOS)"
ARCH="$(go env GOHOSTARCH)"
if [ "$PLATFORM" = "windows" ]; then
    EXT=.exe
fi

VERSION="$(git describe --tags --always --dirty 2>/dev/null || echo dev)"
GIT_COMMIT="$(git rev-parse --short=8 HEAD 2>/dev/null || echo dev)"
BUILD_TIME="$(date +%FT%T%z)"
GO_VERSION="$(go env GOVERSION)"

LDFLAGS="-X ${CONFIG_PKG}.Version=${VERSION} \
-X ${CONFIG_PKG}.GitCommit=${GIT_COMMIT} \
-X ${CONFIG_PKG}.BuildTime=${BUILD_TIME} \
-X ${CONFIG_PKG}.GoVersion=${GO_VERSION} -s -w"

echo "Building ${BINARY_NAME}${EXT} for ${PLATFORM}/${ARCH}..."
echo "  Version:   ${VERSION}"
echo "  GitCommit: ${GIT_COMMIT}"
echo "  Tags:      ${GO_BUILD_TAGS}"

mkdir -p "${BUILD_DIR}"

CGO_ENABLED=0 GOOS="${PLATFORM}" GOARCH="${ARCH}" \
    go build -tags "${GO_BUILD_TAGS}" \
    -ldflags "${LDFLAGS}" \
    -o "${BUILD_DIR}/${BINARY_NAME}-${PLATFORM}-${ARCH}${EXT}" \
    "./${CMD_DIR}"

cp "${BUILD_DIR}/${BINARY_NAME}-${PLATFORM}-${ARCH}${EXT}" \
    "${BUILD_DIR}/${BINARY_NAME}${EXT}"

echo "Build complete: ${BUILD_DIR}/${BINARY_NAME}${EXT}"