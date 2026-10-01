//go:build windows

package processalive

import (
	"os"
	"os/exec"
	"testing"

	"golang.org/x/sys/windows"
)

func TestAliveSeesThisProcess(t *testing.T) {
	t.Parallel()
	if !Alive(os.Getpid()) {
		t.Fatal("Alive(self) = false")
	}
	if Alive(0) || Alive(-1) {
		t.Fatal("Alive of no pid = true")
	}
}

// A process that exited keeps its pid while a handle to it stays open,
// as the parent that killed a daemon holds one for a moment.
func TestAliveIsFalseForAnExitedProcessWithAnOpenHandle(t *testing.T) {
	t.Parallel()
	cmd := exec.Command("cmd", "/d", "/c", "exit 0")
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	pid := cmd.Process.Pid
	h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(pid))
	if err != nil {
		t.Fatal(err)
	}
	defer windows.CloseHandle(h)
	if err := cmd.Wait(); err != nil {
		t.Fatal(err)
	}
	if Alive(pid) {
		t.Fatalf("Alive(%d) = true for a process that exited", pid)
	}
}
