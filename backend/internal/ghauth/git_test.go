package ghauth

import (
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
)

func gitURL(t *testing.T, globalConfig string, env []string, url string) string {
	t.Helper()
	global := filepath.Join(t.TempDir(), "gitconfig")
	if err := os.WriteFile(global, []byte(globalConfig), 0o600); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("git", "ls-remote", "--get-url", url)
	cmd.Env = append(os.Environ(), append(env, "GIT_CONFIG_GLOBAL="+global, "GIT_CONFIG_NOSYSTEM=1")...)
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("git ls-remote --get-url %s: %v", url, err)
	}
	return strings.TrimSpace(string(out))
}

func TestGitEnvKeepsGitHubOnHTTPS(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not installed")
	}
	toSSH := "[url \"git@github.com:\"]\n\tinsteadOf = https://github.com/\n"
	env := gitEnv("ghu_abc")

	for _, tc := range []struct {
		name, global, url, want string
	}{
		{"https under a rule of the user to SSH", toSSH, "https://github.com/octo/hello.git", "https://github.com/octo/hello.git"},
		{"an owner that starts with a digit", toSSH, "https://github.com/9lives/hello.git", "https://github.com/9lives/hello.git"},
		{"an owner in upper case", toSSH, "https://github.com/Octo/hello.git", "https://github.com/Octo/hello.git"},
		{"scp-like SSH", "", "git@github.com:octo/hello.git", "https://github.com/octo/hello.git"},
		{"SSH URL", "", "ssh://git@github.com/octo/hello.git", "https://github.com/octo/hello.git"},
		{"another host", toSSH, "https://gitlab.com/octo/hello.git", "https://gitlab.com/octo/hello.git"},
		{"a rule of the user for one owner", "[url \"git@github.com:octo/\"]\n\tinsteadOf = https://github.com/octo/\n", "https://github.com/octo/hello.git", "git@github.com:octo/hello.git"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := gitURL(t, tc.global, env, tc.url); got != tc.want {
				t.Fatalf("git uses %s, want %s", got, tc.want)
			}
		})
	}
}

func TestWriteGitConfigOnlyWhileTheAppIsInUse(t *testing.T) {
	h := newHarness(t)
	a := h.auth()
	path := filepath.Join(h.dir, "git", "app.gitconfig")
	read := func() string {
		t.Helper()
		if err := a.WriteGitConfig(path, "!babysitter auth git-credential"); err != nil {
			t.Fatal(err)
		}
		b, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		return string(b)
	}

	if got := read(); got != "" {
		t.Fatalf("signed out: config = %q, want it empty", got)
	}
	h.signIn(t, a)
	if got := read(); got != appGitConfig("!babysitter auth git-credential") {
		t.Fatalf("signed in: config = %q, want the rules of the app", got)
	}
	t.Setenv("GITHUB_TOKEN", "ghp_env")
	if got := read(); got != "" {
		t.Fatalf("GITHUB_TOKEN first: config = %q, want it empty", got)
	}
}

