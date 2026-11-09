//go:build windows

package main

// hasRecord reports whether the record command is built on this platform.
const hasRecord = false

// platformCommands is empty on Windows: recording needs a PTY.
type platformCommands struct{}
