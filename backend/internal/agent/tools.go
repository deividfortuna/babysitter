package agent

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
)

var authorDecisions = []string{"mode", "approve", "reject", "retry", "merge", "stop", "takeover", "handback"}

func AuthorCommands(l Launch) []string {
	names := []string{"babysitter"}
	if len(l.Hook) > 0 {
		names = append(names, l.Hook[0], ShellWord(l.Hook[0]))
	}
	slices.Sort(names)
	names = slices.Compact(names)
	out := make([]string, 0, len(names)*len(authorDecisions))
	for _, name := range names {
		for _, decision := range authorDecisions {
			out = append(out, name+" watch "+decision)
		}
	}
	return out
}

func AuthorPatterns() []string {
	out := make([]string, 0, len(authorDecisions))
	for _, decision := range authorDecisions {
		out = append(out, "*babysitter* watch "+decision)
	}
	return out
}

const prePushHook = `#!/bin/sh
echo "babysitter: the daemon pushes this branch. Commit your work and let the turn end; do not push." >&2
exit 1
`

const CoAuthorTrailer = "Co-authored-by: babysitter <335241182+babysitter-orchestrator@users.noreply.github.com>"

const commitMsgHook = `#!/bin/sh
exec git interpret-trailers --in-place --if-exists addIfDifferent --trailer "` + CoAuthorTrailer + `" "$1"
`

var gitHooks = map[string]string{
	"pre-push":   prePushHook,
	"commit-msg": commitMsgHook,
}

func GitEnv(l Launch) ([]string, error) {
	if l.HooksDir == "" {
		return nil, fmt.Errorf("a hooks directory is required")
	}
	if err := os.MkdirAll(l.HooksDir, 0o750); err != nil {
		return nil, fmt.Errorf("create git hooks dir: %w", err)
	}
	for name, script := range gitHooks {
		if err := os.WriteFile(filepath.Join(l.HooksDir, name), []byte(script), 0o750); err != nil {
			return nil, fmt.Errorf("write %s hook: %w", name, err)
		}
	}
	return []string{
		"GIT_CONFIG_COUNT=1",
		"GIT_CONFIG_KEY_0=core.hooksPath",
		"GIT_CONFIG_VALUE_0=" + l.HooksDir,
	}, nil
}
