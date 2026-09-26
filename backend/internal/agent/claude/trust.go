package claude

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

func trustConfigPath() (string, error) {
	if dir := os.Getenv("CLAUDE_CONFIG_DIR"); dir != "" {
		return filepath.Join(dir, ".claude.json"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".claude.json"), nil
}

var trustMu sync.Mutex

func acceptTrust(path, dir string) error {
	trustMu.Lock()
	defer trustMu.Unlock()
	if path == "" {
		var err error
		if path, err = trustConfigPath(); err != nil {
			return fmt.Errorf("find the Claude Code configuration: %w", err)
		}
	}
	config := map[string]json.RawMessage{}
	data, err := os.ReadFile(path)
	switch {
	case err == nil:
		if err := json.Unmarshal(data, &config); err != nil {
			return fmt.Errorf("read %s: %w", path, err)
		}
	case errors.Is(err, os.ErrNotExist):
	default:
		return fmt.Errorf("read %s: %w", path, err)
	}
	projects := map[string]json.RawMessage{}
	if raw, ok := config["projects"]; ok {
		if err := json.Unmarshal(raw, &projects); err != nil {
			return fmt.Errorf("read the projects of %s: %w", path, err)
		}
	}
	project := map[string]any{}
	if raw, ok := projects[dir]; ok {
		if err := json.Unmarshal(raw, &project); err != nil {
			return fmt.Errorf("read the project %s of %s: %w", dir, path, err)
		}
	}
	if accepted, _ := project["hasTrustDialogAccepted"].(bool); accepted {
		return nil
	}
	project["hasTrustDialogAccepted"] = true
	if projects[dir], err = json.Marshal(project); err != nil {
		return err
	}
	if config["projects"], err = json.Marshal(projects); err != nil {
		return err
	}
	out, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return fmt.Errorf("create the directory of %s: %w", path, err)
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".claude.json.*")
	if err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	if _, err := tmp.Write(out); err != nil {
		tmp.Close()
		os.Remove(tmp.Name())
		return fmt.Errorf("write %s: %w", path, err)
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmp.Name())
		return fmt.Errorf("write %s: %w", path, err)
	}
	if err := os.Chmod(tmp.Name(), 0o600); err != nil {
		os.Remove(tmp.Name())
		return fmt.Errorf("write %s: %w", path, err)
	}
	if err := os.Rename(tmp.Name(), path); err != nil {
		os.Remove(tmp.Name())
		return fmt.Errorf("write %s: %w", path, err)
	}
	return nil
}
