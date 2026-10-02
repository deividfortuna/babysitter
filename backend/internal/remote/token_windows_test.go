//go:build windows

package remote

import (
	"testing"
	"time"

	"golang.org/x/sys/windows"
)

const holdFor = 30 * time.Millisecond

// holdExclusive opens the file the way a program that shares nothing
// does, so every other open and every rename over it fails until release.
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

func TestRotateTokenWaitsForAReaderOfTheTokenFile(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	if _, err := Token(dir); err != nil {
		t.Fatal(err)
	}
	release := holdExclusive(t, TokenPath(dir))
	time.AfterFunc(holdFor, release)

	rotated, err := RotateToken(dir)
	if err != nil {
		t.Fatalf("RotateToken() error = %v, want the new token once the reader lets go", err)
	}
	if got := CurrentToken(dir); got != rotated {
		t.Fatalf("CurrentToken() = %q, want the rotated %q", got, rotated)
	}
}

func TestCurrentTokenWaitsForAWriterOfTheTokenFile(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	token, err := Token(dir)
	if err != nil {
		t.Fatal(err)
	}
	release := holdExclusive(t, TokenPath(dir))
	time.AfterFunc(holdFor, release)

	if got := CurrentToken(dir); got != token {
		t.Fatalf("CurrentToken() = %q, want %q once the writer lets go", got, token)
	}
}
