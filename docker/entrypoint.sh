#!/bin/sh
set -e

# The same executable serves the web UI and runs the gateway. Web startup
# creates first-run files itself and keeps serving while the user configures it.
if [ "$#" -eq 0 ]; then
    set -- start --public --no-browser
fi

# Retain explicit headless gateway mode for existing deployments.
if [ "$1" = "gateway" ]; then
    if [ ! -f "${CLAWY_CONFIG:-${CLAWY_HOME:-${HOME}/.clawy}/config.json}" ]; then
        clawy onboard
    fi
fi
exec clawy "$@"
