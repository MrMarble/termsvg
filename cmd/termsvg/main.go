package main

import (
	"fmt"
	"os"
)

func main() {
	if err := run(os.Args[1:], os.Stdout, os.Stderr, os.Exit); err != nil {
		fmt.Fprintf(os.Stderr, "termsvg: error: %v\n", err)
		os.Exit(1)
	}
}
