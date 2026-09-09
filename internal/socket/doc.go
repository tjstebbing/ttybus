// Package socket resolves the ttybus socket path, takes the listen lock,
// unlinks stale sockets, and auto-starts the daemon for CLI commands.
//
// Path precedence: --socket / TTYBUS_SOCKET, then --bus / TTYBUS_BUS,
// then $XDG_RUNTIME_DIR/ttybus/bus.sock (or $TMPDIR/ttybus-<uid>/).
package socket
