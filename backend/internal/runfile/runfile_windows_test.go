//go:build windows

package runfile

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"golang.org/x/sys/windows"
)

// holdExclusive opens the file the way a program that shares nothing
// does, so every other open fails with a sharing violation until release.
func holdExclusive(t *testing.T, path string) (release func()) {
	t.Helper()
	name, err := windows.UTF16PtrFromString(path)
	if err != nil {
		t.Fatal(err)
	}
	h, err := windows.CreateFile(name, windows.GENERIC_READ, 0, nil, windows.OPEN_EXISTING, windows.FILE_ATTRIBUTE_NORMAL, 0)
	if err != nil {
		t.Fatal(err)
	}
	return func() { _ = windows.CloseHandle(h) }
}

func TestReadWaitsForAFileAnotherProcessHolds(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), FileName)
	if err := Write(path, Info{PID: 42, Port: 8080, StartedAt: time.Now()}); err != nil {
		t.Fatal(err)
	}
	release := holdExclusive(t, path)
	if _, err := os.ReadFile(path); !errors.Is(err, windows.ERROR_SHARING_VIOLATION) {
		release()
		t.Fatalf("a plain read error = %v, want the sharing violation the hold causes", err)
	}
	time.AfterFunc(3*busyPause, release)

	info, err := Read(path)
	if err != nil {
		t.Fatalf("Read() error = %v, want the file once the hold ends", err)
	}
	if info == nil || info.PID != 42 {
		t.Fatalf("Read() = %+v", info)
	}
}

func TestReadGivesUpOnAFileHeldForTooLong(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), FileName)
	if err := Write(path, Info{PID: 42, Port: 8080, StartedAt: time.Now()}); err != nil {
		t.Fatal(err)
	}
	release := holdExclusive(t, path)
	defer release()

	if _, err := Read(path); !errors.Is(err, windows.ERROR_SHARING_VIOLATION) {
		t.Fatalf("Read() error = %v, want the sharing violation after the tries", err)
	}
}

func TestWriteWaitsForAFileAnotherProcessHolds(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), FileName)
	if err := Write(path, Info{PID: 1, Port: 1, StartedAt: time.Now()}); err != nil {
		t.Fatal(err)
	}
	release := holdExclusive(t, path)
	time.AfterFunc(3*busyPause, release)

	if err := Write(path, Info{PID: 2, Port: 2, StartedAt: time.Now()}); err != nil {
		t.Fatalf("Write() error = %v, want the replacement once the hold ends", err)
	}
	info, err := Read(path)
	if err != nil || info == nil || info.PID != 2 {
		t.Fatalf("Read() = %+v, %v", info, err)
	}
}

// Windows hands a freed pid to the next process soon. A process created
// after the run file was written has only taken the pid of the daemon.
func TestLiveIgnoresAProcessThatTookTheFreedPid(t *testing.T) {
	path := filepath.Join(t.TempDir(), FileName)
	if err := Write(path, Info{PID: os.Getpid(), Port: 1, StartedAt: time.Now().Add(-time.Hour)}); err != nil {
		t.Fatal(err)
	}
	live, err := Live(path)
	if err != nil {
		t.Fatal(err)
	}
	if live != nil {
		t.Fatalf("Live = %+v for a pid that a newer process took", live)
	}
}
