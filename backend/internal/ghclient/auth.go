package ghclient

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"strings"
)

var ErrNoToken = errors.New("no GitHub token found: set GITHUB_TOKEN or run 'gh auth login'")

func Token(ctx context.Context) (string, error) {
	if t := strings.TrimSpace(os.Getenv("GITHUB_TOKEN")); t != "" {
		return t, nil
	}
	out, err := exec.CommandContext(ctx, "gh", "auth", "token").Output()
	if err != nil {
		return "", ErrNoToken
	}
	t := strings.TrimSpace(string(out))
	if t == "" {
		return "", ErrNoToken
	}
	return t, nil
}
