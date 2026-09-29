package claude

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/deividfortuna/babysitter/internal/agent"
	"github.com/deividfortuna/babysitter/internal/execx"
)

const effortFlag = "--effort"

type Runner struct {
	Bin        string
	Model      string
	Exec       execx.DirRunner
	ConfigPath string
}

func New(bin, model string) *Runner {
	if bin == "" {
		bin = "claude"
	}
	return &Runner{Bin: bin, Model: model, Exec: execx.RunIn}
}

func (c *Runner) Doctor(ctx context.Context) error {
	return agent.CheckVersion(ctx, c.Exec, c.bin())
}

func (c *Runner) bin() string {
	if c.Bin == "" {
		return "claude"
	}
	return c.Bin
}

func (c *Runner) NewSessionID() string { return agent.NewSessionID() }

func (c *Runner) Signals() bool { return true }

func (c *Runner) Prelude() string { return "" }

func (c *Runner) DefaultModel() string { return c.Model }

func (c *Runner) AuthorCommand(l agent.Launch) ([]string, error) {
	flags := append([]string{"--permission-mode", "manual"}, agent.EffortArgs(effortFlag, l)...)
	args, err := agent.AuthorArgs(c.bin(), c.Model, l, flags...)
	if err != nil {
		return nil, err
	}
	if err := acceptTrust(c.ConfigPath, l.WorktreeDir); err != nil {
		return nil, err
	}
	return args, nil
}

func (c *Runner) Command(l agent.Launch) ([]string, []string, error) {
	if l.WorktreeDir == "" || l.SessionID == "" {
		return nil, nil, fmt.Errorf("a worktree and a session id are required")
	}
	if err := acceptTrust(c.ConfigPath, l.WorktreeDir); err != nil {
		return nil, nil, err
	}
	args := []string{c.bin()}
	if l.Resume {
		args = append(args, "--resume", l.SessionID)
	} else {
		args = append(args, "--session-id", l.SessionID)
	}
	args = append(args,
		"--permission-mode", "dontAsk",
		"--setting-sources", "user",
		"--strict-mcp-config",
		"--ax-screen-reader",
		"--allowedTools", strings.Join(allowedTools, ","),
		"--disallowedTools", strings.Join(append(slices.Clone(deniedTools), authorRules(l)...), ","),
		"--append-system-prompt", agent.SystemPrompt(),
	)
	if l.Name != "" {
		args = append(args, "--name", l.Name)
	}
	if len(l.Hook) > 0 {
		args = append(args, "--settings", hookSettings(l.Hook))
	}
	if model := agent.PickModel(c.Model, l.Model); model != "" {
		args = append(args, "--model", model)
	}
	args = append(args, agent.EffortArgs(effortFlag, l)...)
	env, err := agent.GitEnv(l)
	if err != nil {
		return nil, nil, err
	}
	env = append(env, "CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC=1")
	return args, env, nil
}
