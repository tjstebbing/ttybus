# ttybus implementation plan

This is the execution document. Implement in the order of
[Implementation slices](#implementation-slices). Do not skip to the
client library or plumber until `pub`/`sub` is pipeline-correct.

Companion docs:

- [README.md](README.md) — what it is, for humans
- [docs/PROTOCOL.md](docs/PROTOCOL.md) — the wire, normative
- [AGENTS.md](AGENTS.md) — conventions for agents working this repo

## Problem

Unix composition is `cmd1 | cmd2`. That fails for TUIs:

- stdin is the keyboard
- stdout is the screen (raw mode, alternate buffer)
- you cannot `htop | grep` or `yazi | nvim` in any useful way

So every TUI that needs to talk to another process grows a private side
channel (Yazi DDS, `lf -remote`, Kakoune sockets/FIFOs). There is no
shared, grep-friendly bus.

ttybus is that bus: a user daemon on a unix stream socket, a newline
text protocol, and a CLI that *is* a Unix filter. TUIs never join a
pipeline. The CLI does.

## Lessons from ttyzero/minibus (2018)

The old org is cloned at `~/code/ttyzero`. Read it before changing
discovery or framing. The product idea was right. The wire was not.

| What minibus did | Why it hurt | ttybus does instead |
|---|---|---|
| `SOCK_DGRAM` + `bufio.Scanner` | Scanner is a stream parser; datagrams already have boundaries; macOS unixgram buffer/bind semantics differ | `SOCK_STREAM` + newline framing |
| Listeners drop `$PID-$CHANNEL` files; daemon fsnotify-dials out | CREATE-before-bind races; no scan of existing sockets; empty Remove handler; no liveness until Write fails | Clients **connect in**; close = gone |
| `chan:msg` regex, two different channel alphabets | Silent drops; filename vs payload mismatch | One verb/token grammar, one channel charset |
| World-writable `UserCacheDir()/minibus` (`0766`) | Wrong lifetime, wrong perms | `$XDG_RUNTIME_DIR/ttybus/` `0700` |
| `minibus-go` Send writes `"OPEN"` and no newline; closer started after Scan | Messages never parse; channels cannot be closed | Spec first, tests on the parser, CLI mapped 1:1 to verbs |
| `tzpipe` README described `listen \| send`; cobra stubs printed `"send called"` | The actual product was never written | Slice 3 is *only* the filter, specified down to flush and SIGPIPE |
| Three repos (daemon, lib, CLI) | Protocol drifted | One module, one binary |

"Pipe logic" meant both internal routing *and* `tzpipe listen | grep |
tzpipe send`. Both are specified here before any code.

## Goals

- Any process that can `connect(2)` and write a line is a client.
- `ttybus sub foo | grep bar | ttybus pub baz` is the demo, and it works
  when stdout is a pipe.
- A TUI author integrates in ~100 lines (connect, HELLO, SUB, scan MSG
  into the UI event loop).
- One user daemon, optional named buses for multiplexer isolation.
- `nc -U` is a supported client.

## Non-goals (v0)

- Cross-machine, TCP, SSH forwarding of the bus
- Persistence, retained messages, durable queues
- Authentication beyond socket mode
- JSON as the envelope (payloads may be JSON)
- FIFOs as transport
- D-Bus / Varlink compatibility
- Making two TUIs share stdin/stdout
- Windows (unix sockets exist there now; not v0)

## Key decisions

1. **`SOCK_STREAM` + newline framing.** Portable. `SOCK_SEQPACKET` is
   nicer on Linux and historically spotty on macOS. Datagrams were the
   2018 failure mode. If multiline payloads are needed later, add an
   explicit length-prefixed frame; do not overload v0 lines.
2. **Clients connect; daemon never dials out.** Subscriptions live on
   the connection. `close` is liveness. No fsnotify, no pid-named
   sockets, no `kill(pid, 0)`.
3. **One binary, stdlib subcommands.** `serve | pub | sub | ls | send |
   plumb`. No cobra. The 2018 `tzpipe` imported cobra and shipped stubs.
   `flag` + a switch on `os.Args[1]` is enough.
4. **Dedicated daemon, auto-started.** Yazi's "first instance is the
   broker" is right for one app and wrong for a shared bus: when that
   TUI exits the bus dies. `ttybus pub` starts `ttybus serve --daemon`
   if the socket is missing. systemd `--user` is optional, not required.
5. **Text protocol, IRC/syslog shaped.** Verbs uppercase, rest of line
   is payload. JSON only inside payloads.
6. **Pub/sub is the core; SEND is unicast to a peer id; plumb is a
   later CLI that publishes.** Do not fold routing rules into the
   daemon in v0.
7. **CLI never opens `/dev/tty`.** Logs on stderr. Line-flush every
   message. `--limit`, `--timeout`, `--log` as specified in PROTOCOL.md.
8. **Socket in `$XDG_RUNTIME_DIR`**, not cache. Named buses under
   `buses/<name>.sock`. `--socket` and `$TTYBUS_SOCKET` override.
9. **Bounded per-client outbound queue (256), drop + `ERR SLOW`.**
   Never block publisher A because subscriber B is stuck. This is the
   daemon's only backpressure policy.
10. **Go 1.24, module `github.com/tjstebbing/ttybus`.** Matches the
    rest of this machine's TUIs. Zero third-party deps in v0 (no cobra,
    no fsnotify, no zap). Bubble Tea is an *example* dependency only.
