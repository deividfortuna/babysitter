package runfile

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestWriteReadRemove(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", FileName)
	want := Info{PID: os.Getpid(), Port: 4321, StartedAt: time.Now().UTC().Truncate(time.Second), Owner: OwnerApp}

	if err := Write(path, want); err != nil {
		t.Fatal(err)
	}
	got, err := Read(path)
	if err != nil {
		t.Fatal(err)
	}
	if got == nil || got.PID != want.PID || got.Port != want.Port || got.Owner != want.Owner {
		t.Fatalf("Read = %+v, want %+v", got, want)
	}

	live, err := Live(path)
	if err != nil {
		t.Fatal(err)
	}
	if live == nil {
		t.Fatal("Live = nil for the current process")
	}

	if err := RemoveIfOwned(path, want.PID+1); err != nil {
		t.Fatal(err)
	}
	if got, _ := Read(path); got == nil {
		t.Fatal("RemoveIfOwned deleted a file owned by another pid")
	}
	if err := RemoveIfOwned(path, want.PID); err != nil {
		t.Fatal(err)
	}
	if got, _ := Read(path); got != nil {
		t.Fatal("RemoveIfOwned kept the file")
	}
	if err := Remove(path); err != nil {
		t.Fatalf("Remove on a missing file: %v", err)
	}
}

func TestReadMissing(t *testing.T) {
	got, err := Read(filepath.Join(t.TempDir(), FileName))
	if err != nil || got != nil {
		t.Fatalf("Read missing = %+v, %v; want nil, nil", got, err)
	}
}

func TestLiveStale(t *testing.T) {
	path := filepath.Join(t.TempDir(), FileName)
	if err := Write(path, Info{PID: 2147483000, Port: 1}); err != nil {
		t.Fatal(err)
	}
	live, err := Live(path)
	if err != nil {
		t.Fatal(err)
	}
	if live != nil {
		t.Fatalf("Live = %+v for a dead pid", live)
	}
}
