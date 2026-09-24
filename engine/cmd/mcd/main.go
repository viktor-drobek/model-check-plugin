// Command mcd is the model-check engine's command line (plan 14 §6). All
// behaviour lives in package cli so that tests can run it in-process.
package main

import (
	"os"

	"modelcheck/cli"
)

func main() {
	os.Exit(cli.Run(os.Args[1:], os.Stdout, os.Stderr))
}
