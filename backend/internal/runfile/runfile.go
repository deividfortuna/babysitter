package runfile

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/deividfortuna/babysitter/internal/processalive"
)

const FileName = "running.json"

const (
	OwnerApp = "app"
	OwnerCLI = "cli"
)

type Info struct {
	PID        int       `json:"pid"`
	Port       int       `json:"port"`
	StartedAt  time.Time `json:"startedAt"`
	Owner      string    `json:"owner,omitempty"`
	Supervisor string    `json:"supervisor,omitempty"`
	Version    string    `json:"version,omitempty"`
}

func Path(dataDir string) string {
	return filepath.Join(dataDir, FileName)
}

func Write(path string, info Info) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return fmt.Errorf("create run file dir: %w", err)
	}
	data, err := json.MarshalIndent(info, "", "  ")
	if err != nil {
		return fmt.Errorf("encode run file: %w", err)
	}
	data = append(data, '\n')

	tmp, err := os.CreateTemp(filepath.Dir(path), ".running-*.json")
	if err != nil {
		return fmt.Errorf("create temp run file: %w", err)
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)

	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return fmt.Errorf("write temp run file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close temp run file: %w", err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		return fmt.Errorf("replace run file: %w", err)
	}
	return nil
}

func Read(path string) (*Info, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read run file: %w", err)
	}
	var info Info
	if err := json.Unmarshal(data, &info); err != nil {
		return nil, fmt.Errorf("parse run file: %w", err)
	}
	return &info, nil
}

func Remove(path string) error {
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("remove run file: %w", err)
	}
	return nil
}

func RemoveIfOwned(path string, ownerPID int) error {
	info, err := Read(path)
	if err != nil {
		return err
	}
	if info == nil || info.PID != ownerPID {
		return nil
	}
	return Remove(path)
}

func Live(path string) (*Info, error) {
	info, err := Read(path)
	if err != nil {
		return nil, err
	}
	if info == nil || !processalive.Alive(info.PID) {
		return nil, nil
	}
	return info, nil
}
