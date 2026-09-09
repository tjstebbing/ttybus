// Package daemon is the ttybus server: accept loop, subscription hub,
// per-connection reader/writer.
//
// Clients connect in. The daemon never dials out and never watches the
// filesystem for peer sockets. Slice 2 of PLAN.md lives here.
package daemon
