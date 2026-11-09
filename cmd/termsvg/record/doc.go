// Package record implements the record command, which runs a program in a
// pseudo-terminal and saves its output as an asciicast.
//
// Recording needs a PTY, so the implementation is only built on non-Windows
// platforms; the command is not offered on Windows.
package record
