//go:build !windows

package service

import (
	"bytes"
	"context"
	"encoding/xml"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/deividfortuna/babysitter/internal/execx"
)

func TestRenderPlist(t *testing.T) {
	t.Parallel()
	data, err := renderPlist(Config{
		Executable: "/usr/local/bin/babysitter",
		Args:       []string{"serve", "--interval", "60s", "--db", "/tmp/a & b.db"},
		LogDir:     "/Users/x/Library/Logs/babysitter",
	}, "/Users/x")
	if err != nil {
		t.Fatalf("renderPlist() error = %v", err)
	}
	s := string(data)
	for _, want := range []string{
		"<string>" + Label + "</string>",
		"<string>/usr/local/bin/babysitter</string>\n        <string>serve</string>\n        <string>--interval</string>\n        <string>60s</string>",
		"<string>/tmp/a &amp; b.db</string>",
		"<string>/opt/homebrew/bin:/opt/homebrew/sbin:/usr/local/bin:/Users/x/.local/bin:/usr/bin:/bin:/usr/sbin:/sbin</string>",
		"<string>/Users/x/Library/Logs/babysitter/babysitter.log</string>",
		"<key>RunAtLoad</key>\n    <true/>",
	} {
		if !strings.Contains(s, want) {
			t.Errorf("plist is missing %q:\n%s", want, s)
		}
	}
}

func TestRenderPlistEscapesHomeInPath(t *testing.T) {
	t.Parallel()
	data, err := renderPlist(Config{
		Executable: "/usr/local/bin/babysitter",
		LogDir:     "/tmp/logs",
	}, "/Users/a&b")
	if err != nil {
		t.Fatalf("renderPlist() error = %v", err)
	}
	want := "<string>/opt/homebrew/bin:/opt/homebrew/sbin:/usr/local/bin:/Users/a&amp;b/.local/bin:/usr/bin:/bin:/usr/sbin:/sbin</string>"
	if !strings.Contains(string(data), want) {
		t.Errorf("plist is missing %q:\n%s", want, data)
	}
	decoder := xml.NewDecoder(bytes.NewReader(data))
	for {
		_, err := decoder.Token()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			t.Fatalf("plist is not valid XML: %v\n%s", err, data)
		}
	}
}

func TestRenderRejectsGoRunBinary(t *testing.T) {
	t.Parallel()
	cfg := Config{Executable: "/var/folders/xx/T/go-build123/b001/exe/babysitter", LogDir: "/tmp"}
	if _, err := renderPlist(cfg, "/Users/x"); err == nil || !strings.Contains(err.Error(), "go install") {
		t.Fatalf("renderPlist() error = %v, want go run rejection", err)
	}
	if _, err := renderUnit(cfg); err == nil {
		t.Fatal("renderUnit() expected error")
	}
	if _, err := renderUnit(Config{Executable: "relative/babysitter"}); err == nil {
		t.Fatal("renderUnit() expected error for relative path")
	}
}

func TestRenderUnit(t *testing.T) {
	t.Parallel()
	data, err := renderUnit(Config{
		Executable: "/home/x/go/bin/babysitter",
		Args:       []string{"serve", "--interval", "60s", "--db", `/home/x/my "db".db`},
	})
	if err != nil {
		t.Fatalf("renderUnit() error = %v", err)
	}
	s := string(data)
	want := `ExecStart=/home/x/go/bin/babysitter serve --interval 60s --db "/home/x/my \"db\".db"`
	if !strings.Contains(s, want) {
		t.Fatalf("unit is missing %q:\n%s", want, s)
	}
	if !strings.Contains(s, "WantedBy=default.target") {
		t.Fatalf("unit is missing install section:\n%s", s)
	}
}

type fakeRunner struct {
	calls   []string
	replies map[string]struct {
		out  string
		code int
	}
}

func (f *fakeRunner) run(ctx context.Context, name string, args ...string) (string, error) {
	line := name + " " + strings.Join(args, " ")
	f.calls = append(f.calls, line)
	r, ok := f.replies[line]
	if !ok {
		return "", nil
	}
	if r.code != 0 {
		return r.out, &execx.ExitError{Name: name, Args: args, Code: r.code, Output: r.out}
	}
	return r.out, nil
}

func (f *fakeRunner) reply(line, out string, code int) {
	if f.replies == nil {
		f.replies = map[string]struct {
			out  string
			code int
		}{}
	}
	f.replies[line] = struct {
		out  string
		code int
	}{out, code}
}