11. **v0 payloads are single-line.** Document it. Do not invent escaping
    until a real client needs it.
12. **Language bindings are the protocol.** The Go `client/` package is
    a convenience, not the spec. Python/Rust/shell should speak lines.

## Architecture

```
┌─────────────┐     SOCK_STREAM      ┌──────────────────┐
│  TUI (yazi) │◄────────────────────►│                  │
└─────────────┘   HELLO/SUB/MSG      │  ttybus serve    │
┌─────────────┐                      │  (user daemon)   │
│ ttybus sub  │◄────────────────────►│                  │
│     | grep  │   stdout/stdin pipe  │  map[chan][]conn │
│ ttybus pub  │◄────────────────────►│                  │
└─────────────┘                      └──────────────────┘
┌─────────────┐
│   nc -U     │─────────────────────────────────────────┘
└─────────────┘
```

Process roles:

| role | stdin | stdout | socket |
|---|---|---|---|
| TUI | keyboard | screen | bus |
| `ttybus pub` | messages (or argv) | nothing | bus |
| `ttybus sub` | nothing | messages | bus |
| `ttybus serve` | nothing | nothing (or logs) | listen |

Environment:

| var | meaning |
|---|---|
| `TTYBUS_SOCKET` | absolute socket path |
| `TTYBUS_BUS` | named bus (`--bus`) |
| `TTYBUS_ID` | HELLO `id=` (set by a parent TUI for its subshells) |
| `XDG_RUNTIME_DIR` | default socket parent |
| `DEBUG` | daemon logs to stderr |

## Daemon internals

Package `internal/daemon`:

- `Listen(path)` — mkdir 0700, unlink stale sock on ECONNREFUSED, bind,
  accept loop.
- One goroutine per connection: read lines, dispatch, write replies
  and `MSG`.
- `Hub` mutex-protects:
  - `conns map[id]*Conn`
  - `subs map[channel]map[id]*Conn`
- `Conn` has: net.Conn, id, hello attrs, outbound `chan line` (cap 256),
  a writer goroutine that is the only writer to the socket.
- Slow client: `select { case q <- msg: default: send ERR SLOW }`.
  Never `close` the queue from two goroutines; document the shutdown
  sequence (unregister, close outbound, wait writer, close net.Conn).
- Single-flight serve: pid file *or* `flock` on a `bus.lock` next to
  the socket. Prefer `flock` (no stale pid). Second `serve --daemon`
  that cannot take the lock and can connect exits 0.

Do not use `bufio.Scanner` without `Buffer` + `MaxScanTokenSize` set to
the 64 KiB limit. Prefer `bufio.Reader.ReadSlice('\n')` / `ReadBytes`
and reject oversize.

`internal/socket` resolves the path (flag > env > named bus > default)
and implements auto-start. Keep this out of `daemon` so the CLI can
call it without importing the server.

## CLI contract (the pipe path)

This is the part that has to be right or the project is a toy. Tests
must cover it; see Testing.

`pub`:

- `ttybus pub CHAN word word` → one PUB, payload = `strings.Join(words, " ")`
- `ttybus pub CHAN` with stdin a pipe/file → one PUB per line, EOF ends
- `ttybus pub CHAN` with stdin a TTY → usage error (do not hang waiting
  for keyboard). Detect with `os.Stdin.Stat()` ModeCharDevice.
- `--limit N` stops after N lines from stdin

`sub`:

- prints **payload only** by default (strip `MSG <chan> `)
- `--raw` prints the whole `MSG` line
- `--limit N` exits 0 after N messages
- `--timeout D` (Go duration, e.g. `2s`) → exit 124 on expiry
- `--log` dumps HELLO/OK/ERR to stderr
- always `Flush` after each stdout line (`os.Stdout` wrapped in
  `bufio.Writer` is fine if you Flush; or `fmt.Fprintln` + no wrapper)

General:

