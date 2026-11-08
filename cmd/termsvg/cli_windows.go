//go:build windows

package main

// platformCommands is empty on Windows: recording needs a PTY.
type platformCommands struct{}
