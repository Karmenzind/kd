# Testing, CI and local verification

Background for working on kd's tests and CI. The rules themselves live in
`AGENTS.md` (Testing and runtime side effects); this file records how the
pieces fit together and the traps found while writing the current tests.

## CI jobs (`.github/workflows/ci.yml`)

- `ubuntu`: `go vet`, then the whole suite once with the race detector and
  global coverage (`CGO_ENABLED=1 go test -race -count=1 -coverpkg=./...
  -coverprofile=coverage.out ./...`). The race detector needs cgo on Linux;
  release builds stay `CGO_ENABLED=0`. The job is read-only and passes the
  coverage figure to `badge` through job outputs.
- `badge`: runs only on pushes to `Karmenzind/kd` main. It writes a shields.io
  endpoint JSON (colors at 80/60/40%) and force-pushes it to the orphan
  `badges` branch; the README badge reads it from raw.githubusercontent.com.
  Write permission and `concurrency` can only be set per job, which is why it is
  separate. It is `continue-on-error`, so a failed publish never turns CI red.
  Codecov is deliberately not used: the figure is informational, and this needs
  no account, token or third-party upload action.
- `test` (macOS, Windows): `go vet`, `go test -count=1 ./...` with
  `CGO_ENABLED=0`, and a real `kd --daemon` / `--status` / `--stop` smoke test
  in a temporary HOME. A pre-created `<cache>/run/last_fetch_db` marker makes
  the data-download job skip, so the smoke test makes no network requests.
- `cross-build`: `bash scripts/build.sh --all`.
- `archlinux`: an `archlinux:base-devel` container. As the non-root user
  `builder` (kd refuses root) it runs the unit tests with `CGO_ENABLED=0`, the
  same daemon smoke test, `--generate-config </dev/null` (EOF answers yes), and
  the bash/fish Tab-completion e2e cases filtered by name (these build kd and
  run `kd --stop` only; no lookups). It then clones the `kd-bin` AUR package,
  builds it with this commit's binary, checks it with namcap and installs it.
  - namcap exits 0 even on errors, so the job fails on ` E: ` or `^Error: ` in
    its output; warnings pass.
  - The AUR clone is `continue-on-error`; if the AUR is unreachable the
    packaging steps are skipped.
  - kd-bin ships its own copies of `LICENSE` and `kd-server.service` with fixed
    sha256 sums; a non-blocking step warns when the repository copies differ.
  - Temporary exception: the published kd-bin 0.0.14 PKGBUILD installs LICENSE
    under `/usr/share/licenses/kd/` instead of `kd-bin/`, and will not be fixed.
    Until the 0.1.0 PKGBUILD (which must use
    `"$pkgdir/usr/share/licenses/$pkgname/LICENSE"`) is on the AUR, that one
    namcap error and the installed-license check only warn. Remove both
    exceptions in `ci.yml` once it is.

Unit tests never reach the network; CI only downloads Go, pacman packages and
the AUR repository. `e2e/` needs the real Youdao service and is run by hand.

## Coverage

- Method: the `ubuntu` command above, then drop `/cmd/ttstest/` lines and read
  `total:` from `go tool cover -func`. `-coverpkg=./...` counts code reached
  from any package's tests (for example `cmd/kd`'s child-process tests run most
  of `internal/query.go`); the price is that code executed without being
  checked also counts.
- Do not take numbers with Go 1.27.1, which counts about 10% more statements.
  Homebrew may install it; use `GOTOOLCHAIN=go1.27.4` (or later) locally.
- 2026-11-25: 85.6% on CI, up from 60.2% before the dead-code removal and the
  new tests.

## Traps when writing tests

- Paths are computed at package init: `internal/run` computes cache paths and
  `internal/cache` copies them into its own variables, and
  `update.LATEST_TAG_FILE` is fixed at init too. Replace every variable a test
  uses, in each package.
