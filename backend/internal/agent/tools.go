package agent

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
)

var authorDecisions = [][]string{
	{"watch", "mode"},
	{"watch", "approve"},
	{"watch", "reject"},
	{"watch", "retry"},
	{"watch", "merge"},
	{"watch", "stop"},
	{"watch", "takeover"},
	{"watch", "handback"},
	{"auth", "login"},
	{"auth", "logout"},
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
			out = append(out, name+" "+strings.Join(decision, " "))
		}
	}
	return out
}

func AuthorPatterns() []string {
	out := make([]string, 0, len(authorDecisions))
	for _, decision := range authorDecisions {
		out = append(out, "*babysitter* "+strings.Join(decision, " "))
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

func SessionEnv(l Launch) ([]string, error) {
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
	config := [][2]string{{"core.hooksPath", l.HooksDir}}
	if helper := CredentialHelper(l.Exe, l.DataDir); helper != "" {
		config = append(config, [2]string{gitHubHelperKey, helper})
	}
	env := gitConfigEnv(config)
	shimmed, err := writeGHShim(l)
	if err != nil {
		return nil, err
	}
	if shimmed {
		env = append(env, "PATH="+l.BinDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	}
	return env, nil
}

func gitConfigEnv(pairs [][2]string) []string {
	env := []string{"GIT_CONFIG_COUNT=" + strconv.Itoa(len(pairs))}
	for i, p := range pairs {
		env = append(env, fmt.Sprintf("GIT_CONFIG_KEY_%d=%s", i, p[0]), fmt.Sprintf("GIT_CONFIG_VALUE_%d=%s", i, p[1]))
	}
	return env
}
