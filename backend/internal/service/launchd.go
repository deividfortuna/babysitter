package service

import (
	"bytes"
	"context"
	"encoding/xml"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"text/template"

	"github.com/deividfortuna/babysitter/internal/execx"
)

type launchd struct {
	home string
	uid  int
	run  execx.Runner
}

func launchdPath(home string) string {
	return strings.Join([]string{
		"/opt/homebrew/bin",
		"/opt/homebrew/sbin",
		"/usr/local/bin",
		filepath.Join(home, ".local", "bin"),
		"/usr/bin",
		"/bin",
		"/usr/sbin",
		"/sbin",
	}, ":")
}

var plistTemplate = template.Must(template.New("plist").Funcs(template.FuncMap{"xml": xmlEscape}).Parse(
	`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>Label</key>
    <string>{{.Label}}</string>
    <key>ProgramArguments</key>
    <array>
        <string>{{xml .Executable}}</string>
{{- range .Args}}
        <string>{{xml .}}</string>
{{- end}}
    </array>
    <key>EnvironmentVariables</key>
    <dict>
        <key>PATH</key>
        <string>{{xml .Path}}</string>
    </dict>
    <key>RunAtLoad</key>
    <true/>
    <key>KeepAlive</key>
    <dict>
        <key>SuccessfulExit</key>
        <false/>
    </dict>
    <key>ProcessType</key>
    <string>Background</string>
    <key>StandardOutPath</key>
    <string>{{xml .LogFile}}</string>
    <key>StandardErrorPath</key>
    <string>{{xml .LogFile}}</string>
</dict>
</plist>
`))

func xmlEscape(s string) string {
	var b bytes.Buffer
	_ = xml.EscapeText(&b, []byte(s))
	return b.String()
}

func renderPlist(cfg Config, home string) ([]byte, error) {
	if err := cfg.validate(); err != nil {
		return nil, err
	}
	if cfg.LogDir == "" {
		return nil, fmt.Errorf("log dir is required on macOS")
	}
	var b bytes.Buffer
	err := plistTemplate.Execute(&b, struct {
		Label      string
		Executable string
		Args       []string
		Path       string
		LogFile    string
	}{Label, cfg.Executable, cfg.Args, launchdPath(home), filepath.Join(cfg.LogDir, "babysitter.log")})
	if err != nil {
		return nil, fmt.Errorf("render plist: %w", err)
	}
	return b.Bytes(), nil
}

func (l *launchd) plistPath() string {
	return filepath.Join(l.home, "Library", "LaunchAgents", Label+".plist")
}

func (l *launchd) domain() string {
	return fmt.Sprintf("gui/%d", l.uid)
}

func (l *launchd) target() string {
	return l.domain() + "/" + Label
}

func (l *launchd) Install(ctx context.Context, cfg Config) error {
	data, err := renderPlist(cfg, l.home)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(cfg.LogDir, 0o700); err != nil {
		return fmt.Errorf("create log dir: %w", err)
	}
	if err := writeFile(l.plistPath(), data); err != nil {
		return err
	}
	_, _ = l.run(ctx, "launchctl", "bootout", l.target())
	if _, err := l.run(ctx, "launchctl", "bootstrap", l.domain(), l.plistPath()); err != nil {
		return fmt.Errorf("load service: %w", err)
	}
	return nil
}

func (l *launchd) Uninstall(ctx context.Context) error {
	if st, err := l.Status(ctx); err == nil && st.Loaded {
		if _, err := l.run(ctx, "launchctl", "bootout", l.target()); err != nil {
			return fmt.Errorf("unload service: %w", err)
		}
	}
	return removeFile(l.plistPath())
}

func (l *launchd) Start(ctx context.Context) error {
	st, err := l.Status(ctx)
	if err != nil {
		return err
	}
	if !st.Installed {
		return fmt.Errorf("service is not installed, run 'babysitter service install' first")
	}
	if st.Loaded {
		return nil
	}
	if _, err := l.run(ctx, "launchctl", "bootstrap", l.domain(), l.plistPath()); err != nil {
		return fmt.Errorf("start service: %w", err)
	}
	return nil
}

func (l *launchd) Stop(ctx context.Context) error {
	st, err := l.Status(ctx)
	if err != nil {
		return err
	}
	if !st.Loaded {
		return nil
	}
	if _, err := l.run(ctx, "launchctl", "bootout", l.target()); err != nil {
		return fmt.Errorf("stop service: %w", err)
	}
	return nil
}

func (l *launchd) Status(ctx context.Context) (Status, error) {
	st := Status{Path: l.plistPath(), Installed: fileExists(l.plistPath())}
	if st.Installed {
		st.LogPath = logPathFromPlist(l.plistPath())
	}
	out, err := l.run(ctx, "launchctl", "print", l.target())
	if err != nil {
		if execx.ExitCode(err) > 0 {
			return st, nil
		}
		return st, fmt.Errorf("query service: %w", err)
	}
	st.Loaded = true
	for line := range strings.SplitSeq(out, "\n") {
		if !strings.HasPrefix(line, "\t") || strings.HasPrefix(line, "\t\t") {
			continue
		}
		key, value, ok := strings.Cut(strings.TrimSpace(line), " = ")
		if !ok {
			continue
		}
		switch key {
		case "state":
			st.Running = value == "running"
		case "pid":
			st.PID = parseInt(value)
		}
	}
	return st, nil
}

func logPathFromPlist(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	_, rest, ok := strings.Cut(string(data), "<key>StandardOutPath</key>")
	if !ok {
		return ""
	}
	_, rest, ok = strings.Cut(rest, "<string>")
	if !ok {
		return ""
	}
	value, _, _ := strings.Cut(rest, "</string>")
	return strings.TrimSpace(value)
}