- Tests of the full CLI reuse the child-process harness
  `TestQueryCommandProcess` (`cmd/kd/query_command_test.go`): it points
  HOME/USERPROFILE at a temporary directory before package init, sets
  `KD_ALLOW_ROOT=1` and the daemon port, and `queryCommandDaemon` is a protocol
  stub.
- `EnsureReady` checks the daemon first. With nothing answering on the port it
  starts the test binary itself as the daemon and waits 3 seconds, so every case
  needs a stub, even invalid-input and cache-hit cases. To test an unreachable
  daemon, use a listener that closes each connection (`ErrPortOccupied`).
- On Linux, stopping and restarting the daemon (`--stop`, `--restart`,
  `--update`, the restart after editing config) asks
  `systemctl is-active --user kd-server` first; put a fake `systemctl` that says
  inactive first on PATH. `moveFile` in `internal/update` falls back to real
  `sudo mv`: put a fake `sudo` first on PATH and cause the failure with a
  missing parent directory, not permissions (root ignores them).
- `config.InitConfig` recomputes `CONFIG_PATH`, so set HOME and USERPROFILE with
  `t.Setenv`, and clear `http_proxy`, which makes it replace
  `http.DefaultTransport` globally. Config files are `~/.config/kd.toml` on
  Linux and macOS and `~/kd.toml` on Windows.
- `--edit-config` always opens `notepad` on Windows (skip there); elsewhere it
  reads `VISUAL`, then `EDITOR`, so point both at a fake editor.
- The TTS fake player must be named `afplay` on macOS and `ffplay` on Linux and
  Windows; with neither, Windows falls back to `explorer.exe`.
- logger tests must set `Path` explicitly. zap cannot close its file, so
  `t.TempDir` cleanup fails on Windows; skip file output there.
- `AskYN` does not reset its input: an invalid answer followed by an empty line
  or EOF loops forever, so follow invalid input with an explicit `Y` or `n`.
- `EchoFatal` calls `os.Exit(1)` and can only be tested in a child process.
- A test HTTP server must read the request before writing the response; use
  `httptest` rather than a hand-written TCP server.

### Windows fake programs

Fakes on Windows are `.bat` files, and only Windows CI exercises them.

- In a Go format string `%%` becomes `%`. A batch file needs `%%A` inside `for`
  but `%*` and `%VAR%` elsewhere, so write `%%%%A`, `%%*` and `%%VAR%%` in the
  format string. Doubling the latter two prints the literal text.
- `echo` ends with CRLF; code that trims only `\n` then sees `active\r`. Print
  without a newline with `<nul set /p "=active"`.
- `more` ends lines with CRLF and adds a blank line.
- Windows has no execute bits; `os.Chmod` only toggles read-only.
- A connection a probe leaves unused may end with `WSAECONNRESET` instead of
  `io.EOF` when the client process exits.

## Local Linux verification with colima

colima runs an aarch64 Linux VM. It differs from CI (CPU architecture, Debian
image, a clean user environment) but catches most Linux problems before a push.

- Use the latest `golang:1.27` patch image, mount the repository read-only
  (`-v "$PWD":/src:ro`) and set `CGO_ENABLED=0 GOFLAGS=-buildvcs=false`. A local
  `vendor/` switches the container to vendor mode.
- colima mounts only `$HOME`; `/private/tmp` is not visible in the container.
- Containers run as root, which kd refuses: set `KD_ALLOW_ROOT=1`, or
  `useradd` a user and set `GOCACHE`/`GOPATH` under `/tmp`. File-permission
  tests need the non-root user.
- e2e: `apt-get install bash-completion zsh fish`, then
  `KD_E2E_SHELLS=bash,zsh,fish go test -tags e2e -count=1 ./e2e`.
- Cross-build: run `git config --global --add safe.directory <repo>` first.
- Arch: the amd64 `archlinux:base-devel` image crashes Go under qemu. Use
  `menci/archlinuxarm:base-devel` natively with `-e OPENSSL_armcap=0` (pacman
  dies with "Illegal instruction" otherwise) and add `DisableSandbox` under
  `[options]` in `/etc/pacman.conf` (Landlock is unavailable).
