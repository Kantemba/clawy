# Workflow Reference

Step-by-step guidance for each phase of the coding workflow. Apply every phase, but scale depth to task size as described in `SKILL.md`.

## Phase 1 — Understand

Goal: know exactly what to build before touching code.

1. Restate the request in one sentence: what is being built, for whom, and what does success look like?
2. Gather context:
   - Existing repo: identify language(s), framework(s), package manager, build system, test runner, and directory layout. Read neighboring code that resembles the feature being added.
   - Greenfield: check whether the user pinned a language, framework, or deployment target. Note platform constraints (OS, runtime versions).
3. Define acceptance criteria as concrete checks: "X happens when Y", "command Z exits 0", "output matches W". These become the Phase 5 checklist.
4. Resolve blocking ambiguities with short, specific questions (offer a default so the user can just confirm). State non-blocking assumptions explicitly and continue.

Exit when you can list the inputs, outputs, and edge cases without guessing.

## Phase 2 — Plan

Goal: choose the approach and break work into verifiable slices.

1. Choose language, framework, and key dependencies. Prefer whatever the existing project already uses; for greenfield, pick boring, well-supported options and justify non-obvious choices in one line each.
2. Sketch the structure before creating anything:
   - Modules/packages/files to create or modify, with one line on responsibility each.
   - Data flow for the core scenario (entry point → processing → output).
   - Interfaces or contracts between components.
3. Break implementation into ordered vertical slices. Order by risk first (hardest/most uncertain part early), then by dependency. Each slice must be independently compilable/runnable and checkable.
4. For greenfield or multi-file features, present the plan before implementing: chosen stack, proposed layout, milestone order, risks/open questions. Wait for confirmation only when the task is large or the choices were unconstrained; otherwise proceed and summarize.

Anti-patterns: planning in code comments instead of deciding; slicing horizontally (all models, then all views) so nothing runs until the very end.

## Phase 3 — Scaffold

Goal: a minimal runnable skeleton exists and the toolchain works.

1. Initialize the project with the ecosystem's standard tool (`go mod init`, `npm init`, `cargo new`, `dotnet new`, `python -m venv` + `pyproject.toml`, …). See `languages.md` for the exact commands.
2. Add version control early for greenfield projects: `git init`, a language-appropriate `.gitignore`, and an initial commit once the skeleton runs.
3. Configure tooling before writing real code: formatter, linter, test runner, and build command. Fixing tooling later costs more than setting it up first.
4. Create the smallest end-to-end skeleton: entry point, one placeholder module wired through the real data flow, one trivially passing test. Run it. The point is to prove the pipeline (build → run → test) works while almost nothing exists yet.

Do not scaffold directories or resource files that are not used; delete anything unused before delivering.

## Phase 4 — Implement

Goal: fill in the skeleton slice by slice, verifying continuously.

Per slice:

1. Implement the smallest complete unit of behavior — enough to compile and demonstrate the slice's outcome.
2. Follow existing conventions: naming, error handling, logging, file organization. When conventions conflict, match the dominant local pattern and mention the conflict in the summary.
3. Handle errors where they occur: validate inputs, wrap with context, never discard errors silently.
4. Run/build/test immediately after the slice. Fix red before moving on.
5. Commit at stable milestones if git is initialized and the user expects a pushable history; otherwise leave commits to the user.

Writing rules:

- No dead code, no commented-out blocks, no debug prints left behind.
- No hardcoded secrets, hosts, or credentials; use environment/config.
- Public functions/types get a doc comment saying what they do, not restating the name.
- Dependencies: add sparingly, pin versions per ecosystem convention, verify each new dependency is actually imported.

If a slice turns out to be wrong at plan level, stop and re-plan that part rather than patching around a bad design.

## Phase 5 — Verify

Goal: prove correctness with execution, not assertion.

Run, in this order, and fix failures before proceeding:

1. **Build/compile** — clean build with no new warnings.
2. **Tests** — full suite, not just new tests. New behavior needs tests covering happy path, edge cases (empty input, boundaries, invalid input, concurrency if relevant), and at least one regression case for each bug fixed.
3. **Lint/format** — run the project's formatter and linter; resolve findings.
4. **Smoke test** — execute the primary scenario end-to-end as a user would (run the CLI, hit the endpoint, open the page) and compare against the Phase 1 acceptance criteria.
5. **Acceptance criteria review** — walk the Phase 1 list item by item; every item must have been demonstrated, not assumed.

Never report success based on code reading alone. If a required tool is unavailable in the environment, say exactly what was not verified and how the user can verify it.

## Phase 6 — Review & Deliver

Goal: hand over clean, understandable work.

Self-review checklist (read the final diff before delivering):

- [ ] Only changes serving the stated goal are included.
- [ ] No leftover TODOs, stubs, debug output, or unused files/imports.
- [ ] Errors are handled and messages are actionable.
- [ ] Names say what things mean; no misleading identifiers.
- [ ] Tests cover the new behavior and pass.
- [ ] Secrets and local paths are absent.

Documentation:

- Greenfield: provide a `README.md` with purpose, prerequisites, install, run, test, and basic usage examples.
- Existing repo: update the docs the project already keeps (README section, CHANGELOG entry, docstrings) — do not invent parallel documentation.
- New commands or flags introduced anywhere must appear in the docs.

Delivery summary format (keep it short):

```
Done: <one-line outcome>

What changed:
- <file/module>: <what and why>

Verified:
- <commands run and their results>

Notes:
- <assumptions made, known limitations, suggested follow-ups>
```                 

