# Memory and the general self-improvement skill

Clawy creates `skills/self-improvement/SKILL.md` inside each configured agent
workspace. Its complete recall → act → verify → learn → correct workflow is
preloaded on every normal turn, including turns with optional task skills off or
filtered. No skill selection or extra `read_file` call is needed.

The workflow is protected; the agent improves its bounded **Learned rules**
section through the `memory` tool. Lessons become visible in the next prompt,
invalidate the local prompt cache, and survive restarts. Preloading supplies the
workflow to the model; it is not a guarantee that every model will follow it or
that an inferred lesson is correct.

## Three memory targets

| Target | Contents | Workspace path | Character budget |
| --- | --- | --- | --- |
| `user` | Explicit stable user facts and preferences | `USER.md` | 1,375 |
| `memory` | Durable project facts, decisions, environment constraints | `memory/MEMORY.md` | 2,200 |
| `learning` | Reusable trigger/action/verification rules | `skills/self-improvement/SKILL.md` | 4,000 for lessons |

Example tool arguments:

```json
{
  "action": "add",
  "target": "learning",
  "text": "When fixing a reported regression, add a reproducing test and run it before claiming a fix."
}
```

`read`, `replace` (`old_text` / `new_text`), and `remove` (`old_text`) work on all
three targets. Only the learned rules are exposed by `target=learning`; deleting
a rule cannot delete the protected workflow. Ambiguous replacements, duplicate
entries, recognizable secrets, reserved markers, and over-budget changes are
rejected. Consolidation is explicit, never a silent deletion of valid memory.

## Background evolution

The existing `evolution.identity_curation` key remains compatible, but now
extracts general working rules into `learning` and explicit user facts into
`user`, using the **same** memory backend, validation, budgets, and in-process
workspace lock as the agent tool. It no longer modifies `SOUL.md`. The curator is
rate-limited by `memory/.self-improvement-curated` and reports only actual writes.

Task-specific skill creation/revision remains available under the existing
evolution modes. It cannot replace the reserved general skill. Disabling
`evolution.enabled` stops background extraction, not the preloaded workflow or
explicit agent memory operations. Disabling the memory tool prevents the agent
from saving lessons; disabling background curation prevents its separate writes.

## Compatibility and limits

- Existing `MEMORY.md`, `USER.md`, and `SOUL.md` are not erased or automatically
  reclassified. Legacy unmarked memory is retained as a single editable entry.
- Explicit turn profiles with `system_prompt.mode=off` remain an opt-out from
  this workflow, just as they opt out of other default Clawy system context.
- Tool allowlists, permission checks, and approvals still apply. The skill does
  not enable disabled tools, execute autonomous tasks, or permit changing safety
  rules.
- A missing/unreadable/malformed skill never removes the baseline workflow from
  the prompt. An existing file without the lessons delimiters is preserved and
  learning writes fail rather than overwrite it. Repair the delimiter pair
  `<!-- self-improvement:lessons:begin -->` / `<!-- self-improvement:lessons:end -->`
  around its learned entries before updating that target.
- Workspace locks serialize Clawy instances within one process. Independent
  processes should not write the same workspace concurrently.
