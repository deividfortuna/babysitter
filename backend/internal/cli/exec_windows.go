//go:build windows

package cli

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"os/signal"
)

// execInWorktree runs the command in place of this process, as the exec
// of Unix does: it holds the terminal until the user ends the command, and
// nothing cancels it, so it takes the background context.
func execInWorktree(dir string, argv []string) error {
	path, err := exec.LookPath(argv[0])
	if err != nil {
		return err
	}
	cmd := exec.CommandContext(context.Background(), path, argv[1:]...)
	cmd.Dir = dir
	cmd.Env = envIn(dir)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	signal.Ignore(os.Interrupt)
	err = cmd.Run()
	if exitErr, ok := errors.AsType[*exec.ExitError](err); ok {
		os.Exit(exitErr.ExitCode())
	}
	if err != nil {
		return err
	}
	os.Exit(0)
	return nil
}