func TestWriteGitConfigKeepsTheAppWhenTheSignInCannotBeRead(t *testing.T) {
	h := newHarness(t)
	if err := os.WriteFile(filepath.Join(h.dir, signInFileName), []byte("{not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(h.dir, "git", "app.gitconfig")

	if err := h.auth().WriteGitConfig(path, "!babysitter auth git-credential"); err != nil {
		t.Fatal(err)
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != appGitConfig("!babysitter auth git-credential") {
		t.Fatalf("config = %q, want the rules of the app, so git asks babysitter and stops", got)
	}
}

func TestTheGitConfigKeepsGitHubOnHTTPSThroughAnInclude(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not installed")
	}
	include := filepath.Join(t.TempDir(), "app.gitconfig")
	if err := os.WriteFile(include, []byte(appGitConfig("")), 0o600); err != nil {
		t.Fatal(err)
	}
	env := []string{"GIT_CONFIG_COUNT=1", "GIT_CONFIG_KEY_0=include.path", "GIT_CONFIG_VALUE_0=" + include}
	toSSH := "[url \"git@github.com:\"]\n\tinsteadOf = https://github.com/\n"

	if got := gitURL(t, toSSH, env, "https://github.com/octo/hello.git"); got != "https://github.com/octo/hello.git" {
		t.Fatalf("git uses %s, want https", got)
	}
	if got := gitURL(t, "", env, "git@github.com:octo/hello.git"); got != "https://github.com/octo/hello.git" {
		t.Fatalf("git uses %s, want https", got)
	}
}

func TestTheGitConfigAsksTheAppBeforeTheHelpersOfTheUser(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the fake helpers are shell scripts")
	}
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not installed")
	}
	dir := filepath.Join(t.TempDir(), "my tools")
	personal := helperScript(t, dir, "personal", "gho_personal")
	app := helperScript(t, dir, "babysitter", "ghu_app")
	include := filepath.Join(t.TempDir(), "app.gitconfig")
	if err := os.WriteFile(include, []byte(appGitConfig("!'"+app+"'")), 0o600); err != nil {
		t.Fatal(err)
	}
	global := filepath.Join(t.TempDir(), "gitconfig")
	userHelpers := "[credential \"https://github.com\"]\n\thelper =\n\thelper = !'" + personal + "'\n"
	if err := os.WriteFile(global, []byte(userHelpers), 0o600); err != nil {
		t.Fatal(err)
	}

	cmd := exec.Command("git", "credential", "fill")
	cmd.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL="+global, "GIT_CONFIG_NOSYSTEM=1", "GIT_TERMINAL_PROMPT=0",
		"GIT_CONFIG_COUNT=1", "GIT_CONFIG_KEY_0=include.path", "GIT_CONFIG_VALUE_0="+include)
	cmd.Stdin = strings.NewReader("protocol=https\nhost=github.com\n\n")
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("git credential fill: %v", err)
	}

	if !strings.Contains(string(out), "password=ghu_app\n") {
		t.Fatalf("git credential fill = %q, want the token of the app before the helper of the user", out)
	}
}

func helperScript(t *testing.T, dir, name, password string) string {
	t.Helper()
	if err := os.MkdirAll(dir, 0o750); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, name)
	body := "#!/bin/sh\n[ \"$1\" = get ] || exit 0\nprintf 'username=x-access-token\\npassword=" + password + "\\n'\n"
	if err := os.WriteFile(path, []byte(body), 0o750); err != nil {
		t.Fatal(err)
	}
	return path
}

func gitPushURL(t *testing.T, globalConfig string, env []string, origin string) string {
	t.Helper()
	dir := t.TempDir()
	global := filepath.Join(dir, "gitconfig")
	if err := os.WriteFile(global, []byte(globalConfig), 0o600); err != nil {
		t.Fatal(err)
	}
	repo := filepath.Join(dir, "repo")
	if err := os.MkdirAll(repo, 0o750); err != nil {
		t.Fatal(err)
	}
	run := func(args ...string) string {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-C", repo}, args...)...)
		cmd.Env = append(os.Environ(), append(env, "GIT_CONFIG_GLOBAL="+global, "GIT_CONFIG_NOSYSTEM=1")...)
		out, err := cmd.Output()
		if err != nil {
			t.Fatalf("git %v: %v", args, err)
		}
		return strings.TrimSpace(string(out))
	}
	run("init", "-q")
	run("remote", "add", "origin", origin)
	return run("remote", "get-url", "--push", "origin")
}

func TestGitEnvKeepsGitHubPushesOnHTTPS(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not installed")
	}
	pushToSSH := "[url \"git@github.com:\"]\n\tpushInsteadOf = https://github.com/\n"
	env := gitEnv("ghu_abc")

	for _, tc := range []struct {
		name, global, origin, want string
	}{
		{"https under a push rule of the user to SSH", pushToSSH, "https://github.com/octo/hello.git", "https://github.com/octo/hello.git"},
		{"scp-like SSH", "", "git@github.com:octo/hello.git", "https://github.com/octo/hello.git"},
		{"a push rule of the user for one owner", "[url \"git@github.com:octo/\"]\n\tpushInsteadOf = https://github.com/octo/\n", "https://github.com/octo/hello.git", "git@github.com:octo/hello.git"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := gitPushURL(t, tc.global, env, tc.origin); got != tc.want {
				t.Fatalf("git pushes to %s, want %s", got, tc.want)
			}
		})
	}
}

