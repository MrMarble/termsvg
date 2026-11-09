//go:build !windows

package main

import "github.com/mrmarble/termsvg/cmd/termsvg/record"

// hasRecord reports whether the record command is built on this platform.
const hasRecord = true

// platformCommands are the commands that need a PTY and are unix-only.
type platformCommands struct {
	Record record.Cmd `cmd:"" help:"Record a terminal session"`
}
