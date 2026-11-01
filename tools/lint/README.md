# tools/lint

Pins the `staticcheck` that CI runs and the `errcheck` and `nilerr` that `labdrian review sweep` runs. `go.mod` names the versions and `go.sum` holds the checksum of each
and of every module it is built from, so a CI job no longer resolves a version tag at run time.
The module has no code of its own: `doc.go` only gives the per-module checks of the repository a package to look at.

| Pinned | Where |
|---|---|
| staticcheck v0.7.0 (2026.1) | `require honnef.co/go/tools` in `go.mod` |
| errcheck v1.30.0 | `require github.com/kisielk/errcheck` in `go.mod` |
| nilerr v0.1.2 | `require github.com/gostaticanalysis/nilerr` in `go.mod` |
| the modules it is built from | `go.sum` |

## How CI uses it

The jobs that run staticcheck (`test-engine`, `test-tui`, `test-longterm-mem`, `test-archguard`, `test-identity`,
`test-telemetry`) build the binary from this module and run it in the module they lint:

```bash
go build -C "$GITHUB_WORKSPACE/tools/lint" -o "$RUNNER_TEMP/staticcheck" honnef.co/go/tools/cmd/staticcheck
"$RUNNER_TEMP/staticcheck" ./...        # the tui job passes -tags backend
```

`go build` checks every module it downloads against `go.sum` and fails on a mismatch or a missing entry.
To run it by hand, use the same two lines from the module you want to lint, with any directory for the binary.
It is built first and run from the module because `go run -C` would run it inside `tools/lint`, and
`go run -modfile` does not switch toolchains, which the `go 1.21` jobs need.

## Toolchain

The `go` directive is the Go that errcheck v1.30.0 requires (1.26.0), the highest of the three. An older errcheck cannot be pinned: v1.10.0 and v1.20.0, built against an older `golang.org/x/tools`, fail on a Go 1.27 toolchain (`package "errors" without types was imported`); v1.30.0 raises `golang.org/x/tools` to v0.51.0 for staticcheck too, which reports the same lines in every module CI lints (2026-11-01). A job on an older Go (the
`go 1.21` modules) downloads exactly that toolchain through `GOTOOLCHAIN=auto` to build the binary; a job on a newer
Go (tui and longterm-mem, 1.26.1) builds with its own. The code under analysis is still loaded by the job's own `go`.

## Bumping

```bash
cd tools/lint
go get -tool honnef.co/go/tools/cmd/staticcheck@vX.Y.Z   # also raises the go directive when the new version needs it
go get -tool github.com/kisielk/errcheck@vX.Y.Z          # likewise for errcheck
go get -tool github.com/gostaticanalysis/nilerr/cmd/nilerr@vX.Y.Z   # and nilerr
go mod tidy
go mod verify
```

Commit `go.mod` and `go.sum` together. The `// indirect` on `honnef.co/go/tools` is what `go get -tool` writes.
The registry of `tools/deterministic-check-runner` names the same version for the repositories it checks; keep the two equal.
`deadcode` is not covered here: CI runs it as a gate in the engine and tui jobs with `go run golang.org/x/tools/cmd/deadcode@v0.48.0 -test ./...` (the tui adds `-tags backend`), and any line it prints fails the job. Run the same command locally, not an installed `deadcode` binary: a binary built with an older Go fails on a newer toolchain ("packages contain errors"). v0.49.0 reports the same lines as v0.48.0 in every module (2026-10-23).
