//go:build !windows

package processalive

import (
	"errors"
	"syscall"
	"time"
)

func Alive(pid int) bool {
	if pid <= 0 {
		return false
	}
	err := syscall.Kill(pid, 0)
	return err == nil || errors.Is(err, syscall.EPERM)
}

// Created is unknown on Unix. Unix hands out pids in order and reuses one
// only after the counter wraps, so a run file needs no creation time there.
func Created(int) (time.Time, bool) {
	return time.Time{}, false
}
