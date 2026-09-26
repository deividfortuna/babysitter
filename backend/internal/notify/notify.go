package notify

import (
	"context"
	"errors"
	"runtime"
	"strings"

	"github.com/deividfortuna/babysitter/internal/execx"
)

var ErrUnsupported = errors.New("desktop notifications are only supported on macOS and Linux")

var ErrNoBackend = errors.New("no notification tool found: install terminal-notifier on macOS or notify-send on Linux")

const DefaultTitle = "babysitter"

type Notification struct {
	Title    string
	Subtitle string
	Message  string
	URL      string
	Silent   bool
}

type Result struct {
	Backend   string `json:"backend"`
	Clickable bool   `json:"clickable"`
	Silent    bool   `json:"silent"`
	Title     string `json:"title"`
	Subtitle  string `json:"subtitle,omitempty"`
	Message   string `json:"message"`
	URL       string `json:"url,omitempty"`
}

type Notifier interface {
	Send(ctx context.Context, n Notification) (Result, error)
}

type desktop struct {
	goos string
	run  execx.Runner
	look execx.LookPath
}

func New() Notifier {
	return &desktop{goos: runtime.GOOS, run: execx.Run, look: execx.Found}
}

func (d *desktop) Send(ctx context.Context, n Notification) (Result, error) {
	n.Message = strings.TrimSpace(n.Message)
	if n.Message == "" {
		return Result{}, errors.New("notification message is empty")
	}
	if n.Title == "" {
		n.Title = DefaultTitle
	}
	var (
		res Result
		err error
	)
	switch d.goos {
	case "darwin":
		switch {
		case d.look("terminal-notifier"):
			res, err = d.terminalNotifier(ctx, n)
		case d.look("osascript"):
			res, err = d.osascript(ctx, n)
		default:
			return Result{}, ErrNoBackend
		}
	case "linux":
		if !d.look("notify-send") {
			return Result{}, ErrNoBackend
		}
		res, err = d.notifySend(ctx, n)
	default:
		return Result{}, ErrUnsupported
	}
	if err != nil {
		return Result{}, err
	}
	res.Title, res.Subtitle, res.Message, res.URL = n.Title, n.Subtitle, n.Message, n.URL
	return res, nil
}

func (d *desktop) terminalNotifier(ctx context.Context, n Notification) (Result, error) {
	args := []string{"-title", defaultsValue(n.Title), "-message", defaultsValue(n.Message)}
	if n.Subtitle != "" {
		args = append(args, "-subtitle", defaultsValue(n.Subtitle))
	}
	if n.URL != "" {
		args = append(args, "-open", n.URL)
	}
	if !n.Silent {
		args = append(args, "-sound", "default")
	}
	if _, err := d.run(ctx, "terminal-notifier", args...); err != nil {
		return Result{}, err
	}
	return Result{Backend: "terminal-notifier", Clickable: n.URL != "", Silent: n.Silent}, nil
}

func defaultsValue(v string) string {
	if v == "" {
		return v
	}
	switch v[0] {
	case '[', '(', '{', '"', '\'', '<', '\\':
		return `\` + v
	case '-':
		return " " + v
	}
	return v
}

func (d *desktop) osascript(ctx context.Context, n Notification) (Result, error) {
	script := "display notification (item 1 of argv) with title (item 2 of argv)"
	if n.Subtitle != "" {
		script += " subtitle (item 3 of argv)"
	}
	if !n.Silent {
		script += ` sound name "default"`
	}
	args := []string{
		"-e", "on run argv", "-e", script, "-e", "end run",
		"--", withURL(n.Message, n.URL), n.Title, n.Subtitle,
	}
	if _, err := d.run(ctx, "osascript", args...); err != nil {
		return Result{}, err
	}
	return Result{Backend: "osascript", Silent: n.Silent}, nil
}

var markupEscaper = strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;")

var silentHint = []string{"--hint", "boolean:suppress-sound:true"}

func (d *desktop) notifySend(ctx context.Context, n Notification) (Result, error) {
	body := withURL(n.Message, n.URL)
	if n.Subtitle != "" {
		body = n.Subtitle + "\n" + body
	}
	body = markupEscaper.Replace(body)
	args := []string{"--app-name", DefaultTitle}
	if n.Silent {
		args = append(args, silentHint...)
	}
	args = append(args, "--", n.Title, body)
	out, err := d.run(ctx, "notify-send", args...)
	silent := n.Silent
	if err != nil && silent && execx.ExitCode(err) > 0 && strings.Contains(out, "Invalid hint type") {
		silent = false
		args = append([]string{"--app-name", DefaultTitle}, args[len(silentHint)+2:]...)
		_, err = d.run(ctx, "notify-send", args...)
	}
	if err != nil {
		return Result{}, err
	}
	return Result{Backend: "notify-send", Silent: silent}, nil
}

func withURL(message, url string) string {
	if url == "" {
		return message
	}
	return message + "\n" + url
}
