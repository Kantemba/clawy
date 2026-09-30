#!/bin/bash
# Cross-compile the clawy binary for Linux.
#
# By default this builds for linux/amd64 (the most common Linux target). Pass
# an architecture explicitly to build a single target, or pass `all` to build
# every supported Linux architecture:
#
#   ./scripts/build-linux.sh              # linux/amd64
#   ./scripts/build-linux.sh arm64        # linux/arm64
#   ./scripts/build-linux.sh arm          # linux/arm (GOARM=7, e.g. Raspberry Pi Zero 2 W)
#   ./scripts/build-linux.sh all          # amd64 + arm64 + arm
#
# Override the toolchain / tags via the usual env vars:
#   GO                   # go binary (default: go)
#   GO_BUILD_TAGS          # comma-separated build tags (default: goolm,stdjson)
#   VERSION / GIT_COMMIT   # override version metadata (default: derived from git)
#
# Mirrors `make build-linux-*` but is usable from a non-Make host (e.g. macOS
# or Windows with Git Bash) without depending on `make`.

set -e

GO="${GO:-go}"
BINARY_NAME=clawy
BUILD_DIR=build
CMD_DIR=cmd/${BINARY_NAME}
CONFIG_PKG=github.com/Kantemba/clawy/pkg/config
GO_BUILD_TAGS=${GO_BUILD_TAGS:-goolm,stdjson}

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

VERSION="$(git describe --tags --always --dirty 2>/dev/null || echo dev)"
GIT_COMMIT="$(git rev-parse --short=8 HEAD 2>/dev/null || echo dev)"
BUILD_TIME="$(date +%FT%T%z)"
GO_VERSION="$(go env GOVERSION)"

LDFLAGS="-X ${CONFIG_PKG}.Version=${VERSION} \
-X ${CONFIG_PKG}.GitCommit=${GIT_COMMIT} \
-X ${CONFIG_PKG}.BuildTime=${BUILD_TIME} \
-X ${CONFIG_PKG}.GoVersion=${GO_VERSION} -s -w"

bash scripts/build-web.sh
mkdir -p "${BUILD_DIR}"

build_linux() {
    local arch="$1"
    local goarm="${2:-}"
    local out="${BUILD_DIR}/${BINARY_NAME}-linux-${arch}"
    local env_prefix="CGO_ENABLED=0 GOOS=linux GOARCH=${arch}"
    [ -n "${goarm}" ] && env_prefix="${env_prefix} GOARM=${goarm}"

    echo "Building ${BINARY_NAME} for linux/${arch}${goarm:+,${goarm}}..."
    env $env_prefix ${GO} build -tags "${GO_BUILD_TAGS}" \
        -ldflags "${LDFLAGS}" \
        -o "${out}" "./${CMD_DIR}"
    echo "Build complete: ${out}"
}

target="${1:-}"

case "${target}" in
    ""|amd64)
        build_linux amd64
        ;;
    arm64|aarch64)
        build_linux arm64
        ;;
    arm|armv7)
        build_linux arm 7
        ;;
    all)
        build_linux amd64
        build_linux arm64
        build_linux arm 7
        ;;
    *)
        echo "Usage: $0 [amd64|arm64|arm|all]" >&2
        exit 1
        ;;
esac
