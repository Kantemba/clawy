# Your agent, your way

Open **Customize your agent** in the dashboard sidebar (`/setup`) to create
or edit your personal agent. First-time browser chat users are directed here
until they have spawned an identity.

1. Choose a name and avatar (including your own emoji or short text).
2. Describe the agent's purpose, personality, and communication style.
   Personality starters are optional and remain fully editable.
3. Add custom instructions: preferred language, formatting, workflows, and
   boundaries. Do not include credentials or secrets.
4. Set the welcome message and review the identity preview.
5. Select **Spawn**, connect a default AI model, and **Bring your agent to life**.
   If the runtime is already running, simply open chat after saving.

The saved identity appears in the dashboard header, browser tab title, chat
heading, assistant message labels, welcome screen, and message composer.
External services such as Discord or Slack manage their own bot display names;
change those names in the service's app settings if desired.

## Persistence and runtime behavior

The profile is saved atomically to `IDENTITY.json` in the default routed agent's
workspace. Explicit agent workspace overrides and named-agent workspace paths
are honored. It persists across restarts and browsers; it is not a local-storage
preference. Other agents can have independent profiles in their own workspaces.

The runtime uses the profile's name, role, personality, and custom instructions
in its system prompt. It supersedes conflicting legacy names and personality
in `AGENT.md`, `AGENTS.md`, `SOUL.md`, or `IDENTITY.md`, without rewriting those
files or removing their other workspace instructions. Safety and tool permission
rules remain in force. Profile changes invalidate the system-prompt cache and
apply on the next message; a gateway restart is not required. Agent discovery
also reports saved names and roles.

Workspaces without a saved profile retain the Clawy defaults. Invalid profile
files are reported by the dashboard; the runtime logs a warning and falls back
to defaults rather than failing a conversation.

## Dashboard API

- `GET /api/agent/identity` returns the default agent's saved identity or defaults
  with `configured: false`.
- `PUT /api/agent/identity` validates and saves the identity, returning the
  normalized profile with `configured: true`.

Both endpoints use the existing dashboard authentication boundary. Fields are
`name`, `avatar`, `role`, `personality`, `instructions`, `greeting`, and
`configured`. Saving always sets `configured` to true. A name is required;
text fields are bounded and single-line fields reject control characters.
