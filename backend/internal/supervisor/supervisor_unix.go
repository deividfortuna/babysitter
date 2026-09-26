//go:build !windows

package supervisor

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"sync/atomic"
)

const SocketName = "supervisor.sock"

const maxSocketPath = 100

var fallbackSeq atomic.Uint64

func Listen(ctx context.Context, dataDir string) (net.Listener, string, error) {
	addr := filepath.Join(dataDir, SocketName)
	if len(addr) > maxSocketPath {
		addr = filepath.Join(os.TempDir(), fmt.Sprintf("babysitter-%d-%d.sock", os.Getpid(), fallbackSeq.Add(1)))
	}
	if err := os.Remove(addr); err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, "", err
	}
	var lc net.ListenConfig
	ln, err := lc.Listen(ctx, "unix", addr)
	if err != nil {
		return nil, "", err
	}
	return ln, addr, nil
}
