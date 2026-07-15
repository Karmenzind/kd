# AGENTS.md

## Working agreements

- Run all scripts and commands through PowerShell with `pwsh -NoProfile`.
- Prefer `pnpm` if Node.js tooling is introduced. Ask before adding any new production dependency.
- Keep reusable device and OS configuration scripts in `~/Workspace/dotfiles-and-scripts`. Keep generated reports, logs, state, caches, and other runtime output outside that repository.
- Preserve the existing concise bilingual style. Most CLI messages and the README are Chinese with English where useful.

## Project overview

`kd` is a small, cross-platform command-line dictionary distributed as a single binary. It looks up words and long text, uses a local SQLite cache, and delegates online queries to a localhost daemon. It also contains optional TTS, self-update, paging, logging, and shell-completion features.

- Module: `github.com/Karmenzind/kd`
- Go version: 1.26.5 (see `go.mod`)
- CLI entry point: `cmd/kd/kd.go`
- Auxiliary TTS test program: `cmd/ttstest/main.go`
- The SQLite driver is the pure-Go `modernc.org/sqlite`; builds and cross-builds must keep `CGO_ENABLED=0` and require no C compiler.

## Design philosophy

- Prefer simplicity over flexibility, readable direct code over abstraction, and long-term maintenance cost over theoretical extensibility.
- Use the standard library first. Add a dependency only for a clear technical reason that outweighs its ongoing update, security, binary-size, and compatibility costs.
- Do not turn `kd` into a general platform. Avoid frameworks, plugin systems, dependency-injection machinery, and layers that do not solve a current concrete problem.
- Add configuration, persistent state, top-level commands, and background behavior sparingly because each becomes a compatibility and maintenance obligation. Do not add another background service alongside the existing narrowly scoped daemon.
- When several implementations are reasonable, choose the one with fewer concepts, less state, and lower long-term maintenance cost. Do not add a dependency merely to save a small amount of straightforward code.
- For this interactive CLI, prioritize startup latency, response time, and memory use. Measure before optimizing; do not trade away readability for unsupported micro-optimizations.

## Repository map

- `cmd/kd`: CLI flags, startup, configuration wiring, and result presentation.
- `config`: TOML configuration defaults, loading, validation, and sample output.
- `internal/client.go` and `internal/server.go`: client/daemon orchestration and the newline-delimited JSON protocol over localhost TCP.
- `internal/query`: cache/daemon query handling, Youdao response parsing, and output formatting.
- `internal/cache`: SQLite data, not-found data, and query counters.
- `internal/daemon`: daemon lifecycle, process discovery, and scheduled maintenance.
- `internal/model`: shared request, response, result, and runtime data structures.
- `internal/run`: runtime metadata, cache paths, and the fixed daemon port (`19707`).
- `internal/tts` and `internal/update`: optional speech and binary-update flows.
- `pkg`: focused filesystem, HTTP, terminal, process, string, and decoration helpers.
- `scripts/build.sh`: release build logic used by GitHub Actions for Linux amd64/arm64, macOS amd64/arm64, and Windows amd64.

## Development workflow

Use repository-root package paths such as `./cmd/kd` for local builds.

```powershell
pwsh -NoProfile -Command "go test ./..."
pwsh -NoProfile -Command "go vet ./..."
pwsh -NoProfile -Command "go build -o build/kd.exe ./cmd/kd"
pwsh -NoProfile -Command "gofmt -w path/to/changed.go"
```

Before handing off a Go change, run `gofmt` on changed files and at least `go test ./...`. Run `go vet ./...` for changes beyond documentation. The checked-in `Makefile` and release script assume Unix tools; prefer direct Go commands during cross-platform development on Windows.

Do not run `go mod tidy` merely as generic validation because it can rewrite module metadata. When dependency metadata intentionally changes, inspect both `go.mod` and `go.sum`. Prefer small, actively maintained libraries with stable APIs; avoid CGO, large transitive dependency trees, heavyweight frameworks, and dependencies that introduce background services.

