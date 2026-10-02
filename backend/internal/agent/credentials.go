package agent

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"

	"github.com/deividfortuna/babysitter/internal/runfile"
)

func AppGitConfigPath(dataDir string) string {
	return filepath.Join(dataDir, "git", "app.gitconfig")
}

func CredentialHelper(exe, dataDir string) string {
	if exe == "" {
		return ""
	}
	return "!" + shellQuote(exe) + " auth git-credential --data-dir " + shellQuote(dataDir)
}

func ghShim(exe, dataDir, gh string) string {
	return "#!/bin/sh\n" +
		"token=$(" + shellQuote(exe) + " auth token --data-dir " + shellQuote(dataDir) + " 2>/dev/null) && [ -n \"$token\" ] && export GH_TOKEN=\"$token\"\n" +
		"exec " + shellQuote(gh) + " \"$@\"\n"
}

func (l Launch) wantsGHShim() bool {
	return l.Exe != "" && l.BinDir != "" && runtime.GOOS != "windows"
}

func writeGHShim(l Launch) (bool, error) {
	if !l.wantsGHShim() {
		return false, nil
	}
	gh, err := exec.LookPath("gh")
	if errors.Is(err, exec.ErrNotFound) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("find gh: %w", err)
	}
	if err := os.MkdirAll(l.BinDir, 0o750); err != nil {
		return false, fmt.Errorf("create the gh shim dir: %w", err)
	}
	path := filepath.Join(l.BinDir, "gh")
	script := []byte(ghShim(l.Exe, l.DataDir, gh))
	if holds(path, script) {
		return true, nil
	}
	if err := runfile.ReplaceFile(path, script, 0o750); err != nil {
		return false, fmt.Errorf("write the gh shim: %w", err)
	}
	return true, nil
}

func holds(path string, content []byte) bool {
	current, err := os.ReadFile(path)
	return err == nil && bytes.Equal(current, content)
}
