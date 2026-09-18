package cli

import (
	"os"
	"os/exec"
)

// execShell runs a command through the user's shell, inheriting the terminal.
func execShell(shell, command, dir string) error {
	cmd := exec.Command(shell, "-c", command)
	cmd.Dir = dir
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	return cmd.Run()
}
