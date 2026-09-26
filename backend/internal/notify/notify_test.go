package notify

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/deividfortuna/babysitter/internal/execx"
)

type fake struct {
	tools   map[string]bool
	calls   [][]string
	replies []reply
}

type reply struct {
	out string
	err error
}

func (f *fake) desktop(goos string) *desktop {
	return &desktop{
		goos: goos,
		look: func(name string) bool { return f.tools[name] },
		run: func(ctx context.Context, name string, args ...string) (string, error) {
			f.calls = append(f.calls, append([]string{name}, args...))
			if len(f.replies) == 0 {
				return "", nil
			}
			r := f.replies[0]
			f.replies = f.replies[1:]
			return r.out, r.err
		},
	}
}

func (f *fake) last() []string {
	if len(f.calls) == 0 {
		return nil
	}
	return f.calls[len(f.calls)-1]
}

func TestSendPrefersTerminalNotifier(t *testing.T) {
	t.Parallel()
	f := &fake{tools: map[string]bool{"terminal-notifier": true, "osascript": true}}
	res, err := f.desktop("darwin").Send(context.Background(), Notification{
		Title: "PR #42", Subtitle: "octo/hello", Message: "Reply to alice", URL: "https://github.com/octo/hello/pull/42",
	})
	if err != nil {
		t.Fatalf("Send() error = %v", err)
	}
	want := Result{
		Backend: "terminal-notifier", Clickable: true,
		Title: "PR #42", Subtitle: "octo/hello", Message: "Reply to alice", URL: "https://github.com/octo/hello/pull/42",
	}
	if res != want {
		t.Errorf("result = %+v, want %+v", res, want)
	}
	wantArgs := []string{
		"terminal-notifier", "-title", "PR #42", "-message", "Reply to alice", "-subtitle", "octo/hello",
		"-open", "https://github.com/octo/hello/pull/42", "-sound", "default",
	}
	if !reflect.DeepEqual(f.last(), wantArgs) {
		t.Errorf("ran %q, want %q", f.last(), wantArgs)
	}
}

func TestTerminalNotifierEscapesValues(t *testing.T) {
	t.Parallel()
	f := &fake{tools: map[string]bool{"terminal-notifier": true}}
	_, err := f.desktop("darwin").Send(context.Background(), Notification{
		Title: "[WIP] Retry limit", Subtitle: "-5 minutes left", Message: `"quoted" start`, Silent: true,
	})
	if err != nil {
		t.Fatalf("Send() error = %v", err)
	}
	want := []string{"terminal-notifier", "-title", `\[WIP] Retry limit`, "-message", `\"quoted" start`, "-subtitle", " -5 minutes left"}
	if !reflect.DeepEqual(f.last(), want) {
		t.Errorf("ran %q, want %q", f.last(), want)
	}
}