func TestTheGitConfigKeepsGitHubPushesOnHTTPSThroughAnInclude(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not installed")
	}
	include := filepath.Join(t.TempDir(), "app.gitconfig")
	if err := os.WriteFile(include, []byte(appGitConfig("")), 0o600); err != nil {
		t.Fatal(err)
	}
	env := []string{"GIT_CONFIG_COUNT=1", "GIT_CONFIG_KEY_0=include.path", "GIT_CONFIG_VALUE_0=" + include}
	pushToSSH := "[url \"git@github.com:\"]\n\tpushInsteadOf = https://github.com/\n"

	if got := gitPushURL(t, pushToSSH, env, "https://github.com/octo/hello.git"); got != "https://github.com/octo/hello.git" {
		t.Fatalf("git pushes to %s, want https", got)
	}
}

func authorizationsGitSends(t *testing.T, env func(toServer *strings.Replacer) []string) []string {
	t.Helper()
	var (
		mu   sync.Mutex
		sent []string
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		sent = append(sent, r.Header.Values("Authorization")...)
		w.WriteHeader(http.StatusNotFound)
	}))
	t.Cleanup(srv.Close)
	toServer := strings.NewReplacer(gitHubHTTPS, srv.URL+"/")
	global := filepath.Join(t.TempDir(), "gitconfig")
	personal := "[http \"" + srv.URL + "/\"]\n\textraHeader = AUTHORIZATION: bearer ghp_personal\n"
	if err := os.WriteFile(global, []byte(personal), 0o600); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("git", "ls-remote", srv.URL+"/octo/hello.git")
	cmd.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL="+global, "GIT_CONFIG_NOSYSTEM=1", "GIT_TERMINAL_PROMPT=0")
	cmd.Env = append(cmd.Env, env(toServer)...)
	_ = cmd.Run()
	mu.Lock()
	defer mu.Unlock()
	return sent
}

func TestGitEnvSendsOnlyTheHeaderOfTheApp(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not installed")
	}
	sent := authorizationsGitSends(t, func(toServer *strings.Replacer) []string {
		var env []string
		for _, e := range gitEnv("ghu_abc") {
			env = append(env, toServer.Replace(e))
		}
		return env
	})

	if len(sent) == 0 || strings.Contains(strings.Join(sent, "\n"), "ghp_personal") {
		t.Fatalf("git sent %q, want only the header of the app", sent)
	}
}

func TestTheGitConfigDropsTheGitHubHeadersOfTheUser(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not installed")
	}
	sent := authorizationsGitSends(t, func(toServer *strings.Replacer) []string {
		include := filepath.Join(t.TempDir(), "app.gitconfig")
		if err := os.WriteFile(include, []byte(toServer.Replace(appGitConfig(""))), 0o600); err != nil {
			t.Fatal(err)
		}
		return []string{"GIT_CONFIG_COUNT=1", "GIT_CONFIG_KEY_0=include.path", "GIT_CONFIG_VALUE_0=" + include}
	})

	if len(sent) != 0 {
		t.Fatalf("git sent %q, want no header of the user", sent)
	}
}

func TestGitEnvLeavesTheHelpersOfTheUserAlone(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the fake helper is a shell script")
	}
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not installed")
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("WWW-Authenticate", `Basic realm="GitHub"`)
		w.WriteHeader(http.StatusUnauthorized)
	}))
	t.Cleanup(srv.Close)
	dir := t.TempDir()
	calls := filepath.Join(dir, "calls")
	helper := filepath.Join(dir, "personal")
	body := "#!/bin/sh\necho \"$1\" >> '" + calls + "'\n[ \"$1\" = get ] && printf 'username=me\\npassword=ghp_personal\\n'\nexit 0\n"
	if err := os.WriteFile(helper, []byte(body), 0o750); err != nil {
		t.Fatal(err)
	}
	global := filepath.Join(dir, "gitconfig")
	if err := os.WriteFile(global, []byte("[credential]\n\thelper = !'"+helper+"'\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	toServer := strings.NewReplacer(gitHubHTTPS, srv.URL+"/")
	cmd := exec.Command("git", "ls-remote", srv.URL+"/octo/hello.git")
	cmd.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL="+global, "GIT_CONFIG_NOSYSTEM=1", "GIT_TERMINAL_PROMPT=0")
	for _, e := range gitEnv("ghu_revoked") {
		cmd.Env = append(cmd.Env, toServer.Replace(e))
	}
	_ = cmd.Run()

	if got, err := os.ReadFile(calls); err == nil {
		t.Fatalf("git asked the helper of the user: %q", got)
	}
}
