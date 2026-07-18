# AGENTS.md

## Working agreements

- On Windows, prefer PowerShell with `pwsh -NoProfile` for scripts and commands. On Linux and macOS, use the available native shell.
- Prefer `pnpm` if Node.js tooling is introduced. Ask before adding any new production dependency.
- Keep reusable device and OS configuration scripts in `~/Workspace/dotfiles-and-scripts`. Keep generated reports, logs, state, caches, and other runtime output outside that repository.
- The project evolves continuously. When a change materially alters repository structure, workflows, compatibility assumptions, or other durable facts, update `AGENTS.md` as part of the same work so it remains accurate; keep additions concise and avoid documenting temporary implementation details.
- The project primarily serves Chinese-speaking users, so user-facing UI text should be in Chinese. Keep it concise and use English only where useful.
- Developer-facing content may use English, including internal output, logs, daemon output, configuration language, and code comments.

## Project overview

`kd` is a small, cross-platform command-line dictionary distributed as a single binary. It looks up words and long text, uses a local SQLite cache, and delegates online queries to a localhost daemon. It also contains optional TTS, self-update, paging, logging, and shell-completion features.

- Module: `github.com/Karmenzind/kd`
- Go version: 1.26.5 (see `go.mod`)
- CLI entry point: `cmd/kd/kd.go`
- CLI framework: `github.com/urfave/cli/v3`; construct a fresh root `cli.Command` for tests and keep framework types inside `cmd/kd`.
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

- `cmd/kd`: urfave/cli v3 root command, flags, startup, configuration wiring, and result presentation.
- `config`: TOML configuration defaults, loading, validation, and sample output.
- `internal/query.go` and `internal/server.go`: CLI-side query orchestration and the thin server entry point retained for compatibility.
- `internal/query`: cache and online query handling, Youdao response parsing, and output formatting.
- `internal/ui`: delayed, TTY-only query progress rendering and terminal capability fallbacks; it must not contain query business logic.
- `internal/cache`: SQLite data, not-found data, and query counters.
- `internal/daemon`: daemon lifecycle, TCP client/server communication, runtime state, process discovery, and scheduled maintenance.
- `internal/model`: shared request, response, result, and runtime data structures.
- `internal/run`: runtime metadata, cache paths, and the fixed daemon port (`19707`).
- `internal/tts` and `internal/update`: optional speech and binary-update flows.
- `pkg`: focused filesystem, HTTP, terminal, process, string, and decoration helpers.
- `scripts/build.sh`: release build logic used by GitHub Actions for Linux amd64/arm64, macOS amd64/arm64, and Windows amd64.

## Development workflow

Use repository-root package paths such as `./cmd/kd` for local builds.

```shell
go test -count=1 ./...
go vet ./...
gofmt -w path/to/changed.go
```

Build with `CGO_ENABLED=0`. On Unix use `CGO_ENABLED=0 go build -o build/kd ./cmd/kd`; in PowerShell set `$env:CGO_ENABLED = "0"` before running the equivalent `go build` command.

Before handing off a Go change, run `gofmt` on changed files and at least `go test ./...`. Run `go vet ./...` for changes beyond documentation. After a substantial change is pushed, inspect the GitHub Actions CI result across Linux, macOS, and Windows; investigate failures instead of assuming local success covers platform-specific behavior. The checked-in `Makefile` and release script assume Unix tools; prefer direct Go commands during cross-platform development on Windows.

Do not run `go mod tidy` merely as generic validation because it can rewrite module metadata. When dependency metadata intentionally changes, inspect both `go.mod` and `go.sum`. Prefer small, actively maintained libraries with stable APIs; avoid CGO, large transitive dependency trees, heavyweight frameworks, and dependencies that introduce background services.

## Testing and runtime side effects

Package initialization creates user-level runtime directories. On Windows and Linux these are under `~/.cache/kdcache`; on macOS they are under `~/Library/Caches/kdcache`. Windows configuration is read from `~/kd.toml`; Linux and macOS use `~/.config/kd.toml`.