- never `log` to stdout
- `signal.Ignore` nothing; default SIGPIPE
- auto-start on connect failure as in PROTOCOL.md
- `--socket`, `--bus` on every subcommand

The golden pipeline, which must have an integration test:

```sh
ttybus sub animals | grep '^cat' | ttybus pub cats
```

## Client library (`client/`)

~100 lines. Public API sketch:

```go
c, err := ttybus.Dial(ttybus.DefaultSocket()) // or DialPath
c.Hello(ttybus.Info{Name: "mytui", PID: os.Getpid()})
ch, err := c.Sub("files")          // <-chan string payloads
c.Pub("files", path)
c.Close()
```

Must be usable from a Bubble Tea `tea.Cmd` / `tea.Program` without
deadlocking the event loop: the library owns the read goroutine, the
channel is buffered, `Close` unblocks the reader.

No dependency on `internal/daemon`. It may import `internal/protocol`
(or we move protocol to a non-internal package if external language
docs need it — keep it internal until a second client exists; PROTOCOL.md
is the external spec).

## Plumber (later slice)

Not in the daemon. `ttybus plumb [text]` reads `~/.config/ttybus/plumb`
(or `$TTYBUS_PLUMB`) and then either `PUB`s to a channel or `exec`s.
v0 rules, first match wins:

```
# ~/.config/ttybus/plumb
match ^([^\\s:]+):(\\d+)$
  pub editor.open

match ^https?://
  exec open $0

match \\.(go|rs|py)$
  pub editor.open
```

`$0` is the whole text, `$1`… capture groups. `exec` is `exec.Command`
with no shell unless the line is `exec sh -c '…'`. Keep it dumb; this
is Plan 9 plumber's shadow, not a rewrite.

## TUI integration

`examples/bubbletea` is the adoption story:

```go
p := tea.NewProgram(model, ttybus.WithChannel("files"))
```

`WithChannel` is a `tea.ProgramOption` (or a Cmd that yields `BusMsg`).
A received payload becomes `type BusMsg struct { Channel, Payload string }`.
The example TUI shows incoming lines and lets the user PUB the current
selection with a key. That is the screenshot/demo.

Do not add ratatui/textual bindings in this repo.

## Security

- Socket dir `0700`, socket `0600`, never under `~/Library/Caches` or `/tmp`
  world-writable.
- No world-readable logs of payloads by default.
- Named buses do not change the trust model: still same-uid.
- `exec` in plumber is the dangerous bit: only in the later slice, only
  from a user-owned config file, never from a bus message.

## Testing

Layer it so the pipe path cannot regress.

1. **Parser unit tests** (`internal/protocol`) — golden lines in
   `testdata/protocol/*.txt`. Include HELLO attrs, empty payload, oversize
   reject, illegal channel, unknown verb.
2. **Hub unit tests** (`internal/daemon`) — in-memory net.Pipe pairs.
   Fan-out to 0, 1, N subscribers; unicast SEND; slow client drop;
   disconnect unsubscribes; HELLO-before-command.
3. **CLI pipe tests** — `serve` on a temp socket, `pub`/`sub` as
   `exec.Command` with stdin/stdout pipes. Assert:
   - a message published after sub is connected arrives
   - `sub | grep | pub` delivers the filtered line to a second sub
   - stdout of `sub` is not block-buffered (publish, read one line
     without closing the publisher — this is the 2018 bug)
   - stdin TTY detection: skip or stub
   - `--limit 1` exits 0
   - `--timeout 50ms` exits 124 with no message
   - stale socket: serve unlinks and listens
   - second serve --daemon exits 0
4. **No race** — `go test -race ./...` in the Makefile `test` target
   once the daemon exists.

Do not mock the protocol parser in daemon tests; use the real one.

## Repo layout (this is the scaffold)

```
ttybus/
  LICENSE
  README.md
  PLAN.md                  this file
  AGENTS.md
  Makefile
  go.mod
  .gitignore
  cmd/ttybus/              one main; subcommands in same package
  internal/protocol/       parse/print lines
  internal/daemon/         hub + accept loop
  internal/socket/         path, flock, auto-start
  client/                  public Go helper
  examples/bubbletea/      demo TUI (own go files, may add bubbletea dep later)
  contrib/tmux.conf
  contrib/systemd/ttybus.service
  docs/PROTOCOL.md
  testdata/protocol/       golden lines
```

Keep the module dependency-free until `examples/bubbletea` needs
charmbracelet; that example may become `examples/bubbletea/` with its
own `go.mod` if we do not want bubbletea on the library. Prefer a
separate `examples/bubbletea/go.mod` when that slice lands.

## Implementation slices

Execute in this order. Each slice should leave `go test ./...` green
and `go build ./cmd/ttybus` working. Do not start slice N+1 with a red
tree.

