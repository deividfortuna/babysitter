//go:build windows

package session

import (
	"context"
	"errors"
)

var ErrUnsupported = errors.New("agent sessions are not supported on this platform")

type PTY struct {
	Timing Timing
}

func New() *PTY {
	return &PTY{Timing: DefaultTiming}
}

func (h *PTY) Start(context.Context, Spec) (Handle, error) {
	return nil, ErrUnsupported
}
