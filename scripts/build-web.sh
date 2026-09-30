#!/bin/bash
# Compile the web UI before embedding it in the single Clawy executable.
set -euo pipefail
ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR/clients/web/frontend"

if [ "${SKIP_WEB_BUILD:-0}" = "1" ]; then
    if [ ! -f ../backend/dist/index.html ]; then
        echo "Embedded web UI is missing. Run scripts/build-web.sh first." >&2
        exit 1
    fi
    exit 0
fi

if ! command -v node >/dev/null 2>&1; then
    echo "Node.js 20.19+ is required to build Clawy's embedded web UI." >&2
    exit 1
fi
if [ ! -f node_modules/typescript/bin/tsc ] || [ ! -f node_modules/vite/bin/vite.js ]; then
    if command -v pnpm >/dev/null 2>&1; then
        CI=true pnpm install --frozen-lockfile
    else
        CI=true npx --yes pnpm@10.33.0 install --frozen-lockfile
    fi
fi
# Generate the route tree before type-checking new routes.
node node_modules/vite/bin/vite.js build --outDir ../backend/dist --emptyOutDir
node scripts/ensure-backend-gitkeep.cjs
node node_modules/typescript/bin/tsc -b
