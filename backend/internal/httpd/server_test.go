package httpd

import (
	"context"
	"fmt"
	"net/http"
	"testing"
)

func serveFor(t *testing.T, srv *Server) string {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		_ = srv.Serve(ctx)
		close(done)
	}()
	t.Cleanup(func() {
		cancel()
		<-done
	})
	return fmt.Sprintf("http://127.0.0.1:%d/", srv.Port())
}

func statusWithHost(t *testing.T, url, host string) int {
	t.Helper()
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, url, nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Host = host
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	return resp.StatusCode
}

func TestOnlyTheLocalListenerAnswersLoopbackHostsOnly(t *testing.T) {
	ok := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusNoContent) })
	local, err := Listen(context.Background(), 0, ok)
	if err != nil {
		t.Fatal(err)
	}
	remote, err := ListenAddr(context.Background(), "127.0.0.1:0", ok)
	if err != nil {
		t.Fatal(err)
	}

	if got := statusWithHost(t, serveFor(t, local), "evil.example"); got != http.StatusForbidden {
		t.Fatalf("local listener, foreign Host = %d, want 403", got)
	}
	if got := statusWithHost(t, serveFor(t, remote), "server.local:7420"); got != http.StatusNoContent {
		t.Fatalf("remote listener, its own Host = %d, want 204: the remote guard checks its token", got)
	}
}
