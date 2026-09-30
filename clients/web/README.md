# Clawy Web Console

The web console is built into **Clawy**. There is one executable to install:
`clawy` (or `clawy.exe` on Windows). No separate launcher is needed.

## Start and Set Up

```bash
clawy start
```

Clawy serves its embedded dashboard at `http://localhost:18800` and opens your
browser. On first run it creates the config and workspace without CLI prompts.
The browser walks you through creating a dashboard password, connecting a model
and credentials, optionally adding messaging channels, and starting chat.
Existing configurations and workspace files are preserved.

Creating the dashboard password also signs you in. The setup checklist lives at
`/setup` and remains available in the sidebar. Models, credentials, channels,
tools, skills, logs, and settings are all managed in the web console.

Running `clawy` without arguments also opens the web console, including when the
executable is opened from a desktop shortcut or the macOS app bundle. Existing
CLI commands such as `clawy agent`, `clawy gateway`, and `clawy onboard` remain
available for advanced use.

Keep the console running while using Clawy. Ctrl+C shuts down the web server and
any gateway it owns. An independently started gateway is not stopped on exit.

### Options

```bash
clawy start --no-browser
clawy start --port 19999 /path/to/config.json
clawy start --host 127.0.0.1 --debug
clawy start --public --no-browser
```

- The default app config is `~/.clawy/config.json`.
- `CLAWY_HOME` overrides the data directory; `CLAWY_CONFIG` overrides the config.
- A positional config path takes precedence over those defaults.
- Explicit networking flags override saved web-console settings.
- `--no-browser` is useful on servers; open the displayed URL manually.
- Default listening addresses are loopback-only. Public access needs an explicit
  networking setting, a dashboard password, and appropriate network restrictions.

For upgrade compatibility, dashboard networking settings still live in
`launcher-config.json` beside the app config. Existing password storage,
`launcher-auth.db` (or the platform fallback), is preserved as well.

## Architecture

- `frontend/`: React + Vite + TanStack Router dashboard.
- `backend/`: importable Go web-console package, called by `clawy start`.
- `backend/dist/`: compiled frontend assets embedded into the Clawy executable.
- `backend/api/`: configuration, model, credential, channel, skill, and runtime APIs.
- `backend/middleware/`: dashboard authentication, HTTP middleware, and IP access controls.

The HTTP server runs in the `clawy start` process. For gateway management it
starts `clawy gateway -E` using **the same executable**, even if that executable
was copied to a new directory or renamed. There is no companion executable or
PATH dependency. `CLAWY_BINARY` remains an optional development override.

The console only starts the gateway when a usable default model is configured.
It captures gateway logs, supports start/stop/restart, and proxies browser chat
WebSockets through `/pico/ws`.

## Authentication and Networking

- `/launcher-setup` creates the dashboard password; `/launcher-login` signs in.
  These existing route names are retained for compatibility.
- Authentication uses an HttpOnly session cookie and a bcrypt password hash.
- Sessions are invalidated when the web-console process restarts.
- Auto-opening a local browser can use a one-shot loopback-only login grant.
- `allowed_cidrs` restricts client IP ranges when configured.
- `allow_localhost_bypass` defaults to true; disable it if same-host proxies
  should not bypass the allowlist.
- `trusted_proxy_cidrs` identifies proxies allowed to supply client IP headers.
  Such proxies must sanitize forwarded headers.
- Legacy dashboard tokens are migrated to password login. URL tokens and bearer
  tokens are not supported for dashboard authentication.

## Build

Requirements: Go matching `go.mod`, Node.js 20.19+ or a supported newer version,
and pnpm (the native script can use npx if pnpm is not installed).

From the repository root:

```bash
./scripts/build-native.sh
./build/clawy start
```

On Windows the output is `build/clawy.exe`. This builds the frontend first, then
embeds it while compiling **one Clawy binary**. The platform-qualified output is
a copy of that same binary, not another component. Stop running Clawy processes
before replacing executables on Windows.

```bash
./scripts/build-native.sh --skip-frontend  # reuse already-built embedded assets
make build                               # also builds the embedded UI
make build-web                           # frontend only
```

A plain `go build ./cmd/clawy` requires `backend/dist` to have been built first
for a working web console. Missing assets produce an actionable startup error,
not a silently blank dashboard. Release builds always build and embed the UI.

## Development and Tests

From `clients/web/`:

```bash
make dev        # build Clawy, then run backend and Vite
make dev-frontend
make dev-backend # runs the same Clawy binary with the start command
make test
make lint
```

Or from the repository root:

```bash
go test -tags goolm,stdjson ./cmd/clawy/... ./clients/web/backend/...
```

The backend APIs and frontend are tested separately; the production frontend is
always embedded in Clawy. Self-update downloads only the `clawy` release binary.
