package agent

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
)

var authorDecisions = []string{
	"watch mode", "watch approve", "watch reject", "watch retry", "watch merge", "watch stop",
	"watch takeover", "watch handback",
}

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
			out = append(out, name+" "+decision)
		}
	}
	return out
}

func AuthorPatterns() []string {
	out := make([]string, 0, len(authorDecisions))
	for _, decision := range authorDecisions {
		out = append(out, "*babysitter* "+decision)
	}
	return out
}

const prePushHook = `#!/bin/sh
echo "babysitter: the daemon pushes this branch. Commit your work and let the turn end; do not push." >&2
exit 1
`

func GitEnv(l Launch) ([]string, error) {
	if l.HooksDir == "" {
		return nil, fmt.Errorf("a hooks directory is required")
	}
	if err := os.MkdirAll(l.HooksDir, 0o750); err != nil {
		return nil, fmt.Errorf("create git hooks dir: %w", err)
	}
	if err := os.WriteFile(filepath.Join(l.HooksDir, "pre-push"), []byte(prePushHook), 0o750); err != nil {
		return nil, fmt.Errorf("write pre-push hook: %w", err)
	}
	return []string{
		"GIT_CONFIG_COUNT=1",
		"GIT_CONFIG_KEY_0=core.hooksPath",
		"GIT_CONFIG_VALUE_0=" + l.HooksDir,
	}, nil
}
