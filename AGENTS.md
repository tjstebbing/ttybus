# Agents guide for ttybus

ttybus is a text message bus for TUIs and CLIs. TUIs never join a
pipeline; the `ttybus` CLI does. Spec: [docs/PROTOCOL.md](docs/PROTOCOL.md).
Execution order: [PLAN.md](PLAN.md).

## Stack

- Go 1.24, module `github.com/tjstebbing/ttybus`
- Zero third-party deps on the root module until
  `examples/bubbletea` (and then only in that example's go.mod)
- Stdlib `flag` + `os.Args` subcommands. No cobra.

Build / test:

```sh
make build
make test
go test -race ./...
```

## Invariants (do not violate)

- `SOCK_STREAM` + newline framing. No unix datagrams. No fsnotify discovery.
- Clients connect in; daemon never dials out.
- CLI never opens `/dev/tty`. Logs on stderr. Flush every stdout line.
- Socket under `$XDG_RUNTIME_DIR/ttybus/` (0700), not `UserCacheDir`.
- Parser and hub have tests before the CLI grows features.
- Do not implement a slice past the current one in PLAN.md if earlier
  tests are red.

## Layout

- `cmd/ttybus` — one main package
- `internal/protocol` — parse/print
- `internal/daemon` — hub
- `internal/socket` — path, flock, auto-start
- `client` — public Go helper (after slice 5)
- `examples/` — not imported by the library
- `testdata/protocol` — golden lines

## Voice

README and PROTOCOL are user-facing and short. PLAN is the engineering
doc. Comments in code explain non-obvious constraints (flush, flock,
queue drop), not the history of minibus.
