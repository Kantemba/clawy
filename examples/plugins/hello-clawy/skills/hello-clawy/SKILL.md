---
name: hello-clawy
description: Example skill shipped by the hello-clawy plugin. Use it when the user asks for a plugin demo.
---

# hello-clawy

This skill lives inside a plugin directory, not the workspace. Clawy discovers
it through `clawy-plugin.json` and lists it in the skill catalog with
`<source>plugin</source>`.

## Steps

1. Read the user's request.
2. Confirm the plugin skill loaded by checking the skill catalog entry.
3. Answer using the procedure below:
   - Greet the user.
   - Mention which plugin provided this skill.
   - Offer to run `/hello-clawy` for the prompt-command demo.
