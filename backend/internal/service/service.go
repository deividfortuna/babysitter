package service

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"

	"github.com/deividfortuna/babysitter/internal/execx"
)

var ErrUnsupported = errors.New("service management is only supported on macOS and Linux")

const Label = "com.deividfortuna.babysitter"

const UnitName = "babysitter.service"

type Config struct {
	Executable string
	Args       []string
	LogDir     string
}

type Status struct {
	Installed bool   `json:"installed"`
	Loaded    bool   `json:"loaded"`
	Running   bool   `json:"running"`
	PID       int    `json:"pid,omitempty"`
	Path      string `json:"path"`
	LogPath   string `json:"log_path,omitempty"`
}

type Manager interface {
	Install(ctx context.Context, cfg Config) error
	Uninstall(ctx context.Context) error
	Start(ctx context.Context) error
	Stop(ctx context.Context) error
	Status(ctx context.Context) (Status, error)
}

func NewManager() (Manager, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("user home dir: %w", err)
	}
	switch runtime.GOOS {
	case "darwin":
		return &launchd{home: home, uid: os.Getuid(), run: execx.Run}, nil
	case "linux":
		return &systemd{home: home, run: execx.Run}, nil
	default:
		return nil, ErrUnsupported
	}
}

func DefaultLogDir() string {
	if runtime.GOOS != "darwin" {
		return ""
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, "Library", "Logs", "babysitter")
}

func (c Config) validate() error {
	if c.Executable == "" || !filepath.IsAbs(c.Executable) {
		return fmt.Errorf("executable path %q must be absolute", c.Executable)
	}
	if strings.Contains(filepath.ToSlash(c.Executable), "/go-build") {
		return errors.New("the binary is a temporary 'go run' build; install it with 'go install ./cmd/babysitter' first")
	}
	return nil
}

func writeFile(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create %s: %w", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	return nil
}

func removeFile(path string) error {
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("remove %s: %w", path, err)
	}
	return nil
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func parseInt(s string) int {
	n, err := strconv.Atoi(strings.TrimSpace(s))
	if err != nil {
		return 0
	}
	return n
}
