package copilot

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/deividfortuna/babysitter/internal/agent"
)

const (
	pluginName        = "babysitter-watch"
	pluginDescription = "Reports the session of a babysitter watch to the daemon and refuses the decisions of the author"
	worktreeHooksFile = ".github/hooks/babysitter.json"
	hooksVersion      = 1
	hookTimeout       = 10
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

type pluginManifest struct {
	Name        string `json:"name"`
	Description string `json:"description"`
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
	return indentedJSON(file)
}

func indentedJSON(v any) ([]byte, error) {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(data, '\n'), nil
}

func installPlugin(dir string, hook []string) error {
	hooksData, err := hooksJSON(hook)
	if err != nil {
		return err
	}
	manifest, err := indentedJSON(pluginManifest{Name: pluginName, Description: pluginDescription})
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return fmt.Errorf("create the copilot plugin dir: %w", err)
	}
	for name, data := range map[string][]byte{"plugin.json": manifest, "hooks.json": hooksData} {
		if err := os.WriteFile(filepath.Join(dir, name), data, 0o600); err != nil {
			return fmt.Errorf("write the copilot plugin %s: %w", name, err)
		}
	}
	return nil
}

func removeWorktreeHooks(worktree string) error {
	root, err := os.OpenRoot(worktree)
	if err != nil {
		return fmt.Errorf("open the worktree: %w", err)
	}
	defer root.Close()
	err = root.Remove(worktreeHooksFile)
	if err == nil || errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return fmt.Errorf("remove the copilot hooks from the worktree: %w", err)
}
