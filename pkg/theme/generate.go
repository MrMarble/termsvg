//go:generate go run ../../cmd/themegen

package theme

// This file contains the go:generate directive for theme code generation.
// Run `go generate ./pkg/theme/` to regenerate built-in themes from JSON files.
//
// The generator reads all .json files from the themes/ directory and generates
// pkg/theme/builtin.go with theme constants and a registry map.
