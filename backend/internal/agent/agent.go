package agent

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/deividfortuna/babysitter/internal/checks"
	"github.com/deividfortuna/babysitter/internal/store"
)

type Launch struct {
	WorktreeDir  string
	Model        string
	Effort       string
	SessionID    string
	Resume       bool
	Name         string
	Hook         []string
	HooksDir     string
	BinDir       string
	Exe          string
	DataDir      string
	PluginDir    string
	ScreenReader bool
}

type Runner interface {
	NewSessionID() string
	Command(l Launch) (argv []string, env []string, err error)
	Doctor(ctx context.Context) error
	DefaultModel() string
	Signals() bool
	Prelude() string
	AuthorCommand(l Launch) ([]string, error)
}

func AuthorArgs(bin, model string, l Launch, flags ...string) ([]string, error) {
	if l.WorktreeDir == "" || l.SessionID == "" {
		return nil, fmt.Errorf("a worktree and a session id are required")
	}
	conversation := "--session-id"
	if l.Resume {
		conversation = "--resume"
	}
	args := append([]string{bin, conversation, l.SessionID}, flags...)
	if model := PickModel(model, l.Model); model != "" {
		args = append(args, "--model", model)
	}
	return args, nil
}

type Item struct {
	ID           int64
	Kind         store.ActivityKind
	Actor        string
	At           time.Time
	URL          string
	Body         string
	Path         string
	Line         int
	State        string
	ItemID       int64
	Check        string
	Conclusion   checks.Conclusion
	RunID        int64
	JobID        int64
	JobName      string
	LogsEndpoint string
	NoLog        bool
	Log          string
	LogStep      string
	Base         string
}

type PullRequest struct {
	Repo         string
	Number       int
	Title        string
	URL          string
	Author       string
	HeadRef      string
	BaseRef      string
	HeadSHA      string
	Dependabot   bool
	DaemonPushes bool
	MergesBase   bool
}

func (p PullRequest) Identity() string {
	return "PR #" + strconv.Itoa(p.Number) + " (" + Sanitize(p.HeadRef) + " -> " + Sanitize(p.BaseRef) + ")"
}

func PickModel(runner, launch string) string {
	if launch != "" {
		return launch
	}
	return runner
}

func EffortArgs(flag string, l Launch) []string {
	if l.Effort == "" {
		return nil
	}
	return []string{flag, l.Effort}
}
