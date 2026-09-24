// Command mcd is the model-check engine's command line (plan 14 §6). All
// behaviour lives in package cli so that tests can run it in-process.
package main

import (
	"os"

	"modelcheck/cli"
)

func main() {
	os.Exit(dispatch(os.Args[1:], cli.Run)) // `mcd serve` → serve.go (MCP server); everything else → cli.Run
}
