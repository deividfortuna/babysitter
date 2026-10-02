package agent

import (
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"testing"
)

func TestSessionEnvIncludesTheGitConfigOfTheApp(t *testing.T) {
	t.Parallel()
	l := Launch{HooksDir: filepath.Join(t.TempDir(), "hooks"), Exe: "/opt/babysitter", DataDir: "/data"}

	env, err := SessionEnv(l)
	if err != nil {
		t.Fatal(err)
	}

	config := gitConfig(env)
	if got, want := config["include.path"], filepath.Join("/data", "git", "app.gitconfig"); got != want {
		t.Fatalf("include.path = %q, want %q", got, want)
	}
	if helper, ok := config["credential.https://github.com.helper"]; ok {
		t.Fatalf("credential helper = %q, want it in the included file only", helper)
	}
}

func gitConfig(env []string) map[string]string {
	vars := map[string]string{}
	for _, e := range env {
		k, v, _ := strings.Cut(e, "=")
		vars[k] = v
	}
	config := map[string]string{}
	for i := 0; vars["GIT_CONFIG_KEY_"+strconv.Itoa(i)] != ""; i++ {
		config[vars["GIT_CONFIG_KEY_"+strconv.Itoa(i)]] = vars["GIT_CONFIG_VALUE_"+strconv.Itoa(i)]
	}
	return config
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
		{"the app is not in use", "exit 0", "gh api /user as nobody\n"},
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

func TestTheGHShimStopsWhenTheAppTokenFails(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the shim is a shell script")
	}
	tools := t.TempDir()
	script(t, tools, "gh", "#!/bin/sh\necho \"gh $* as ${GH_TOKEN:-nobody}\"\n")
	t.Setenv("PATH", tools+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("GH_TOKEN", "ghp_personal")
	babysitter := script(t, t.TempDir(), "babysitter", "#!/bin/sh\nexit 1\n")
	l := Launch{HooksDir: filepath.Join(t.TempDir(), "hooks"), BinDir: filepath.Join(t.TempDir(), "bin"), Exe: babysitter, DataDir: "/data"}
	env, err := SessionEnv(l)
	if err != nil {
		t.Fatal(err)
	}

	cmd := exec.Command("sh", "-c", "gh api /user")
	cmd.Env = append(os.Environ(), env...)
	out, err := cmd.Output()
	if err == nil {
		t.Fatalf("gh ran with output %q, want the shim to stop before it", out)
	}
	if len(out) != 0 {
		t.Fatalf("output = %q, want none", out)
	}
}

func TestANewGHShimLeavesTheOldOneWhole(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the shim is a shell script")
	}
	first, second := t.TempDir(), t.TempDir()
	script(t, first, "gh", "#!/bin/sh\n")
	script(t, second, "gh", "#!/bin/sh\n")
	l := Launch{HooksDir: filepath.Join(t.TempDir(), "hooks"), BinDir: filepath.Join(t.TempDir(), "bin"), Exe: "/opt/babysitter", DataDir: "/data"}
	t.Setenv("PATH", first)
	if _, err := SessionEnv(l); err != nil {
		t.Fatal(err)
	}
	running, err := os.Open(filepath.Join(l.BinDir, "gh"))
	if err != nil {
		t.Fatal(err)
	}
	defer running.Close()

	t.Setenv("PATH", second)
	if _, err := SessionEnv(l); err != nil {
		t.Fatal(err)
	}

	old, err := io.ReadAll(running)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(old), filepath.Join(first, "gh")) {
		t.Fatalf("the shim a session has open = %q, want it whole, with the first gh", old)
	}
	current, err := os.ReadFile(filepath.Join(l.BinDir, "gh"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(current), filepath.Join(second, "gh")) {
		t.Fatalf("the shim = %q, want the second gh", current)
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

func script(t *testing.T, dir, name, body string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(body), 0o750); err != nil {
		t.Fatal(err)
	}
	return path
}
