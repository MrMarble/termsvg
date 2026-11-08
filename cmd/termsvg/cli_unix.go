//go:build !windows

package main

import "github.com/mrmarble/termsvg/cmd/termsvg/record"

// platformCommands are the commands that need a PTY and are unix-only.
type platformCommands struct {
	Record record.Cmd `cmd:"" help:"Record a terminal session"`
}
