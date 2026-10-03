package httpd

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"sync"
	"time"
)

type Server struct {
	http *http.Server
	ln   net.Listener

	stopOnce sync.Once
	stopped  chan struct{}
}

func Listen(ctx context.Context, port int, handler http.Handler) (*Server, error) {
	return ListenAddr(ctx, fmt.Sprintf("127.0.0.1:%d", port), loopbackHostOnly(handler))
}

func ListenAddr(ctx context.Context, addr string, handler http.Handler) (*Server, error) {
	var lc net.ListenConfig
	ln, err := lc.Listen(ctx, "tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("listen: %w", err)
	}
	return &Server{
		http: &http.Server{
			Handler:           handler,
			ReadHeaderTimeout: 10 * time.Second,
		},
		ln:      ln,
		stopped: make(chan struct{}),
	}, nil
}

func (s *Server) Port() int {
	return s.ln.Addr().(*net.TCPAddr).Port
}

func (s *Server) Addr() string {
	return s.ln.Addr().String()
}

func (s *Server) Serve(ctx context.Context) error {
	errc := make(chan error, 1)
	go func() { errc <- s.http.Serve(s.ln) }()

	select {
	case err := <-errc:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	case <-ctx.Done():
	case <-s.stopped:
	}

	drain, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := s.http.Shutdown(drain); err != nil {
		_ = s.http.Close()
	}
	<-errc
	return nil
}

func (s *Server) RequestShutdown() {
	s.stopOnce.Do(func() { close(s.stopped) })
}