func TestDefaultsValue(t *testing.T) {
	t.Parallel()
	cases := map[string]string{
		"":              "",
		"plain":         "plain",
		"[tag] x":       `\[tag] x`,
		"(a)":           `\(a)`,
		"{b}":           `\{b}`,
		`"q"`:           `\"q"`,
		"'q'":           `\'q'`,
		"<x>":           `\<x>`,
		`\back`:         `\\back`,
		"-dash":         " -dash",
		"middle [tag]":  "middle [tag]",
		"middle - dash": "middle - dash",
	}
	for in, want := range cases {
		if got := defaultsValue(in); got != want {
			t.Errorf("defaultsValue(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestSendOsascript(t *testing.T) {
	t.Parallel()
	f := &fake{tools: map[string]bool{"osascript": true}}
	res, err := f.desktop("darwin").Send(context.Background(), Notification{
		Message: `Say "hi" to bob`, URL: "https://example.com", Silent: true,
	})
	if err != nil {
		t.Fatalf("Send() error = %v", err)
	}
	want := Result{Backend: "osascript", Silent: true, Title: DefaultTitle, Message: `Say "hi" to bob`, URL: "https://example.com"}
	if res != want {
		t.Errorf("result = %+v, want %+v", res, want)
	}
	wantArgs := []string{
		"osascript", "-e", "on run argv",
		"-e", "display notification (item 1 of argv) with title (item 2 of argv)",
		"-e", "end run", "--",
		"Say \"hi\" to bob\nhttps://example.com", DefaultTitle, "",
	}
	if !reflect.DeepEqual(f.last(), wantArgs) {
		t.Errorf("ran %q, want %q", f.last(), wantArgs)
	}
}

func TestSendOsascriptSubtitleAndSound(t *testing.T) {
	t.Parallel()
	f := &fake{tools: map[string]bool{"osascript": true}}
	if _, err := f.desktop("darwin").Send(context.Background(), Notification{Subtitle: "sub", Message: "-1 approval left"}); err != nil {
		t.Fatalf("Send() error = %v", err)
	}
	args := f.last()
	script := args[4]
	if !strings.Contains(script, "subtitle (item 3 of argv)") || !strings.Contains(script, `sound name "default"`) {
		t.Errorf("script = %q", script)
	}
	if args[7] != "--" || args[8] != "-1 approval left" || args[len(args)-1] != "sub" {
		t.Errorf("arguments = %q", args[7:])
	}
}

func TestSendNotifySend(t *testing.T) {
	t.Parallel()
	f := &fake{tools: map[string]bool{"notify-send": true}}
	res, err := f.desktop("linux").Send(context.Background(), Notification{
		Title: "t", Subtitle: "s", Message: "m", URL: "u", Silent: true,
	})
	if err != nil {
		t.Fatalf("Send() error = %v", err)
	}
	want := Result{Backend: "notify-send", Silent: true, Title: "t", Subtitle: "s", Message: "m", URL: "u"}
	if res != want {
		t.Errorf("result = %+v, want %+v", res, want)
	}
	wantArgs := []string{"notify-send", "--app-name", DefaultTitle, "--hint", "boolean:suppress-sound:true", "--", "t", "s\nm\nu"}
	if !reflect.DeepEqual(f.last(), wantArgs) {
		t.Errorf("ran %q, want %q", f.last(), wantArgs)
	}
}

func TestNotifySendEscapesMarkup(t *testing.T) {
	t.Parallel()
	f := &fake{tools: map[string]bool{"notify-send": true}}
	_, err := f.desktop("linux").Send(context.Background(), Notification{
		Title: "-PR 42 <b>", Message: "change Vec<String> to Vec<&str>", URL: "https://x.test/?a=1&b=2",
	})
	if err != nil {
		t.Fatalf("Send() error = %v", err)
	}
	want := []string{
		"notify-send", "--app-name", DefaultTitle, "--", "-PR 42 <b>",
		"change Vec&lt;String&gt; to Vec&lt;&amp;str&gt;\nhttps://x.test/?a=1&amp;b=2",
	}
	if !reflect.DeepEqual(f.last(), want) {
		t.Errorf("ran %q, want %q", f.last(), want)
	}
}

func TestNotifySendRetriesWithoutBooleanHint(t *testing.T) {
	t.Parallel()
	out := `Invalid hint type "boolean". Valid types are int, double, string and byte.`
	f := &fake{tools: map[string]bool{"notify-send": true}, replies: []reply{
		{out: out, err: &execx.ExitError{Name: "notify-send", Code: 1, Output: out}},
	}}
	res, err := f.desktop("linux").Send(context.Background(), Notification{Title: "t", Message: "m", Silent: true})
	if err != nil {
		t.Fatalf("Send() error = %v", err)
	}
	if res.Silent {
		t.Error("result.Silent = true after the fallback, want false")
	}
	if len(f.calls) != 2 {
		t.Fatalf("calls = %d, want 2", len(f.calls))
	}
	want := []string{"notify-send", "--app-name", DefaultTitle, "--", "t", "m"}
	if !reflect.DeepEqual(f.calls[1], want) {
		t.Errorf("second call %q, want %q", f.calls[1], want)
	}

	f = &fake{tools: map[string]bool{"notify-send": true}, replies: []reply{
		{out: "boom", err: &execx.ExitError{Name: "notify-send", Code: 1, Output: "boom"}},
	}}
	if _, err := f.desktop("linux").Send(context.Background(), Notification{Message: "m", Silent: true}); err == nil {
		t.Error("other failure: expected error")
	}
	if len(f.calls) != 1 {
		t.Errorf("other failure: calls = %d, want 1", len(f.calls))
	}
}

func TestSendNormalizesContent(t *testing.T) {
	t.Parallel()
	f := &fake{tools: map[string]bool{"osascript": true}}
	res, err := f.desktop("darwin").Send(context.Background(), Notification{Message: "  padded  "})
	if err != nil {
		t.Fatalf("Send() error = %v", err)
	}
	if res.Title != DefaultTitle || res.Message != "padded" {
		t.Errorf("result = %+v", res)
	}
	if got := f.last()[8]; got != "padded" {
		t.Errorf("message argument = %q, want padded", got)
	}
}

func TestSendErrors(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	if _, err := (&fake{tools: map[string]bool{"osascript": true}}).desktop("darwin").Send(ctx, Notification{Message: "  "}); err == nil {
		t.Error("empty message: expected error")
	}
	if _, err := (&fake{}).desktop("darwin").Send(ctx, Notification{Message: "m"}); !errors.Is(err, ErrNoBackend) {
		t.Errorf("no tool on darwin: err = %v, want ErrNoBackend", err)
	}
	if _, err := (&fake{}).desktop("linux").Send(ctx, Notification{Message: "m"}); !errors.Is(err, ErrNoBackend) {
		t.Errorf("no tool on linux: err = %v, want ErrNoBackend", err)
	}
	if _, err := (&fake{}).desktop("windows").Send(ctx, Notification{Message: "m"}); !errors.Is(err, ErrUnsupported) {
		t.Errorf("windows: err = %v, want ErrUnsupported", err)
	}
	boom := errors.New("boom")
	f := &fake{tools: map[string]bool{"notify-send": true}, replies: []reply{{err: boom}}}
	if _, err := f.desktop("linux").Send(ctx, Notification{Message: "m"}); !errors.Is(err, boom) {
		t.Errorf("tool failure: err = %v, want boom", err)
	}
}
