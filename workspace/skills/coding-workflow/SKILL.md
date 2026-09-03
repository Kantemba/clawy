---
name: coding-workflow
description: "Structured end-to-end workflow for writing code and building software projects in any programming language. Use when creating a new project, app, service, library, or script from scratch; implementing features or fixing bugs in an existing codebase; scaffolding a repository with build/test tooling; refactoring; or whenever the user asks to build, create, implement, write, or code something."
---

# Coding Workflow

Work through six phases in order: **Understand → Plan → Scaffold → Implement → Verify → Deliver**. Never claim a coding task is complete before Phase 5 has actually run. Code written before requirements are understood is throwaway; code that has not been executed is not done.

This workflow is language-agnostic. Detect the language and ecosystem first, then follow that ecosystem's conventions instead of imposing defaults. Detailed guidance per phase lives in [references/workflow.md](references/workflow.md). Ready-to-adapt init/build/run/test/lint commands per language live in [references/languages.md](references/languages.md).

## Phases at a Glance

| # | Phase | Goal | Exit Criteria |
| - | ----- | ---- | ------------- |
| 1 | Understand | Know exactly what to build and for whom | Goal restated; acceptance criteria defined; blocking ambiguities resolved |
| 2 | Plan | Decide how to build it | Language/stack chosen; file layout sketched; work broken into ordered slices |
| 3 | Scaffold | Create a runnable skeleton | Project initializes; build tool works; empty skeleton runs or compiles |
| 4 | Implement | Write the code slice by slice | Every planned slice implemented; no placeholders or stubs left |
| 5 | Verify | Prove it works | Build passes; tests pass; lint/format clean; smoke test done |
| 6 | Deliver | Hand it over cleanly | Self-review done; docs updated; concise summary given |

## Sizing: Full Workflow vs Fast Path

Match process weight to task size:

- **Fast path** (trivial single-file change, tiny function, config tweak): compress Phases 1–3 into a quick restatement of the goal plus locating the affected files. Phases 5–6 are never compressed: still run the code/tests before reporting success.
- **Standard** (feature or bug fix inside an existing repository): run all phases, keeping the plan lightweight and internal unless the user asked to review it.
- **Greenfield** (new project, app, or service): run the full workflow and present the Phase 2 plan to the user for confirmation before scaffolding. Confirm language/framework choice, scope, and any assumptions that are expensive to reverse.

When the request is ambiguous in a way that changes what gets built (wrong guesses are expensive), ask targeted clarifying questions in Phase 1 rather than assuming. When ambiguity is minor, state the assumption explicitly and proceed.

## Universal Rules

These apply regardless of language or framework:

1. **Read before writing.** Explore existing code, configs, and docs first. Match the codebase's established style, naming, and patterns over personal preference.
2. **Prefer project-native commands.** If the repo ships a `Makefile`, `package.json` scripts, `justfile`, or CI config, use those entry points instead of inventing equivalent commands.
3. **Smallest correct change.** Avoid drive-by refactors unrelated to the goal. Note unrelated issues instead of fixing them silently.
4. **Run after every meaningful change.** Compile, run, or test after each slice — never batch verification until the end.
5. **No placeholders.** No `TODO: implement`, no stubbed bodies, no fake return values in delivered code. If something is genuinely out of scope, say so explicitly in the delivery summary.
6. **Handle failure paths.** Validate inputs, surface errors instead of swallowing them, and clean up resources.
7. **Keep secrets out of code.** Read configuration from environment variables or config files excluded by `.gitignore`.
8. **Version control.** In an existing repo, do not commit unless asked; leave the working tree ready for review. In greenfield projects, initialize git early and commit at stable milestones when the user expects a ready-to-push result.

## References

Read these only when their detail is needed:

- [references/workflow.md](references/workflow.md) — step-by-step actions, checklists, and templates for each of the six phases.
- [references/languages.md](references/languages.md) — canonical init/build/run/test/format/lint commands for common language ecosystems, plus how to detect a project's stack.
