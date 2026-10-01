//go:build windows

package supervisor

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"

	"github.com/Microsoft/go-winio"
)

// PipePrefix starts the name of the named pipe that stands in for the
// Unix socket. The run file carries the whole name, and Node connects to
// it with net.connect as it does to the socket.
const PipePrefix = `\\.\pipe\babysitter-supervisor-`

var seq atomic.Uint64

func Listen(_ context.Context, dataDir string) (net.Listener, string, error) {
	addr := pipeName(dataDir)
	ln, err := winio.ListenPipe(addr, nil)
	if err != nil {
		return nil, "", fmt.Errorf("listen on %s: %w", addr, err)
	}
	return ln, addr, nil
}

// pipeName is unique per data directory, process and listener, so two
// daemons never share a pipe and a test can listen twice in one process.
func pipeName(dataDir string) string {
	abs, err := filepath.Abs(dataDir)
	if err != nil {
		abs = dataDir
	}
	sum := sha256.Sum256([]byte(strings.ToLower(filepath.Clean(abs))))
	return fmt.Sprintf("%s%s-%d-%d", PipePrefix, hex.EncodeToString(sum[:8]), os.Getpid(), seq.Add(1))
}