## Runtime and test side effects

Package initialization creates user-level runtime directories. On Windows and Linux these are under `~/.cache/kdcache`; on macOS they are under `~/Library/Caches/kdcache`. Windows configuration is read from `~/kd.toml`; Linux and macOS use `~/.config/kd.toml`.

- Test coverage remains focused rather than exhaustive; add regression tests around behavior being changed.
- Running the CLI for a real query may create or restart a daemon, bind localhost port `19707`, write cache/database/log/runtime files, and make network requests.
- Update, TTS, configuration-generation/editing, daemon lifecycle, and release scripts have additional external side effects. Do not use them as routine smoke tests.
- Prefer focused unit tests with temporary directories and injected/local test servers for cache, filesystem, or HTTP behavior. Do not depend on the user's config, cache contents, network access, or a running daemon.

## Change guidance

### Cache and persisted data

- SQLite is a local optimization layer, not the source of truth. A missing or corrupt cache, or a recoverable database failure, should degrade to online lookup instead of preventing it.
- Keep the schema simple. Do not add tables, indexes, or persistent metadata without a measured or otherwise explicit benefit.
- Preserve existing database files and cached data where practical. Changes to the SQLite driver, schema, time representation, or serialized formats require compatibility tests using data in the previous format and an explicit migration or fallback when needed.
- Treat the config file, cache database, serialized cache records, and daemon protocol as compatibility-sensitive public state. Never silently invalidate them.

### Cross-platform behavior

- Every feature must continue to support Linux, macOS, and Windows by default. Concentrate platform-specific behavior in focused helpers or platform files rather than scattering `runtime.GOOS` branches through business logic.
- Changes involving paths, processes, terminals, character encoding, shell invocation, or executable replacement must consider and verify every affected platform.
- Keep the single-binary, `CGO_ENABLED=0` distribution model. Do not edit generated binaries (`kd.exe`, release artifacts) or vendored code during normal source changes.

### CLI, errors, and logging

- Human-readable output is part of the public interface. Do not casually change wording, layout, colors, ordering, or add emoji; keep Chinese and English output concise and stylistically consistent.
- Keep flag behavior and help text together in `cmd/kd/kd.go`. When public behavior changes, update CLI help and README usage in the same change.
- Return errors with useful context instead of panicking. User-facing errors should be concise, understandable, and actionable; degrade gracefully when the problem is recoverable.
- Log enough context to diagnose failures without flooding routine operation. Do not log configuration secrets, tokens, user queries, or other sensitive data unless the task explicitly requires it and the exposure is documented.

### Configuration, daemon, and releases

- Keep configuration changes synchronized across `Config` fields/default tags, validation, generated samples in `config/output.go`, and the README. Avoid new options when an existing behavior or command is sufficient.
- For daemon request, response, or serialization changes, review `internal/model`, `internal/client.go`, `internal/server.go`, and `internal/query` together. Preserve compatibility between old and new CLI/daemon versions where practical; otherwise explicitly handle stale processes, protocol versions, and cached payloads.
- Release builds are tag-driven through `.github/workflows/release-tags.yml`; `scripts/build.sh` cross-builds all artifacts from one host and is the source of truth for artifact names and linker/build tags.
- The single Ubuntu release job may cross-compile macOS artifacts only while the build remains pure Go with `CGO_ENABLED=0` and has no dependency on Apple SDKs, Objective-C, native dynamic libraries, or macOS system frameworks. Native macOS CI still provides compile-and-test coverage, but the Linux runner cannot execute the generated release artifacts.
- Restore a native macOS release job when adding CGO or platform-native code, Apple code signing or notarization, macOS-specific packaging, or when release artifacts require native smoke testing. Treat any such change as a release-process decision rather than silently extending the cross-build script.
