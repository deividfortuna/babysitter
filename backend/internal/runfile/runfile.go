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

// Windows refuses to open a file while another process replaces it, and
// refuses to replace a file another process has open. Both last
// microseconds, so a reader and the writer try again for a moment.
const (
	busyTries = 20
	busyPause = 10 * time.Millisecond
)

// startSlack lets a StartedAt cut to the second, or a clock that steps
// back a little, still name the process that wrote the file.
const startSlack = time.Second

type Info struct {
	PID        int       `json:"pid"`
	Port       int       `json:"port"`
	StartedAt  time.Time `json:"startedAt"`
	Owner      string    `json:"owner,omitempty"`
	Supervisor string    `json:"supervisor,omitempty"`
	Version    string    `json:"version,omitempty"`
	RemotePort int       `json:"remotePort,omitempty"`
	RemoteHost string    `json:"remoteHost,omitempty"`
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
	if err := WhileBusy(func() error { return os.Rename(tmpName, path) }); err != nil {
		return fmt.Errorf("replace run file: %w", err)
	}
	return nil
}

func Read(path string) (*Info, error) {
	var data []byte
	err := WhileBusy(func() (err error) {
		data, err = os.ReadFile(path)
		return err
	})
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

// WhileBusy runs op again while it fails because another process has the
// file, up to busyTries times, and returns the last error.
func WhileBusy(op func() error) error {
	for try := 1; ; try++ {
		err := op()
		if err == nil || !busy(err) || try == busyTries {
			return err
		}
		time.Sleep(busyPause)
	}
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
	if info == nil || !info.running() {
		return nil, nil
	}
	return info, nil
}

// running reports whether the process that wrote the file still runs. A
// process created after the file was written only took over a freed pid.
func (i *Info) running() bool {
	if !processalive.Alive(i.PID) {
		return false
	}
	created, known := processalive.Created(i.PID)
	bothKnown := known && !i.StartedAt.IsZero()
	return !bothKnown || !created.After(i.StartedAt.Add(startSlack))
}
