//go:build windows

package supervisor

import (
	"net"
	"strings"
	"testing"
	"time"

	"github.com/Microsoft/go-winio"
)

func dialLink(addr string) (net.Conn, error) {
	timeout := 2 * time.Second
	return winio.DialPipe(addr, &timeout)
}

func TestListenNamesAPipePerListener(t *testing.T) {
	dir := t.TempDir()
	first, firstAddr, err := Listen(t.Context(), dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { first.Close() })

	second, secondAddr, err := Listen(t.Context(), dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { second.Close() })

	if !strings.HasPrefix(firstAddr, PipePrefix) {
		t.Fatalf("address = %s, want a named pipe", firstAddr)
	}
	if firstAddr == secondAddr {
		t.Fatalf("both listeners took %s, so one takes the link of the other", firstAddr)
	}
	// A named pipe takes a client only while its listener accepts.
	go func() {
		if c, err := first.Accept(); err == nil {
			c.Close()
		}
	}()
	conn, err := dialLink(firstAddr)
	if err != nil {
		t.Fatalf("the first pipe is gone: %v", err)
	}
	conn.Close()
}
