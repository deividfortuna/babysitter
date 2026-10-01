//go:build !windows

package supervisor

import (
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func dialLink(addr string) (net.Conn, error) {
	return net.Dial("unix", addr)
}

func TestListenFallbackIsUniquePerListener(t *testing.T) {
	first, firstAddr, err := Listen(t.Context(), longDataDir(t))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { first.Close() })

	second, secondAddr, err := Listen(t.Context(), longDataDir(t))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { second.Close() })

	if firstAddr == secondAddr {
		t.Fatalf("both listeners took %s, so one unlinks the socket of the other", firstAddr)
	}
	conn, err := dialLink(firstAddr)
	if err != nil {
		t.Fatalf("the first socket is gone: %v", err)
	}
	conn.Close()
}

func longDataDir(t *testing.T) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), strings.Repeat("d", 80))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	return dir
}
