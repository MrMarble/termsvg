# termsvg

CLI tool to record, share and export your terminal as a animated SVG image.
It uses the same format as [asciinema](https://asciinema.org) so it should be compatible both ways.


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