# bubbletea example

A tiny TUI that subscribes to a ttybus channel and treats each payload
as a Bubble Tea `Msg`. Type a line and press enter to publish.

```sh
# terminal 1
ttybus serve --foreground

# terminal 2
cd examples/bubbletea && go run . demo

# terminal 3
ttybus pub demo hello
```

This module has its own `go.mod` so charmbracelet does not land on the
root ttybus module.
