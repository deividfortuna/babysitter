//go:build windows

package ghauth

import (
	"context"
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

func TestTheSignInIsReadOnceAnotherProcessLetsTheFileGo(t *testing.T) {
	h := newHarness(t)
	want := h.signIn(t, h.auth())
	path := filepath.Join(h.dir, signInFileName)
	release := holdExclusive(t, path)
	if _, err := os.ReadFile(path); !errors.Is(err, windows.ERROR_SHARING_VIOLATION) {
		release()
		t.Fatalf("a plain read error = %v, want the sharing violation the hold causes", err)
	}
	time.AfterFunc(30*time.Millisecond, release)

	got, err := h.auth().Token(context.Background())

	if err != nil || got != want.AccessToken {
		t.Fatalf("Token = %q, %v; want the token of the app once the hold ends", got, err)
	}
}
