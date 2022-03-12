# termsvg

[![golangci-lint](https://github.com/MrMarble/termsvg/actions/workflows/golangci-lint.yml/badge.svg)](https://github.com/MrMarble/termsvg/actions/workflows/golangci-lint.yml)
[![pre-commit](https://img.shields.io/badge/pre--commit-enabled-brightgreen?logo=pre-commit&logoColor=white)](https://github.com/pre-commit/pre-commit)
![GitHub release (latest SemVer)](https://img.shields.io/github/v/release/mrmarble/termsvg)
[![Go Reference](https://pkg.go.dev/badge/github.com/mrmarble/termsvg.svg)](https://pkg.go.dev/github.com/mrmarble/termsvg)

CLI tool to record, share and export your terminal as a animated SVG image.
It uses the same format as [asciinema](https://asciinema.org) so it should be compatible both ways.

---
## Usage

```
Usage: termsvg <command>

A cli tool for recording terminal sessions

Flags:
  -h, --help     Show context-sensitive help.
      --debug    Enable debug mode.

Commands:
  play <file>
    Play a recording.

  rec <file>
    Record a terminal sesion.

  export
    Export asciicast.

Run "termsvg <command> --help" for more information on a command.

termsvg: error: expected one of "play",  "rec",  "export"
```

## Example

Asciinema recording [inverted pendulum ](https://asciinema.org/a/444816)
![inverted pendulum](examples/444816.svg)

More examples at the [examples](examples) folder