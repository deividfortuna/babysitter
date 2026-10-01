package copilot

import (
	"context"
	"fmt"
	"slices"

	"github.com/deividfortuna/babysitter/internal/agent"
	"github.com/deividfortuna/babysitter/internal/execx"
)

const effortFlag = "--reasoning-effort"

type Runner struct {
	Bin   string
	Model string
	Exec  execx.DirRunner
}

func New(bin, model string) *Runner {
	if bin == "" {
		bin = "copilot"
	}
	return &Runner{Bin: bin, Model: model, Exec: execx.RunIn}
}

func (c *Runner) Doctor(ctx context.Context) error {
	return agent.CheckVersion(ctx, c.Exec, c.bin())
}

func (c *Runner) bin() string {
	if c.Bin == "" {
		return "copilot"
	}
	return c.Bin
}

func (c *Runner) NewSessionID() string { return agent.NewSessionID() }

func (c *Runner) Signals() bool { return true }

func (c *Runner) Prelude() string { return agent.SystemPrompt() }

func (c *Runner) DefaultModel() string { return c.Model }

func (c *Runner) AuthorCommand(l agent.Launch) ([]string, error) {
	return agent.AuthorArgs(c.bin(), c.Model, l, agent.EffortArgs(effortFlag, l)...)
}

func (c *Runner) Command(l agent.Launch) ([]string, []string, error) {
	if l.WorktreeDir == "" || l.SessionID == "" {
		return nil, nil, fmt.Errorf("a worktree and a session id are required")
	}
	args := []string{
		c.bin(),
		"--session-id", l.SessionID,
		"--allow-all-tools",
		"--disable-builtin-mcps", "--no-auto-update",
	}
	if l.ScreenReader {
		args = append(args, "--screen-reader")
	}
	for _, deny := range append(slices.Clone(denied), authorRules(l)...) {
		args = append(args, "--deny-tool", deny)
	}
	if l.Name != "" && !l.Resume {
		args = append(args, "--name", l.Name)
	}
	if model := agent.PickModel(c.Model, l.Model); model != "" {
		args = append(args, "--model", model)
	}
	args = append(args, agent.EffortArgs(effortFlag, l)...)
	env, err := agent.GitEnv(l)
	if err != nil {
		return nil, nil, err
	}
	if err := removeWorktreeHooks(l.WorktreeDir); err != nil {
		return nil, nil, err
	}
	plugin, err := pluginArgs(l)
	if err != nil {
		return nil, nil, err
	}
	env = append(env, "COPILOT_ALLOW_ALL=true")
	return append(args, plugin...), env, nil
}

func pluginArgs(l agent.Launch) ([]string, error) {
	if len(l.Hook) == 0 {
		return nil, nil
	}
	if l.PluginDir == "" {
		return nil, fmt.Errorf("a plugin directory is required for the hooks")
	}
	if err := installPlugin(l.PluginDir, l.Hook); err != nil {
		return nil, err
	}
	return []string{"--plugin-dir", l.PluginDir}, nil
}