func assertCalls(t *testing.T, got []string, want ...string) {
	t.Helper()
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("calls:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}

func TestLaunchdLifecycle(t *testing.T) {
	home := t.TempDir()
	fr := &fakeRunner{}
	l := &launchd{home: home, uid: 501, run: fr.run}
	ctx := context.Background()
	plist := filepath.Join(home, "Library", "LaunchAgents", Label+".plist")
	logDir := filepath.Join(home, "Library", "Logs", "babysitter")

	fr.reply("launchctl print gui/501/"+Label, "Could not find service", 113)
	st, err := l.Status(ctx)
	if err != nil || st.Installed || st.Loaded || st.Running {
		t.Fatalf("Status() = %+v, %v", st, err)
	}
	if err := l.Start(ctx); err == nil || !strings.Contains(err.Error(), "not installed") {
		t.Fatalf("Start() error = %v", err)
	}

	cfg := Config{Executable: "/usr/local/bin/babysitter", Args: []string{"serve"}, LogDir: logDir}
	fr.calls = nil
	if err := l.Install(ctx, cfg); err != nil {
		t.Fatalf("Install() error = %v", err)
	}
	assertCalls(t, fr.calls,
		"launchctl bootout gui/501/"+Label,
		"launchctl bootstrap gui/501 "+plist,
	)
	if _, err := os.Stat(plist); err != nil {
		t.Fatalf("plist not written: %v", err)
	}
	if _, err := os.Stat(logDir); err != nil {
		t.Fatalf("log dir not created: %v", err)
	}

	fr.reply("launchctl print gui/501/"+Label, "gui/501/"+Label+" = {\n\tactive count = 1\n\tstate = running\n\tpid = 4242\n\tendpoints = {\n\t\t\"x\" = {\n\t\t\tstate = active\n\t\t}\n\t}\n}\n", 0)
	st, err = l.Status(ctx)
	if err != nil || !st.Installed || !st.Loaded || !st.Running || st.PID != 4242 {
		t.Fatalf("Status() = %+v, %v", st, err)
	}
	if st.LogPath != filepath.Join(logDir, "babysitter.log") {
		t.Fatalf("LogPath = %q", st.LogPath)
	}

	fr.calls = nil
	if err := l.Start(ctx); err != nil {
		t.Fatalf("Start() when loaded error = %v", err)
	}
	assertCalls(t, fr.calls, "launchctl print gui/501/"+Label)

	fr.calls = nil
	if err := l.Stop(ctx); err != nil {
		t.Fatalf("Stop() error = %v", err)
	}
	assertCalls(t, fr.calls, "launchctl print gui/501/"+Label, "launchctl bootout gui/501/"+Label)

	fr.reply("launchctl print gui/501/"+Label, "Could not find service", 113)
	fr.calls = nil
	if err := l.Start(ctx); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	assertCalls(t, fr.calls, "launchctl print gui/501/"+Label, "launchctl bootstrap gui/501 "+plist)

	fr.calls = nil
	if err := l.Uninstall(ctx); err != nil {
		t.Fatalf("Uninstall() error = %v", err)
	}
	assertCalls(t, fr.calls, "launchctl print gui/501/"+Label)
	if _, err := os.Stat(plist); !os.IsNotExist(err) {
		t.Fatalf("plist still exists: %v", err)
	}
}

func TestLaunchdKeepsTheLogDirPrivate(t *testing.T) {
	home := t.TempDir()
	fr := &fakeRunner{}
	l := &launchd{home: home, uid: 501, run: fr.run}
	logDir := filepath.Join(home, "Library", "Logs", "babysitter")

	cfg := Config{Executable: "/usr/local/bin/babysitter", Args: []string{"serve"}, LogDir: logDir}
	if err := l.Install(context.Background(), cfg); err != nil {
		t.Fatalf("Install() error = %v", err)
	}

	info, err := os.Stat(logDir)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0o700 {
		t.Fatalf("log dir mode = %o, want 700", got)
	}
}

func TestSystemdLifecycle(t *testing.T) {
	home := t.TempDir()
	fr := &fakeRunner{}
	s := &systemd{home: home, run: fr.run}
	ctx := context.Background()
	unit := filepath.Join(home, ".config", "systemd", "user", UnitName)

	st, err := s.Status(ctx)
	if err != nil || st.Installed {
		t.Fatalf("Status() = %+v, %v", st, err)
	}
	if err := s.Start(ctx); err == nil {
		t.Fatal("Start() before install expected error")
	}

	if err := s.Install(ctx, Config{Executable: "/usr/bin/babysitter", Args: []string{"serve"}}); err != nil {
		t.Fatalf("Install() error = %v", err)
	}
	assertCalls(t, fr.calls,
		"systemctl --user daemon-reload",
		"systemctl --user enable --now "+UnitName,
	)
	if _, err := os.Stat(unit); err != nil {
		t.Fatalf("unit not written: %v", err)
	}

	fr.reply("systemctl --user is-active "+UnitName, "active\n", 0)
	fr.reply("systemctl --user show -p MainPID --value "+UnitName, "77\n", 0)
	st, err = s.Status(ctx)
	if err != nil || !st.Installed || !st.Loaded || !st.Running || st.PID != 77 {
		t.Fatalf("Status() = %+v, %v", st, err)
	}

	fr.reply("systemctl --user is-active "+UnitName, "inactive\n", 3)
	fr.reply("systemctl --user is-enabled "+UnitName, "disabled\n", 1)
	st, err = s.Status(ctx)
	if err != nil || !st.Installed || st.Loaded || st.Running {
		t.Fatalf("Status() inactive = %+v, %v", st, err)
	}

	fr.calls = nil
	if err := s.Stop(ctx); err != nil {
		t.Fatal(err)
	}
	if err := s.Start(ctx); err != nil {
		t.Fatal(err)
	}
	assertCalls(t, fr.calls, "systemctl --user stop "+UnitName, "systemctl --user start "+UnitName)

	fr.calls = nil
	if err := s.Uninstall(ctx); err != nil {
		t.Fatalf("Uninstall() error = %v", err)
	}
	assertCalls(t, fr.calls,
		"systemctl --user disable --now "+UnitName,
		"systemctl --user daemon-reload",
	)
	if _, err := os.Stat(unit); !os.IsNotExist(err) {
		t.Fatalf("unit still exists: %v", err)
	}
}
