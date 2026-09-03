#!/bin/sh
# Clawy install script — downloads the right GitHub Release artifact.
#
# Usage:
#   curl -fsSL https://raw.githubusercontent.com/Kantemba/clawy/main/scripts/install.sh | sh
#   curl -fsSL .../install.sh | sh -s -- --version v0.3.0 --dir ~/.local/bin
#   CLAWY_REPO=myfork/clawy sh scripts/install.sh
#
# Flags:
#   --version <tag>   Release tag (default: latest stable)
#   --dir <path>      Install directory (default: ~/.local/bin)
#   --no-launcher     Install only the `clawy` CLI, skip clawy-launcher
#   --help            Show this help
set -eu

REPO="${CLAWY_REPO:-Kantemba/clawy}"
VERSION=""
INSTALL_DIR="${INSTALL_DIR:-$HOME/.local/bin}"
WITH_LAUNCHER=1

usage() {
  sed -n '2,/^set -eu/p' "$0" | sed 's/^# \{0,1\}//'
}

while [ $# -gt 0 ]; do
  case "$1" in
    --version) VERSION="$2"; shift 2 ;;
    --dir) INSTALL_DIR="$2"; shift 2 ;;
    --no-launcher) WITH_LAUNCHER=0; shift ;;
    -h|--help) usage; exit 0 ;;
    *) echo "Unknown flag: $1" >&2; usage >&2; exit 1 ;;
  esac
done

need() { command -v "$1" >/dev/null 2>&1 || { echo "Missing required tool: $1" >&2; exit 1; }; }
need curl
need tar

OS="$(uname -s)"
ARCH="$(uname -m)"
case "$OS" in
  Linux) GOOS="Linux" ;;
  Darwin) GOOS="macOS" ;;
  MINGW*|MSYS*|CYGWIN*|Windows_NT) GOOS="Windows" ;;
  *) echo "Unsupported OS: $OS" >&2; exit 1 ;;
esac
case "$ARCH" in
  x86_64|amd64) GOARCH="x86_64" ;;
  arm64|aarch64) GOARCH="arm64" ;;
  riscv64) GOARCH="riscv64" ;;
  loongarch64) GOARCH="loong64" ;;
  armv7l|armv6l) GOARCH="armv7" ;;
  mipsel) GOARCH="mipsle" ;;
  *) echo "Unsupported arch: $ARCH (try a manual download from GitHub Releases)" >&2; exit 1 ;;
esac

if [ -z "$VERSION" ]; then
  echo "Resolving latest release for ${REPO}..."
  VERSION="$(curl -fsSL "https://api.github.com/repos/${REPO}/releases/latest" | grep -m1 '"tag_name"' | cut -d'"' -f4)"
  [ -n "$VERSION" ] || { echo "Could not resolve latest release" >&2; exit 1; }
fi
echo "Installing clawy ${VERSION} (${GOOS}/${GOARCH})..."

TMPDIR="$(mktemp -d)"
trap 'rm -rf "$TMPDIR"' EXIT INT TERM

if [ "$GOOS" = "Windows" ]; then
  ASSET="clawy_Windows_${GOARCH}.zip"
else
  ASSET="clawy_${GOOS}_${GOARCH}.tar.gz"
fi
URL="https://github.com/${REPO}/releases/download/${VERSION}/${ASSET}"
echo "Downloading ${URL}..."
curl -fsSL --retry 3 -o "$TMPDIR/$ASSET" "$URL"

# Verify checksum when the release provides checksums.txt.
if curl -fsSL --retry 2 -o "$TMPDIR/checksums.txt" "https://github.com/${REPO}/releases/download/${VERSION}/checksums.txt" 2>/dev/null; then
  if command -v sha256sum >/dev/null 2>&1; then
    (cd "$TMPDIR" && sha256sum -c --status <(grep -F "$ASSET" checksums.txt) 2>/dev/null) \
      && echo "Checksum OK." \
      || echo "Warning: checksum verification skipped/failed; continuing." >&2
  fi
fi

mkdir -p "$TMPDIR/extract"
if [ "$GOOS" = "Windows" ]; then
  need unzip
  unzip -q -o "$TMPDIR/$ASSET" -d "$TMPDIR/extract"
else
  tar -xzf "$TMPDIR/$ASSET" -C "$TMPDIR/extract"
fi

mkdir -p "$INSTALL_DIR"
BIN_EXT=""
[ "$GOOS" = "Windows" ] && BIN_EXT=".exe"

install_bin() {
  src="$(find "$TMPDIR/extract" -name "$1" -type f | head -n 1)"
  [ -n "$src" ] || { echo "Binary $1 not found in ${ASSET}" >&2; exit 1; }
  cp -f "$src" "$INSTALL_DIR/$1"
  chmod +x "$INSTALL_DIR/$1"
  echo "Installed $INSTALL_DIR/$1"
}

install_bin "clawy${BIN_EXT}"
if [ "$WITH_LAUNCHER" = "1" ]; then
  if find "$TMPDIR/extract" -name "clawy-launcher${BIN_EXT}" -type f | grep -q .; then
    install_bin "clawy-launcher${BIN_EXT}"
  else
    echo "Note: clawy-launcher not in this archive; skipping."
  fi
fi

echo ""
echo "Done. Make sure $INSTALL_DIR is on your PATH:"
echo "  export PATH=\"$INSTALL_DIR:\$PATH\""
echo ""
echo "Next: clawy onboard"
echo "Updates: clawy update --check  (disable notices: CLAWY_NO_UPDATE_CHECK=1)"
