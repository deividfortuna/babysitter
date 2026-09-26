//go:build !windows

package cli

import (
	"os"
	"os/exec"
	"syscall"
)

func execInWorktree(dir string, argv []string) error {
	path, err := exec.LookPath(argv[0])
	if err != nil {
		return err
	}
	if err := os.Chdir(dir); err != nil {
		return err
	}
	return syscall.Exec(path, argv, envIn(dir))
}