- Test coverage is risk-driven rather than percentage-driven. Add regression tests for changed behavior and failure modes, using table-driven tests where they improve clarity.
- Prefer the standard `testing` package, `t.TempDir`, `t.Setenv`, `httptest`, `net.Pipe`, and listeners on `127.0.0.1:0`. Do not add a test or mock framework for behavior the standard library can express clearly.
- Tests must not depend on execution order, the user's config or cache, real network services, a running daemon, or fixed port `19707`. Use a real temporary SQLite database instead of mocking `database/sql`.
- Avoid `time.Sleep` in tests. Coordinate concurrency with channels, wait groups, contexts, or listener readiness, and run focused `go test -race` checks after changing concurrent cache or protocol code when the platform supports it.
- Running the CLI for a real query may create or restart a daemon, bind localhost port `19707`, write cache/database/log/runtime files, and make network requests.
- Update, TTS, configuration-generation/editing, daemon lifecycle, and release scripts have additional external side effects. Do not use them as routine smoke tests.
- Test TTS, update, editor, process, and system-service code at the parameter, parsing, or filesystem boundary; do not perform real playback, binary replacement, editor launch, process termination, or systemd changes.

## Change guidance

### Cache and persisted data

- SQLite is a local optimization layer, not the source of truth. A missing or corrupt cache, or a recoverable database failure, should degrade to online lookup instead of preventing it.
- Downloaded data ZIPs are also optional cache inputs. Download or decompression failures must leave the current database untouched and must not stop the daemon or block online queries.
- TTS audio uses opaque hashed filenames and atomic downloads. Its opportunistic cleanup runs at most once per 24 hours when TTS is used, respects `audio_cache_max_size_mb` (default 2048 MiB), and treats zero as no persistent audio cache; cleanup failures must not block playback.
- Keep long-text JSON cache updates serialized within the process and replace the cache file atomically. Do not reintroduce asynchronous truncate-and-rewrite behavior that lets readers observe partial files.
- Keep the schema simple. Do not add tables, indexes, or persistent metadata without a measured or otherwise explicit benefit.
- Preserve existing database files and cached data where practical. Changes to the SQLite driver, schema, time representation, or serialized formats require compatibility tests using data in the previous format and an explicit migration or fallback when needed.
- Treat the config file, cache database, serialized cache records, and daemon protocol as compatibility-sensitive public state. Never silently invalidate them.

### Cross-platform behavior

- Every feature must continue to support Linux, macOS, and Windows by default. Concentrate platform-specific behavior in focused helpers or platform files rather than scattering `runtime.GOOS` branches through business logic.
- Changes involving paths, processes, terminals, character encoding, shell invocation, or executable replacement must consider and verify every affected platform.
- Keep the single-binary, `CGO_ENABLED=0` distribution model. Do not edit generated binaries (`kd.exe`, release artifacts) or vendored code during normal source changes.

### CLI, errors, and logging

- Human-readable output is part of the public interface. Do not casually change wording, layout, colors, ordering, or add emoji; keep Chinese and English output concise and stylistically consistent.
- Keep dynamic query status on stderr and stop it before any final output. It must remain disabled for non-TTY, JSON, and other machine-readable paths; keep ANSI and animation details inside `internal/ui`.
- Keep flag behavior and help text together in `cmd/kd/kd.go`. When public behavior changes, update CLI help and README usage in the same change.
- Return errors with useful context instead of panicking. User-facing errors should be concise, understandable, and actionable; degrade gracefully when the problem is recoverable.
- Log enough context to diagnose failures without flooding routine operation. Do not log configuration secrets, tokens, user queries, or other sensitive data unless the task explicitly requires it and the exposure is documented.
- Preserve the structured `component` field so daemon/server logs use `component=server` and CLI logs use `component=client`.

### Configuration, daemon, and releases

- Keep configuration changes synchronized across `Config` fields/default tags, validation, generated samples in `config/output.go`, and the README. Avoid new options when an existing behavior or command is sufficient.
- For daemon request, response, or serialization changes, review `internal/model`, `internal/daemon/client.go`, `internal/daemon/server.go`, and `internal/query.go` together. Preserve compatibility between old and new CLI/daemon versions where practical; otherwise explicitly handle stale processes, protocol versions, and cached payloads.
- Release builds are tag-driven through `.github/workflows/release-tags.yml`; `scripts/build.sh` cross-builds all artifacts from one host and is the source of truth for artifact names and linker/build tags.
- The single Ubuntu release job may cross-compile macOS artifacts only while the build remains pure Go with `CGO_ENABLED=0` and has no dependency on Apple SDKs, Objective-C, native dynamic libraries, or macOS system frameworks. Native macOS CI still provides compile-and-test coverage, but the Linux runner cannot execute the generated release artifacts.
- Restore a native macOS release job when adding CGO or platform-native code, Apple code signing or notarization, macOS-specific packaging, or when release artifacts require native smoke testing. Treat any such change as a release-process decision rather than silently extending the cross-build script.
