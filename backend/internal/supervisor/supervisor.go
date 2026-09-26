package supervisor

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net"
	"sync"
	"time"
)

type Supervisor struct {
	grace  time.Duration
	onLost func()
	log    *slog.Logger

	mu     sync.Mutex
	linked int
	timer  *time.Timer
}

func New(grace time.Duration, onLost func(), log *slog.Logger) *Supervisor {
	if log == nil {
		log = slog.New(slog.DiscardHandler)
	}
	return &Supervisor{grace: grace, onLost: onLost, log: log}
}

func (s *Supervisor) Serve(ctx context.Context, ln net.Listener) error {
	go func() {
		<-ctx.Done()
		ln.Close()
	}()
	for {
		conn, err := ln.Accept()
		if err != nil {
			if closedOnShutdown(ctx, err) {
				return nil
			}
			return err
		}
		go s.hold(conn)
	}
}

func closedOnShutdown(ctx context.Context, err error) bool {
	return ctx.Err() != nil || errors.Is(err, net.ErrClosed)
}

func (s *Supervisor) hold(conn net.Conn) {
	defer conn.Close()
	s.link()
	_, _ = io.Copy(io.Discard, conn)
	s.unlink()
}

func (s *Supervisor) link() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.linked++
	if s.timer != nil {
		s.timer.Stop()
		s.timer = nil
		s.log.Info("supervisor: link restored, shutdown cancelled")
	} else {
		s.log.Info("supervisor: link established", "links", s.linked)
	}
}

func (s *Supervisor) unlink() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.linked--
	if s.linked > 0 {
		return
	}
	s.log.Info("supervisor: last link lost, shutdown scheduled", "grace", s.grace)
	s.timer = time.AfterFunc(s.grace, func() {
		s.mu.Lock()
		s.timer = nil
		fire := s.linked == 0
		s.mu.Unlock()
		if fire {
			s.onLost()
		}
	})
}
