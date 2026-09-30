# hello-clawy

Minimal Clawy plugin: one skill plus one prompt slash command.

## Layout

```
hello-clawy/
├── clawy-plugin.json            manifest (Clawy native dialect)
├── skills/hello-clawy/SKILL.md  skill shown in the agent catalog
├── commands/hello-clawy.md      /hello-clawy prompt command
└── README.md
```

## Install

```bash
clawy plugin install ./examples/plugins/hello-clawy
clawy plugin list
```

Then restart the gateway (or send `/reload` in a chat):

- `hello-clawy` appears in the skill catalog with `<source>plugin</source>`
- `/hello-clawy deploy api` rewrites the message into the prompt defined in
  `commands/hello-clawy.md`

## Try the other surfaces

Add these files next to `clawy-plugin.json`:

```jsonc
// .mcp.json — MCP servers, connected at startup
{
  "mcpServers": {
    "echo": { "command": "node", "args": ["server.js"] }
  }
}
```

```jsonc
// hooks.json — mounted by the hook runtime
{
  "processes": {
    "audit": {
      "command": ["node", "hook.js"],
      "observe": ["after_tool"],
      "intercept": ["before_tool"]
    }
  }
}
```

See `docs/plugins.md` for the full manifest reference.
