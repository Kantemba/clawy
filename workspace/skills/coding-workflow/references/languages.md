# Language Toolchain Reference

Canonical commands for detecting a project's stack and running its init/build/run/test/format/lint cycle. Always prefer scripts the project already defines (`Makefile`, `package.json` scripts, CI config) over these defaults.

## Detecting the Stack

Check these marker files first, then apply the matching column below:

| Marker file | Ecosystem |
| ----------- | --------- |
| `go.mod` | Go |
| `pyproject.toml`, `requirements.txt`, `setup.py` | Python |
| `package.json` | JavaScript/TypeScript |
| `Cargo.toml` | Rust |
| `pom.xml`, `build.gradle(.kts)` | Java/JVM |
| `*.csproj`, `*.sln` | C#/.NET |
| `CMakeLists.txt`, `Makefile` (C/C++ sources) | C/C++ |
| `Gemfile` | Ruby |
| `composer.json` | PHP |
| `pubspec.yaml` | Dart/Flutter |

## Go

```bash
go mod init github.com/user/project   # init (greenfield)
go build ./...                        # build
go run ./cmd/app                      # run
go test ./...                         # test
go vet ./...                          # lint (static checks)
gofmt -w .                            # format
```

Conventions: standard project layout (`cmd/` for binaries, `internal/` for private packages); table-driven tests in `_test.go` beside the code; errors as values — wrap with `%w`.

## Python

```bash
python -m venv .venv && source .venv/bin/activate   # env (Windows: .venv\Scripts\activate)
pip install -e ".[dev]"                             # install project + dev deps
uvicorn app.main:app --reload                       # run (web apps)
pytest                                              # test
ruff check .                                        # lint
ruff format .                                       # format  (or: black .)
mypy .                                              # type-check (if configured)
```

Declare metadata and dependencies in `pyproject.toml`. Tests live in `tests/`; use pytest fixtures over shared setup.

## JavaScript / TypeScript

```bash
npm install                # or: pnpm install / yarn / bun install
npm run dev                # run in dev mode (script name varies)
npm test                   # test (vitest/jest)
npm run lint               # lint (eslint)
npm run format             # format (prettier)
npm run build              # production build
npx tsc --noEmit           # type-check (TypeScript projects without separate script)
```

Always check which scripts `package.json` defines and reuse them. Respect lockfiles: use the package manager whose lockfile exists (`package-lock.json` npm, `pnpm-lock.yaml` pnpm, `yarn.lock` yarn, `bun.lockb` bun).

## Rust

```bash
cargo new project          # init binary (--lib for libraries)
cargo build                # build
cargo run                  # run
cargo test                 # test
cargo clippy               # lint
cargo fmt                  # format
```

Tests live in-module (`#[cfg(test)]`) and in `tests/`. Check both `cargo clippy` and `cargo fmt --check` before delivering.

## Java / JVM (Maven and Gradle)

```bash
mvn archetype:generate ...          # init (Maven) — usually created via IDE/template instead
mvn compile && mvn package          # build
mvn exec:java -Dexec.mainClass=...  # run (or run the packaged jar)
mvn test                            # test
./gradlew build                     # build + test (Gradle)
./gradlew test                      # test (Gradle)
```

Prefer the wrapper (`mvnw`/`gradlew`) when present so versions stay pinned.

## C# / .NET

```bash
dotnet new console -o MyApp    # init (also: webapi, mstest, xunit)
dotnet build                   # build
dotnet run --project MyApp     # run
dotnet test                    # test
dotnet format                  # format
```

Solution layout: one solution (`sln`) referencing projects per concern (`src/`, `tests/`).

## C / C++ (CMake)

```bash
cmake -B build                 # configure
cmake --build build            # build
./build/app                    # run
ctest --test-dir build         # test (with CTest configured)
clang-format -i src/*.cpp      # format (if .clang-format present)
```

Match the formatting config the project ships (`.clang-format`, `.editorconfig`); never reformat unrelated code.

## Ruby

```bash
bundle install             # deps
bundle exec ruby app.rb    # run
bundle exec rspec          # test (or: bundle exec rake test)
rubocop                    # lint
rubocop -A                 # autofix
```

## PHP

```bash
composer install           # deps
php artisan serve          # run (Laravel) / php -S localhost:8000 (plain)
./vendor/bin/phpunit       # test
./vendor/bin/php-cs-fixer fix  # format
```

## Cross-Language Notes

- **Windows environments:** activate venvs via `.venv\Scripts\activate`; use `.\gradlew.bat`, `mvnw.cmd`; path separators in configs may need escaping.
- **Shell scripts:** target POSIX sh for portability; set `set -euo pipefail`; quote variables.
- **Unknown or niche stacks:** find the truth in the repo itself — README, CI pipelines (`.github/workflows/`, `.gitlab-ci.yml`), and task runners define the canonical build/test invocations. Mirror them exactly.
- **Monorepos:** scope commands to the affected subproject instead of running the full build when possible.
