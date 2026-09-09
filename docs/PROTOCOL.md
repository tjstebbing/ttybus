# ttybus protocol

Version 0. UTF-8 text, one command per line, `\n` terminated.
`\r` is stripped if present. The daemon and the CLI speak this; so can
`nc -U`.

This is the spec. Implementation lives in `internal/protocol`.

## Transport

- `AF_UNIX` `SOCK_STREAM` (not datagrams, not `SOCK_SEQPACKET`).
- Default socket: `$XDG_RUNTIME_DIR/ttybus/bus.sock`.
- Fallback if `XDG_RUNTIME_DIR` is unset: `$TMPDIR/ttybus-<uid>/bus.sock`.
- Directory mode `0700`, socket mode `0600`.
- Named bus: `$XDG_RUNTIME_DIR/ttybus/buses/<name>.sock` via `--bus NAME`
  or `$TTYBUS_BUS`.
- Override: `--socket PATH` or `$TTYBUS_SOCKET`.

Clients **connect**. The daemon never dials out, never watches a
directory for socket files.

Maximum line length: 64 KiB including the newline. Longer lines are a
hard `ERR LINELEN` and the connection is closed.

## Framing

One line = one message. Payloads in v0 **must not contain a raw newline**.
If a future version needs multiline payloads it will add an explicit
length-prefixed frame; v0 will not guess.

Tokens are split on ASCII space. The first token is the verb (uppercase).
Remaining structure is verb-specific. Unknown verbs → `ERR VERB`.

Empty lines are ignored.

## Verbs (client → daemon)

### `HELLO [key=value ...]`

Must be the first non-empty line. Further commands before HELLO →
`ERR HELLO`.

Keys (all optional):

| key    | meaning                                      |
|--------|----------------------------------------------|
| `name` | application name (`yazi`, `btop`, `ttybus`)  |
| `pid`  | process id                                   |
| `pane` | multiplexer pane id (`%3`, zellij pane, …)   |
| `cwd`  | working directory                            |
| `id`   | client-chosen id; otherwise the daemon assigns one |

`id` must match `^[A-Za-z0-9._:-]{1,64}$`. Duplicate `id` → `ERR ID`.

Reply: `OK id=<assigned-or-echoed>`

The CLI sets `name=ttybus`, `pid`, and `id` from `$TTYBUS_ID` when present.

### `SUB <channel>`

Subscribe this connection to `channel`. Duplicate SUB is idempotent.
Reply: `OK`

### `UNSUB <channel>`

Unsubscribe. Unknown channel is not an error.
Reply: `OK`

### `PUB <channel> [payload...]`

Broadcast `payload` (the remainder of the line after the channel, may be
empty) to every current subscriber of `channel`, **including the publisher
if it is subscribed**.

Reply to publisher: `OK n=<subscriber-count>`
Delivered to subscribers as: `MSG <channel> [payload...]`

A `PUB` to a channel with zero subscribers is still `OK n=0`. The daemon
does not retain messages. There is no persistence in v0.

### `SEND <id> <channel> [payload...]`

Unicast. Deliver `MSG <channel> [payload...]` only to the connection whose
HELLO `id` is `<id>`, regardless of subscription. If you want
subscription-gated unicast, check `LIST` first; v0 SEND is addressed to a
peer, not a channel membership.

Missing peer → `ERR PEER`
Reply on success: `OK`

### `LIST`

Reply: zero or more `PEER` lines, then `OK n=<count>`

```
PEER id=yazi-1 name=yazi pid=4123 pane=%3 cwd=/code/foo subs=files,build
PEER id=cli-9 name=ttybus pid=4188
OK n=2
```

`subs` is a comma-separated list, omitted if empty.

## Verbs (daemon → client)

### `MSG <channel> [payload...]`

A published (or sent) payload. Clients must tolerate `MSG` at any time
after HELLO.

### `OK [key=value ...]`

