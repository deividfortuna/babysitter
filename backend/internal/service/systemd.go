package service

import (
	"bytes"
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"text/template"

	"github.com/deividfortuna/babysitter/internal/execx"
)

type systemd struct {
	home string
	run  execx.Runner
}

var unitTemplate = template.Must(template.New("unit").Funcs(template.FuncMap{"quote": systemdQuote}).Parse(
	`[Unit]
Description=babysitter GitHub pull request watcher
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
ExecStart={{quote .Executable}}{{range .Args}} {{quote .}}{{end}}
Restart=on-failure
RestartSec=10

[Install]
WantedBy=default.target
`))

func systemdQuote(s string) string {
	if s != "" && !strings.ContainsAny(s, " \t\"'\\") {
		return s
	}
	return `"` + strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(s) + `"`
}

func renderUnit(cfg Config) ([]byte, error) {
	if err := cfg.validate(); err != nil {
		return nil, err
	}
	var b bytes.Buffer
	if err := unitTemplate.Execute(&b, cfg); err != nil {
		return nil, fmt.Errorf("render unit: %w", err)
	}
	return b.Bytes(), nil
}

func (s *systemd) unitPath() string {
	return filepath.Join(s.home, ".config", "systemd", "user", UnitName)
}

func (s *systemd) Install(ctx context.Context, cfg Config) error {
	data, err := renderUnit(cfg)
	if err != nil {
		return err
	}
	if err := writeFile(s.unitPath(), data); err != nil {
		return err
	}
	if _, err := s.run(ctx, "systemctl", "--user", "daemon-reload"); err != nil {
		return fmt.Errorf("reload systemd: %w", err)
	}
	if _, err := s.run(ctx, "systemctl", "--user", "enable", "--now", UnitName); err != nil {
		return fmt.Errorf("enable service: %w", err)
	}
	return nil
}

func (s *systemd) Uninstall(ctx context.Context) error {
	if fileExists(s.unitPath()) {
		if _, err := s.run(ctx, "systemctl", "--user", "disable", "--now", UnitName); err != nil {
			return fmt.Errorf("disable service: %w", err)
		}
	}
	if err := removeFile(s.unitPath()); err != nil {
		return err
	}
	if _, err := s.run(ctx, "systemctl", "--user", "daemon-reload"); err != nil {
		return fmt.Errorf("reload systemd: %w", err)
	}
	return nil
}

func (s *systemd) Start(ctx context.Context) error {
	if !fileExists(s.unitPath()) {
		return fmt.Errorf("service is not installed, run 'babysitter service install' first")
	}
	if _, err := s.run(ctx, "systemctl", "--user", "start", UnitName); err != nil {
		return fmt.Errorf("start service: %w", err)
	}
	return nil
}

func (s *systemd) Stop(ctx context.Context) error {
	if _, err := s.run(ctx, "systemctl", "--user", "stop", UnitName); err != nil {
		return fmt.Errorf("stop service: %w", err)
	}
	return nil
}

func (s *systemd) Status(ctx context.Context) (Status, error) {
	st := Status{Path: s.unitPath(), Installed: fileExists(s.unitPath())}
	if !st.Installed {
		return st, nil
	}
	if _, err := s.run(ctx, "systemctl", "--user", "is-enabled", UnitName); err == nil {
		st.Loaded = true
	} else if execx.ExitCode(err) < 0 {
		return st, fmt.Errorf("query service: %w", err)
	}
	out, err := s.run(ctx, "systemctl", "--user", "is-active", UnitName)
	if err != nil && execx.ExitCode(err) < 0 {
		return st, fmt.Errorf("query service: %w", err)
	}
	st.Running = strings.TrimSpace(out) == "active"
	if st.Running {
		out, err := s.run(ctx, "systemctl", "--user", "show", "-p", "MainPID", "--value", UnitName)
		if err == nil {
			st.PID = parseInt(out)
		}
	}
	return st, nil
}
