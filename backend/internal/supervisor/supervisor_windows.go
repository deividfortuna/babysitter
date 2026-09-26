//go:build windows

package supervisor

import (
	"context"
	"errors"
	"net"
)

var ErrUnsupported = errors.New("supervisor socket is not supported on windows")

func Listen(ctx context.Context, dataDir string) (net.Listener, string, error) {
	return nil, "", ErrUnsupported
}