### Slice 1 — Protocol parser

**Done when:** `internal/protocol` parses and prints every verb in
PROTOCOL.md; testdata goldens pass; `go test ./internal/protocol` is
the whole suite.

- Types: `Line` (verb, args, raw), `Hello`, `Err`, channel validator
- `Parse(line string) (Line, error)`
- `Format` helpers so daemon and CLI do not strcat
- Reject oversize *at parse of a complete line*; the 64 KiB cap is
  enforced by the reader in slice 2, but parse should still refuse a
  channel that doesn't match the charset

No network.

### Slice 2 — Daemon

**Done when:** `ttybus serve --foreground --socket /tmp/…` accepts
`nc`, a HELLO/SUB/PUB/MSG round-trip works, slow-client drop is tested
with net.Pipe.

- `cmd/ttybus` grows `serve` (`--foreground`, `--daemon`, `--socket`,
  `--bus`)
- `internal/daemon` Hub + Conn
- `internal/socket` path resolution + stale unlink + flock
- `--daemon`: double-fork or `os.StartProcess` with files /dev/null;
  on macOS a simple `Setsid` + redirect is enough. Do not pull
  `go-daemon`.
- Graceful SIGTERM: stop accept, close conns, unlink socket, release flock
- Logs only if `DEBUG` is set, else discard (minibus did this; keep it)

`pub`/`sub` still stubbed with a usage line.

### Slice 3 — CLI filter (`pub` / `sub`)  ★ critical

**Done when:** the golden pipeline integration test passes, including
the "one line arrives without closing the publisher" flush test.

- Implement `pub` and `sub` exactly as PROTOCOL.md "CLI mapping"
- Auto-start `serve --daemon`
- `--limit`, `--timeout`, `--log`, `--raw`
- stdin TTY guard on `pub`
- Integration tests in `cmd/ttybus` or `internal/clitest`

Do not add `--filter` regex. `grep` exists. The 2018 README reimplemented
filters; that is not Unix.

### Slice 4 — `ls`, identity, named buses

**Done when:** two `--bus` sockets can run at once; `ttybus ls` shows
HELLO attrs; `$TTYBUS_ID` round-trips.

- `LIST` / `PEER`
- `--bus` / `$TTYBUS_BUS` path under `buses/`
- document tmux: `TTYBUS_BUS=$TMUX` in a snippet, not code

### Slice 5 — `send --to` and public `client/`

**Done when:** a tiny Go program (not a TUI) Dials, Subs, receives a
SEND; example in `client` doc comment.

- `SEND` verb
- `client` package as sketched
- Keep the CLI `send` thin over the same protocol helpers the daemon uses

### Slice 6 — Bubble Tea example

**Done when:** `cd examples/bubbletea && go run .` shows MSG lines and
can PUB. Separate go.mod if it would otherwise add charmbracelet to
the root module.

### Slice 7 — Plumber

**Done when:** a fixture plumb file routes `main.go:42` to `PUB
editor.open` and a URL to a fake `exec` (record argv, do not
`open(1)` in tests).

- `ttybus plumb`
- config path `$XDG_CONFIG_HOME/ttybus/plumb`
- no daemon changes

### Slice 8 — contrib + polish

- `contrib/tmux.conf` — example `status-right` from a cached `sub`
  is probably too racy; instead document `run-shell 'ttybus pub tmux #{session_name}'`
  on session create, and a one-liner to `send --to`
- `contrib/systemd/ttybus.service` — `Type=simple`, `ExecStart=ttybus serve --foreground`
- README: drop "nothing is implemented yet", add install
- Optional: `ttybus serve` man-style `-h` text

## What not to do while executing

- Do not reintroduce fsnotify, datagrams, or `$PID-$CHANNEL` files
- Do not add cobra, viper, zap, grpc, nats
- Do not persist messages
- Do not implement `--filter`
- Do not open `/dev/tty` from the CLI
- Do not block the hub mutex while writing to a client
- Do not use `UserCacheDir`
- Do not split into multiple modules/repos until there is a second language
- Do not start slice 6–7 because they are more fun than flush tests

## Open questions (locked unless you say otherwise)

These were decided in the 2018 post-mortem so execution is not blocked:

- Language: Go
- Binary name: `ttybus`
- Default bus: per-user, not per-tmux (named bus is opt-in)
- Publisher receives its own PUB if subscribed (like MQTT, unlike some IRC)
- `sub` stdout is payload-only by default
- Timeout exit code 124
- No Windows in v0

If a slice discovers a real conflict with PROTOCOL.md, update PROTOCOL.md
in the same change and mention it; do not silently diverge.
