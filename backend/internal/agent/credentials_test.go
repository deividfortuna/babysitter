package agent

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
)

func TestSessionEnvAsksBabysitterForGitCredentials(t *testing.T) {
	t.Parallel()
	l := Launch{HooksDir: filepath.Join(t.TempDir(), "hooks"), Exe: "/opt/my tools/babysitter", DataDir: "/data"}

	env, err := SessionEnv(l)
	if err != nil {
		t.Fatal(err)
	}

	want := []string{
		"GIT_CONFIG_COUNT=2",
		"GIT_CONFIG_KEY_1=credential.https://github.com.helper",
		"GIT_CONFIG_VALUE_1=!'/opt/my tools/babysitter' auth git-credential --data-dir '/data'",
	}
	for _, w := range want {
		if !slices.Contains(env, w) {
			t.Fatalf("env = %q, lacks %q", env, w)
		}
	}
}

func TestGitRunsTheBabysitterHelper(t *testing.T) {
	t.Parallel()
	if runtime.GOOS == "windows" {
		t.Skip("the fake babysitter is a shell script")
	}
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not installed")
	}
	dir := filepath.Join(t.TempDir(), "my tools")
	if err := os.MkdirAll(dir, 0o750); err != nil {
		t.Fatal(err)
	}
	babysitter := script(t, dir, "babysitter", "#!/bin/sh\n"+
		"[ \"$*\" = \"auth git-credential --data-dir /data dir get\" ] || exit 1\n"+
		"printf 'username=x-access-token\\npassword=ghu_app\\n'\n")
	env, err := SessionEnv(Launch{HooksDir: filepath.Join(t.TempDir(), "hooks"), Exe: babysitter, DataDir: "/data dir"})
	if err != nil {
		t.Fatal(err)
	}

	cmd := exec.Command("git", "credential", "fill")
	cmd.Env = append(os.Environ(), append(env, "GIT_TERMINAL_PROMPT=0", "GIT_CONFIG_GLOBAL="+os.DevNull, "GIT_CONFIG_NOSYSTEM=1")...)
	cmd.Stdin = strings.NewReader("protocol=https\nhost=github.com\n\n")
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("git credential fill: %v %s", err, stderr(err))
	}
	if !strings.Contains(string(out), "password=ghu_app\n") {
		t.Fatalf("git credential fill = %q, want the app token", out)
	}
}

func TestSessionEnvWithoutTheBabysitterCommandHasNoHelper(t *testing.T) {
	t.Parallel()

	env, err := SessionEnv(Launch{HooksDir: filepath.Join(t.TempDir(), "hooks"), DataDir: "/data"})
	if err != nil {
		t.Fatal(err)
	}

	if !slices.Contains(env, "GIT_CONFIG_COUNT=1") {
		t.Fatalf("env = %q, want the hooks path only", env)
	}
}

func TestTheGHShimGivesGHTheAppToken(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the shim is a shell script")
	}
	tools := t.TempDir()
	script(t, tools, "gh", "#!/bin/sh\necho \"gh $* as ${GH_TOKEN:-nobody}\"\n")
	t.Setenv("PATH", tools+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("GH_TOKEN", "")

	for _, tc := range []struct {
		name  string
		token string
		want  string
	}{
		{"the app is in use", "echo ghu_app", "gh api /user as ghu_app\n"},
		{"the app is not in use", "exit 1", "gh api /user as nobody\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			babysitter := script(t, t.TempDir(), "babysitter", "#!/bin/sh\n"+tc.token+"\n")
			l := Launch{HooksDir: filepath.Join(t.TempDir(), "hooks"), BinDir: filepath.Join(t.TempDir(), "bin"), Exe: babysitter, DataDir: "/data"}
			env, err := SessionEnv(l)
			if err != nil {
				t.Fatal(err)
			}

			cmd := exec.Command("sh", "-c", "gh api /user")
			cmd.Env = append(os.Environ(), env...)
			out, err := cmd.Output()
			if err != nil {
				t.Fatal(err)
			}
			if string(out) != tc.want {
				t.Fatalf("output = %q, want %q", out, tc.want)
			}
		})
	}
}

func TestNoGHShimWithoutGH(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	l := Launch{HooksDir: filepath.Join(t.TempDir(), "hooks"), BinDir: filepath.Join(t.TempDir(), "bin"), Exe: "/opt/babysitter", DataDir: "/data"}

	env, err := SessionEnv(l)
	if err != nil {
		t.Fatal(err)
	}

	if slices.ContainsFunc(env, func(e string) bool { return strings.HasPrefix(e, "PATH=") }) {
		t.Fatalf("env = %q, want the PATH of the daemon when gh is not installed", env)
	}
}

func stderr(err error) string {
	if exitErr, ok := errors.AsType[*exec.ExitError](err); ok {
		return string(exitErr.Stderr)
	}
	return ""
}

func script(t *testing.T, dir, name, body string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(body), 0o750); err != nil {
		t.Fatal(err)
	}
	return path
}
