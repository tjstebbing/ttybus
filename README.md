# ttybus

```
 ________
|[][][]|_\_
| TTYBus   |
=-OO----OO-=
```

A text message bus for TUIs, CLIs, and shell scripts.

Unix programs compose with pipes. TUIs cannot: stdin is the keyboard and
stdout is the screen. **ttybus is the side channel.** TUIs never join a
pipeline. A small CLI does.

```sh
ttybus serve                  # user daemon; also auto-started by pub/sub

ttybus pub  files main.go
echo main.go | ttybus pub files
ttybus sub  files             # lines on stdout until Ctrl-C
ttybus sub  files --limit 1   # one line, then exit 0

# compose like any Unix filter
ttybus sub animals | grep --line-buffered '^cat' | ttybus pub cats

ttybus ls
ttybus send --to yazi-1 files main.go
ttybus plumb main.go:42
```

`grep` block-buffers when its stdout is a pipe. Use GNU
`grep --line-buffered`, or `awk '/^cat/ { print; fflush() }'`.

Speak the protocol yourself:

```sh
nc -U "$XDG_RUNTIME_DIR/ttybus/bus.sock"
HELLO name=demo pid=$$
SUB files
PUB files /tmp/hello.go
```

## Install

```sh
go install github.com/tjstebbing/ttybus/cmd/ttybus@latest
```

From a checkout:

```sh
make build
./ttybus serve --foreground
```

## Why

A TUI running in one tmux pane has no Unix-native way to tell a TUI in
another pane "open this file", or to let a shell script inject an event,
or to emit something `grep` can see without trashing the alternate screen.

Each TUI that has solved this has grown a *private* bus (Yazi DDS, `lf
-remote`, Kakoune sockets). ttybus is the shared one: text lines over a
unix stream socket, a CLI that behaves like `cat`/`tee` for named
channels, an optional plumber for "this looks like a file:line → editor".

This is a clean-room take on an idea from 2018
([ttyzero/minibus](https://github.com/ttyzero/minibus)). Clients
**connect in**; the protocol is newline-delimited text; the CLI is
pipeline-correct (line-flushed, logs on stderr, never opens `/dev/tty`).

## Rules of the road

1. **TUIs never participate in pipelines.** stdin = keys, stdout = screen,
   socket = bus.
2. **The CLI is a Unix filter.** Logs go to stderr. Every message is one
   line, flushed. `--limit` and `--timeout` exist so scripts can block
   without `head` races. Timeout exits `124` like GNU `timeout`.
3. **Text on the wire.** `PUB files {"path":"…"}` is fine; JSON as the
   *envelope* is not. `nc`, `grep`, and `awk` must work.
4. **One user daemon**, socket at `$XDG_RUNTIME_DIR/ttybus/bus.sock`
   (or `$TMPDIR/ttybus-<uid>/bus.sock`). Named buses (`--bus NAME` /
   `$TTYBUS_BUS`) isolate multiplexer sessions.

A TUI can export `TTYBUS_ID` so shells it spawns identify themselves.
Full contract: [docs/PROTOCOL.md](docs/PROTOCOL.md).

## Go helper

```go
c, err := ttybus.Dial(ttybus.DefaultSocket())
_ = c.Hello(ttybus.Info{Name: "mytui"})
ch, _ := c.Sub("files")
_ = c.Pub("files", path)
```

See `client/` and `examples/bubbletea`.

## Plumber

`~/.config/ttybus/plumb` (or `$TTYBUS_PLUMB`):

```
match ^([^\s:]+):(\d+)$
  pub editor.open

match ^https?://
  exec open $0

match \.(go|rs|py)$
  pub editor.open
```

`$0` is the whole text; `$1`… are capture groups. First match wins.

## Layout

```
cmd/ttybus/          serve | pub | sub | ls | send | plumb
internal/protocol/   line parser
internal/daemon/     socket server, subscriptions, fan-out
internal/socket/     path resolution, flock, auto-start
internal/plumb/      rule file
client/              Go helper for TUI authors
examples/bubbletea/  bus messages as Bubble Tea Msg
contrib/             tmux.conf, systemd --user unit
docs/PROTOCOL.md     wire spec
PLAN.md              implementation notes
```

## Prior art

Closest production system is [Yazi DDS](https://yazi-rs.github.io/docs/dds)
(`ya pub` / `ya sub`), which is Yazi-only. Philosophical ancestor is
[Plan 9 plumber](https://9p.io/sys/doc/plumb.html). D-Bus is the desktop
equivalent and is the thing we are deliberately not.

## License

MIT. See [LICENSE](LICENSE).
