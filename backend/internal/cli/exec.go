package cli

import (
	"cmp"
	"os"
	"runtime"
	"slices"
	"strings"
)

func envIn(dir string) []string {
	env := slices.DeleteFunc(os.Environ(), func(e string) bool { return strings.HasPrefix(e, "PWD=") })
	return append(env, "PWD="+dir)
}

func userShell() string {
	return shellOf(runtime.GOOS, os.Getenv)
}

func shellOf(goos string, getenv func(string) string) string {
	if shell := getenv("SHELL"); shell != "" {
		return shell
	}
	if goos == "windows" {
		return cmp.Or(getenv("COMSPEC"), "cmd.exe")
	}
	return "/bin/sh"
}
