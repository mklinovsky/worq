// Command worq manages git worktrees and per-project setup.
package main

import (
	"os"

	"github.com/mklinovsky/worq/internal/cli"
)

func main() {
	os.Exit(cli.Main(os.Args[1:]))
}
