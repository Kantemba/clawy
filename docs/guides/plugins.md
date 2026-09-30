# Plugins

Clawy loads plugins from plain directories. A plugin adds skills, slash
commands, MCP servers and hooks to a running Clawy **without rebuilding it**.

Two manifest dialects are understood:

| Dialect | Manifest | Source |
| --- | --- | --- |
| Codex | `<plugin>/.codex-plugin/plugin.json` | [github.com/openai/plugins](https://github.com/openai/plugins) |
| Clawy | `<plugin>/clawy-plugin.json` (or `.clawy-plugin/plugin.json`) | native |

Because the Codex dialect is supported, a clone of `openai/plugins` dropped
into any search root works unchanged.

## Quick start

```bash
# 1. Install plugins from a checkout (or a git URL)
clawy plugin install ~/src/plugins            # a checkout that contains plugins/
clawy plugin install https://github.com/openai/plugins
clawy plugin install ./examples/plugins/hello-clawy

# 2. See what Clawy found
clawy plugin list
clawy plugin info notion

# 3. Load them: send /reload in a chat, or restart the gateway
```

Create your own plugin:

```bash
clawy plugin new my-plugin --description "Does something useful"
```

which scaffolds:

```
<workspace>/plugins/my-plugin/
├── clawy-plugin.json
├── skills/my-plugin/SKILL.md
├── commands/my-plugin.md
└── README.md
```

## Search roots

Plugins are discovered from these roots, in order:

1. `$CLAWY_PLUGINS` — `os.PathListSeparator` separated (`,` on Windows, `:` elsewhere)
2. `plugins.dirs` from `config.json`
3. `<workspace>/plugins` — where `clawy plugin new`/`install` write
4. `~/.clawy/plugins`

Each root is scanned at three levels: the root itself, its immediate
children, and the children of a nested `plugins/` directory. That covers both
"the clone *is* the search root" and "the clone sits inside
`~/.clawy/plugins/`".

Config:

```jsonc
{
  "plugins": {
    "enabled": true,            // default true; false disables discovery
    "dirs": ["~/src/my-plugins"],
    "disable": ["some-plugin"]  // skip by name (case-insensitive)
  }
}
```

A manifest with `"enabled": false` is skipped as well. Discovery never fails
startup: a broken manifest is logged as a warning and skipped, and
`clawy plugin list` shows the same warnings.

## Manifest reference

Fields common to both dialects:

| Field | Type | Meaning |
| --- | --- | --- |
| `name` | string | Unique plugin name (defaults to the directory name) |
| `version` | string | Semver-ish version (defaults to `0.0.0`) |
| `description` | string | Shown by `clawy plugin info` |
| `author` | string or object | `"Ada"` or `{"name","email","url"}` |
| `homepage`, `repository`, `license`, `keywords` | — | Metadata only |
| `enabled` | bool | `false` skips the plugin |
| `skills` | string or list | Path(s) to skill roots (default `./skills/`) |
| `commands` | string or list | Path(s) to command files or directories (default `./commands/`) |
| `hooks` | string or list | Path(s) to hook files (default `./hooks.json`) |
| `mcpServers` | string or list or object | Path(s) to MCP config, or an inline server map (default `./.mcp.json`) |

Paths are relative to the plugin directory. Unknown fields (for example the
Codex-only `apps`, `interface`) are tolerated and ignored, and unrecognized
shapes (such as `"hooks": {}`) are treated as "use the defaults" so foreign
manifests still load.

### Surfaces Clawy does not implement yet

| Codex surface | Status |
| --- | --- |
| `apps` / `.app.json` (app connectors) | ignored |
| `interface` (composer UI metadata) | ignored |
| `agents/` (agent definitions) | reserved for a future release |

A plugin that only provides those shows up as `manifest-only` in
`clawy plugin list`. Its metadata still loads; it simply contributes no
skills, commands, MCP servers or hooks.

### Example (Codex dialect)

```json
{
  "name": "notion",
  "version": "0.1.7",
  "description": "Notion workflows",
  "author": { "name": "Notion", "email": "support@openai.com" },
  "skills": "./skills/",
  "mcpServers": "./.mcp.json"
}
```

### Example (Clawy dialect)

```json
{
  "name": "hello-clawy",
  "version": "0.1.0",
  "description": "Minimal Clawy plugin",
  "skills": "./skills/",
  "commands": "./commands/"
}
```

## Surfaces

### Skills — `skills/<name>/SKILL.md`

Skill roots contributed by plugins are appended to every skill loader
(agent, CLI, web backend, evolution). Resolution priority stays:

```
workspace > global (~/.clawy/skills) > plugin > builtin
```

so a hand-installed skill always wins over a plugin's copy. The skill catalog
labels plugin skills with `<source>plugin</source>`, and reading a plugin's
`SKILL.md` activates it exactly like any other skill.

### Commands — `commands/*.md`

A markdown file becomes a **prompt command**: its body is expanded with the
message arguments and forwarded to the LLM instead of running a Go handler.

```markdown
---
description: Deploy a service
argument-hint: "<service> [env]"
---

Deploy {{args}} to production and report the rollout status.
```

Frontmatter keys: `name` (defaults to the file name), `description`,
`usage` / `argument-hint`, `aliases`.

Command files are deliberately forgiving, because command corpora are shared
with other tools:

- a **list** value is accepted where a string is expected
  (`argument-hint: [agent-description]`)
- unknown keys are ignored (`allowed-tools: [Write, Bash]`)
- a block that does not parse is dropped instead of the command
- files starting with `_` or `.` (for example `_conventions.md`) and
  `README`/`LICENSE`/`CHANGELOG` are never registered
- when there is no frontmatter, the description is taken from the first
  paragraph after the leading `#` heading

Placeholders in the body:

| Placeholder | Value |
| --- | --- |
| `{{args}}`, `$ARGUMENTS` | the full argument string |
| `$1` … `$9` | individual argument tokens |
| *(none)* | arguments are appended as a final paragraph |

So `/deploy api prod` with the body above becomes the user message
`Deploy api prod to production and report the rollout status.`

Commands show up in `/help`, `/list commands` and channel bot menus. Builtins
always win a name collision.

### MCP servers — `.mcp.json`

```json
{
  "mcpServers": {
    "echo": { "command": "node", "args": ["server.js"] },
    "figma": { "command": ["uvx", "figma-mcp"] },
    "remote": { "url": "https://example.com/mcp", "type": "streamable-http" }
  }
}
```

Accepted per server: `command` (string **or** array), `args`, `env`,
`env_file` (resolved against the plugin directory), `type`, `url`, `headers`,
`oauth`, `deferred`, `enabled` (defaults to `true`).

Notes:

- `$VAR` / `${VAR}` are expanded from the process environment; unknown
  variables stay literal.
- Server names that clash with a user-configured server are namespaced as
  `<plugin>-<server>`; the plugin's entry never overwrites yours.
- Plugin MCP servers are merged into the config before the agent starts, so
  they behave exactly like hand-configured servers (connect, `/mcp`, tools in
  the LLM context).

### Hooks — `hooks.json`

```json
{
  "processes": {
    "audit": {
      "command": ["node", "hook.js"],
      "observe": ["after_tool"],
      "intercept": ["before_tool"]
    }
  },
  "builtins": {
    "review-gate": { "enabled": true, "priority": 10 }
  }
}
```

The document uses Clawy's `hooks` config shape. Process hooks run with `dir`
defaulting to the plugin directory; builtin hook names must match the
registered builtin (they are merged verbatim, and a collision is reported
instead of overwritten).

## Reload behavior

- Gateway startup and every config reload re-run plugin discovery.
- `/reload` (or editing `config.json` with hot reload enabled) therefore picks
  up newly added or removed plugin directories.
- Skills are re-read on each catalog build, so new skills appear without a
  restart as soon as the bundle is reloaded.

## In-process plugins (`pkg/pluginsdk`)

`pkg/pluginsdk` is a separate, compile-time SDK for plugins written in Go that
register tools and hooks through a typed `Host` interface. It is unrelated to
the directory loader described here; directory plugins are the supported way
to extend a shipped Clawy build.

## Troubleshooting

| Symptom | Check |
| --- | --- |
| Plugin not listed | `clawy plugin list --roots` — is the directory under a search root? |
| Skills missing | Does `<plugin>/skills/<name>/SKILL.md` exist? `clawy plugin info <name>` prints resolved skill roots. |
| Command does nothing | The message must start with `/`. Check `clawy plugin info <name>` for the command name and `plugins.disable` for a typo. |
| MCP server not connecting | `clawy mcp list --status`, and confirm global MCP is enabled (`tools.mcp.enabled`). |
| Warnings on startup | They are repeated by `clawy plugin list`. |

See `examples/plugins/hello-clawy` for a runnable plugin.