Success for the in-flight request. Requests are not pipelined in v0: the
client sends one command, waits for `OK` or `ERR`, then sends the next.
`MSG` lines may arrive between request and reply; they are not replies.

The CLI `sub` command is the exception: after `SUB` + `OK` it prints
`MSG` payloads to stdout (channel name stripped by default) and does not
send further commands except on exit.

### `ERR <code> <message...>`

| code    | when                                      |
|---------|-------------------------------------------|
| `HELLO` | command before HELLO, or bad HELLO        |
| `VERB`  | unknown verb                              |
| `CHAN`  | missing/illegal channel name              |
| `ID`    | illegal or duplicate client id            |
| `PEER`  | SEND to unknown id                        |
| `LINE`  | malformed line                            |
| `LINELEN` | line exceeded 64 KiB                    |
| `SLOW`  | this client's outbound queue overflowed   |

`ERR` does not always close the connection. `LINELEN` does.

## Channel names

```
channel = 1*64 ( ALPHA / DIGIT / "." / "_" / "-" / "/" )
```

Must start with `ALPHA`. Examples: `files`, `build.ok`, `editor/open`.

No globbing in v0. No implied hierarchy beyond what clients choose to
put in the name.

## Fan-out and backpressure

Each connection has a bounded outbound queue (256 messages). The daemon
**never blocks** a publisher on a slow subscriber.

- If a subscriber's queue is full, that message is dropped for that
  subscriber only and the subscriber is sent `ERR SLOW <channel>`.
- Other subscribers are unaffected.
- The publisher still gets `OK n=<count>` where `n` is the number of
  subscribers at enqueue time, not the number that actually received.

On `write(2)` error or close, the connection is dropped, its
subscriptions removed. That is the only liveness mechanism. No pidfiles,
no `kill(pid, 0)`, no fsnotify.

## CLI mapping

The CLI is a filter over this protocol. It never opens `/dev/tty`.

| command | on the wire |
|---------|-------------|
| `ttybus pub CHAN [words…]` | `HELLO` then `PUB CHAN words…` then exit |
| `echo line \| ttybus pub CHAN` | `HELLO` then one `PUB` per stdin line |
| `ttybus sub CHAN` | `HELLO` then `SUB CHAN`; print payload of each `MSG` to stdout |
| `ttybus sub CHAN --limit 1` | as above, exit 0 after one `MSG` |
| `ttybus ls` | `HELLO` then `LIST`; print `PEER` lines |
| `ttybus send --to ID CHAN [words…]` | `HELLO` then `SEND ID CHAN words…` |

Stdout is line-flushed after every message (`\n` + flush), including when
stdout is a pipe. Diagnostics go to stderr. `--log` prints protocol
traffic to stderr.

Exit codes: `0` success or clean EOF from the daemon; `1` usage/protocol
error; `2` could not connect (and auto-start failed); `130` SIGINT.
`SIGPIPE` is the default (die like `head`).

`--timeout` is a deadline for receiving the first (or `--limit`-th)
message; on expiry exit `0` with no output if nothing arrived? **No:**
exit `124` (same as GNU `timeout`) so scripts can distinguish "got
nothing" from "got an empty payload".

## Auto-start

If `connect(2)` fails with `ENOENT` or `ECONNREFUSED`:

1. Spawn `ttybus serve --daemon` (detached, same `--bus`/`--socket`).
2. Retry connect with short backoff (e.g. 10 attempts, 20ms then doubling,
   cap 200ms).
3. If still down, exit `2`.

`ttybus serve --daemon` is idempotent: if the socket already has a live
owner, the second serve exits `0` without replacing it. Stale sockets
(file exists, connect refused) are unlinked by `serve` before listen.

## What v0 is not

- Not JSON-RPC, not D-Bus, not MQTT.
- Not a FIFO directory, not 9P.
- Not persistent. Restart the daemon, subscriptions and in-flight
  messages are gone.
- Not authenticated beyond unix socket permissions.
- Not a plumber. `ttybus plumb` is a later command that *uses* this
  protocol; it does not change it.
