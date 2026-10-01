//go:build windows

package processalive

import (
	"math"

	"golang.org/x/sys/windows"
)

// stillActive is the exit code GetExitCodeProcess gives for a process
// that has not exited, STILL_ACTIVE in the Windows headers.
const stillActive = 259

// Alive reports whether the process runs. Opening the process is not
// enough: a process that exited keeps its object, and so its pid, while
// anyone holds a handle to it, and OpenProcess still succeeds on it.
func Alive(pid int) bool {
	if pid <= 0 || pid > math.MaxUint32 {
		return false
	}
	h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(pid))
	if err != nil {
		return false
	}
	defer func() { _ = windows.CloseHandle(h) }()
	var code uint32
	if err := windows.GetExitCodeProcess(h, &code); err != nil {
		return false
	}
	return code == stillActive
}
