package copilot

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/deividfortuna/babysitter/internal/agent"
)

const (
	hooksFile    = ".github/hooks/babysitter.json"
	hooksVersion = 1
	hookTimeout  = 10
)

var hooks = []struct {
	key, event string
}{
	{"sessionStart", agent.EventSessionStart},
	{"userPromptSubmitted", agent.EventUserPromptSubmit},
	{"preToolUse", agent.EventPreToolUse},
	{"postToolUse", agent.EventPostToolUse},
	{"agentStop", agent.EventStop},
	{"sessionEnd", agent.EventSessionEnd},
}

type hookEntry struct {
	Type       string `json:"type"`
	Bash       string `json:"bash"`
	Powershell string `json:"powershell"`
	TimeoutSec int    `json:"timeoutSec"`
}

type hookFile struct {
	Version int                    `json:"version"`
	Hooks   map[string][]hookEntry `json:"hooks"`
}

func hooksJSON(hook []string) ([]byte, error) {
	file := hookFile{Version: hooksVersion, Hooks: map[string][]hookEntry{}}
	for _, h := range hooks {
		argv := append(append([]string(nil), hook...), h.event)
		file.Hooks[h.key] = []hookEntry{{
			Type:       "command",
			Bash:       agent.ShellJoin(argv),
			Powershell: agent.PowerShellJoin(argv),
			TimeoutSec: hookTimeout,
		}}
	}
	data, err := json.MarshalIndent(file, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(data, '\n'), nil
}

func installHooks(worktree string, hook []string) error {
	path := filepath.Join(worktree, filepath.FromSlash(hooksFile))
	if len(hook) == 0 {
		return removeFile(path)
	}
	data, err := hooksJSON(hook)
	if err != nil {
		return err
	}
	root, err := os.OpenRoot(worktree)
	if err != nil {
		return fmt.Errorf("open the worktree: %w", err)
	}
	defer root.Close()
	if err := root.MkdirAll(filepath.Dir(hooksFile), 0o750); err != nil {
		return fmt.Errorf("create the copilot hooks dir: %w", err)
	}
	if err := writeIn(root, hooksFile, data); err != nil {
		return fmt.Errorf("write the copilot hooks: %w", err)
	}
	return excludeFromGit(worktree, "/"+hooksFile)
}

func writeIn(root *os.Root, name string, data []byte) error {
	f, err := root.Create(name)
	if err != nil {
		return err
	}
	if _, err := f.Write(data); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}

func removeFile(path string) error {
	err := os.Remove(path)
	if err == nil || errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}

func excludeFromGit(worktree, pattern string) error {
	gitDir, err := gitCommonDir(worktree)
	if err != nil || gitDir == "" {
		return err
	}
	path := filepath.Join(gitDir, "info", "exclude")
	data, err := os.ReadFile(path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("read %s: %w", path, err)
	}
	body := string(data)
	if slices.Contains(strings.Split(body, "\n"), pattern) {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return fmt.Errorf("create %s: %w", filepath.Dir(path), err)
	}
	if body != "" && !strings.HasSuffix(body, "\n") {
		body += "\n"
	}
	body += pattern + "\n"
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	return nil
}

func gitCommonDir(worktree string) (string, error) {
	gitDir, err := gitDirOf(worktree)
	if err != nil || gitDir == "" {
		return "", err
	}
	return pointedDir(gitDir, filepath.Join(gitDir, "commondir"), "")
}

func gitDirOf(worktree string) (string, error) {
	path := filepath.Join(worktree, ".git")
	info, err := os.Stat(path)
	if errors.Is(err, os.ErrNotExist) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	if info.IsDir() {
		return path, nil
	}
	return pointedDir(worktree, path, "gitdir:")
}

func pointedDir(base, file, prefix string) (string, error) {
	data, err := os.ReadFile(file)
	if errors.Is(err, os.ErrNotExist) {
		return base, nil
	}
	if err != nil {
		return "", err
	}
	text := strings.TrimSpace(string(data))
	if !strings.HasPrefix(text, prefix) {
		return "", fmt.Errorf("%s: unexpected content %q", file, text)
	}
	dir := strings.TrimSpace(strings.TrimPrefix(text, prefix))
	if dir == "" {
		return base, nil
	}
	if filepath.IsAbs(dir) {
		return filepath.Clean(dir), nil
	}
	return filepath.Clean(filepath.Join(base, dir)), nil
}
