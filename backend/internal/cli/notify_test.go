package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/deividfortuna/babysitter/internal/notify"
	"github.com/deividfortuna/babysitter/internal/runfile"
)

type fakeNotifier struct {
	got      notify.Notification
	deadline time.Time
	hasDL    bool
	err      error
}

func (f *fakeNotifier) Send(ctx context.Context, n notify.Notification) (notify.Result, error) {
	f.got = n
	f.deadline, f.hasDL = ctx.Deadline()
	if err := ctx.Err(); err != nil {
		return notify.Result{}, err
	}
	if f.err != nil {
		return notify.Result{}, f.err
	}
	title := n.Title
	if title == "" {
		title = notify.DefaultTitle
	}
	return notify.Result{
		Backend: "fake", Clickable: n.URL != "", Silent: n.Silent,
		Title: title, Subtitle: n.Subtitle, Message: strings.TrimSpace(n.Message), URL: n.URL,
	}, nil
}

func runNotify(t *testing.T, f *fakeNotifier, args ...string) (string, error) {
	t.Helper()
	root := NewRootCmd(WithNotifier(f))
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&bytes.Buffer{})
	root.SetArgs(append([]string{"notify", "--data-dir", t.TempDir()}, args...))
	err := root.ExecuteContext(context.Background())
	return out.String(), err
}

func TestNotifyGivesTheNotificationToTheRunningDaemon(t *testing.T) {
	t.Parallel()
	d := newFakeDaemon()
	notificationRoutes(d)
	f := &fakeNotifier{}

	out, err := runAgainstDaemonWith(t, d, []Option{WithNotifier(f)}, "notify",
		"--title", "PR #42", "--url", "https://github.com/octo/hello/pull/42", "Reply to alice")
	if err != nil {
		t.Fatalf("notify error = %v", err)
	}
	if len(d.posted) != 1 {
		t.Fatalf("the daemon got %d notifications, want 1", len(d.posted))
	}
	if d.posted[0]["title"] != "PR #42" || d.posted[0]["body"] != "Reply to alice" {
		t.Fatalf("body = %v, want the title and the message of the command", d.posted[0])
	}
	if f.got.Message != "" {
		t.Fatalf("the desktop got %+v, want the daemon to show it instead", f.got)
	}
	if !strings.Contains(out, "babysitter") {
		t.Fatalf("notify = %q, want the notification it recorded", out)
	}
}

func TestNotifyShowsItHereWithLocal(t *testing.T) {
	t.Parallel()
	d := newFakeDaemon()
	notificationRoutes(d)
	f := &fakeNotifier{}

	if _, err := runAgainstDaemonWith(t, d, []Option{WithNotifier(f)}, "notify", "--local", "Reply to alice"); err != nil {
		t.Fatalf("notify error = %v", err)
	}
	if len(d.posted) != 0 {
		t.Fatalf("the daemon got %d notifications, want none with --local", len(d.posted))
	}
	if f.got.Message != "Reply to alice" {
		t.Fatalf("the desktop got %+v, want the message of the command", f.got)
	}
}

func TestNotifyShowsItHereWhenTheDaemonRefusesTheRow(t *testing.T) {
	t.Parallel()
	d := newFakeDaemon()
	d.mux.HandleFunc("/api/v1/notifications", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte(`{"error":{"code":"notifications_unavailable","message":"this daemon records no notifications"}}`))
	})
	f := &fakeNotifier{}

	out, err := runAgainstDaemonWith(t, d, []Option{WithNotifier(f)}, "notify", "Reply to alice")
	if err != nil {
		t.Fatalf("notify error = %v, want the notification shown here instead", err)
	}
	if f.got.Message != "Reply to alice" {
		t.Fatalf("the desktop got %+v, want the message the daemon refused", f.got)
	}
	if !strings.Contains(out, "fake") {
		t.Errorf("notify = %q, want the notifier that showed it", out)
	}
}

func TestNotifyShowsItHereWhenTheDaemonHangs(t *testing.T) {
	t.Parallel()
	d := newFakeDaemon()
	release := make(chan struct{})
	defer close(release)
	d.mux.HandleFunc("/api/v1/notifications", func(_ http.ResponseWriter, r *http.Request) {
		select {
		case <-release:
		case <-r.Context().Done():
		}
	})
	f := &fakeNotifier{}

	_, err := runAgainstDaemonWith(t, d, []Option{WithNotifier(f)}, "notify", "--timeout", "2s", "Reply to alice")
	if err != nil {
		t.Fatalf("notify error = %v, want the notification shown here instead", err)
	}
	if f.got.Message != "Reply to alice" {
		t.Fatalf("the desktop got %+v, want the message the daemon never answered for", f.got)
	}
}

func TestNotifyShowsItHereWhenTheDaemonDoesNotAnswer(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.NewServeMux())
	var port int
	fmt.Sscanf(strings.TrimPrefix(srv.URL, "http://127.0.0.1:"), "%d", &port)
	srv.Close()
	dataDir := t.TempDir()
	if err := runfile.Write(runfile.Path(dataDir), runfile.Info{PID: os.Getpid(), Port: port, Owner: runfile.OwnerCLI}); err != nil {
		t.Fatal(err)
	}
	f := &fakeNotifier{}
	root := NewRootCmd(WithNotifier(f))
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&bytes.Buffer{})
	root.SetArgs([]string{"notify", "--data-dir", dataDir, "Reply to alice"})

	if err := root.ExecuteContext(context.Background()); err != nil {
		t.Fatalf("notify error = %v, want the notification shown here instead", err)
	}
	if f.got.Message != "Reply to alice" {
		t.Fatalf("the desktop got %+v, want the message the daemon never took", f.got)
	}
}

func TestNotifyJoinsArgsAndPrintsText(t *testing.T) {
	f := &fakeNotifier{}
	out, err := runNotify(t, f, "Reply", "to", "alice")
	if err != nil {
		t.Fatalf("notify error = %v", err)
	}
	if f.got.Message != "Reply to alice" || f.got.Title != notify.DefaultTitle || f.got.Silent {
		t.Errorf("notification = %+v", f.got)
	}
	if out != "Sent the notification with fake\n" {
		t.Errorf("output = %q", out)
	}
}

func TestNotifyStopsFlagsAtTheMessage(t *testing.T) {
	f := &fakeNotifier{}
	if _, err := runNotify(t, f, "-o", "json", "CI", "failed", "-3", "times", "--silent"); err != nil {
		t.Fatalf("notify error = %v", err)
	}
	if f.got.Message != "CI failed -3 times --silent" || f.got.Silent {
		t.Errorf("notification = %+v", f.got)
	}
}

func TestNotifyAppliesTimeout(t *testing.T) {
	f := &fakeNotifier{}
	before := time.Now()
	if _, err := runNotify(t, f, "--timeout", "5s", "hi"); err != nil {
		t.Fatalf("notify error = %v", err)
	}
	if !f.hasDL || f.deadline.Before(before.Add(4*time.Second)) || f.deadline.After(before.Add(6*time.Second)) {
		t.Errorf("deadline = %v (%v), want about 5s from now", f.deadline, f.hasDL)
	}
	f = &fakeNotifier{}
	if _, err := runNotify(t, f, "--timeout", "0", "hi"); err != nil {
		t.Fatalf("notify error = %v", err)
	}
	if f.hasDL {
		t.Error("--timeout 0: context has a deadline")
	}
}

func TestNotifyFlagsAndJSON(t *testing.T) {
	f := &fakeNotifier{}
	out, err := runNotify(t, f, "-o", "json", "--title", "PR #42", "--subtitle", "octo/hello",
		"--url", "https://github.com/octo/hello/pull/42", "--silent", "CI failed three times")
	if err != nil {
		t.Fatalf("notify error = %v", err)
	}
	want := notify.Notification{
		Title: "PR #42", Subtitle: "octo/hello", Message: "CI failed three times",
		URL: "https://github.com/octo/hello/pull/42", Silent: true,
	}
	if f.got != want {
		t.Errorf("notification = %+v, want %+v", f.got, want)
	}
	var got notifyOutput
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("json: %v\n%s", err, out)
	}
	if !got.Sent || got.Backend != "fake" || !got.Clickable || !got.Silent || got.Title != "PR #42" ||
		got.Subtitle != "octo/hello" || got.Message != "CI failed three times" || got.URL != want.URL {
		t.Errorf("json = %+v", got)
	}
}

func TestNotifyPrintsWhatTheNotifierSent(t *testing.T) {
	f := &fakeNotifier{}
	out, err := runNotify(t, f, "-o", "json", "--title", "", " padded ")
	if err != nil {
		t.Fatalf("notify error = %v", err)
	}
	var got notifyOutput
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("json: %v\n%s", err, out)
	}
	if got.Title != notify.DefaultTitle || got.Message != "padded" {
		t.Errorf("json = %+v, want the normalized title and message", got)
	}
}

func TestNotifyErrors(t *testing.T) {
	if _, err := runNotify(t, &fakeNotifier{}); err == nil || !strings.Contains(err.Error(), "requires at least 1 arg") {
		t.Errorf("no args: err = %v", err)
	}
	boom := errors.New("boom")
	if _, err := runNotify(t, &fakeNotifier{err: boom}, "hi"); !errors.Is(err, boom) {
		t.Errorf("send failure: err = %v, want boom", err)
	}
}

func TestNotifyGivesTheSubtitleToTheDaemonOfItsOwn(t *testing.T) {
	t.Parallel()
	d := newFakeDaemon()
	notificationRoutes(d)

	_, err := runAgainstDaemonWith(t, d, []Option{WithNotifier(&fakeNotifier{})}, "notify",
		"--title", "PR #42", "--subtitle", "octo/hello", "Reply to alice")
	if err != nil {
		t.Fatalf("notify error = %v", err)
	}
	if len(d.posted) != 1 {
		t.Fatalf("the daemon got %d notifications, want 1", len(d.posted))
	}
	if d.posted[0]["subtitle"] != "octo/hello" {
		t.Errorf("subtitle = %v, want the one the command was given", d.posted[0]["subtitle"])
	}
	if d.posted[0]["body"] != "Reply to alice" {
		t.Errorf("body = %v, want the message alone", d.posted[0]["body"])
	}
}

func TestNotifyTakesTheDaemonsWordWhenTheAnswerComesLate(t *testing.T) {
	t.Parallel()
	d := newFakeDaemon()
	release := make(chan struct{})
	defer close(release)
	d.mux.HandleFunc("/api/v1/notifications", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			fmt.Fprintf(w, `{"notifications":[{"id":7,"kind":"agent","repo":"","number":0,"title":"babysitter","body":"Reply to alice","url":"","createdAt":%q}],"unreadCount":1}`,
				time.Now().UTC().Format(time.RFC3339))
			return
		}
		select {
		case <-release:
		case <-r.Context().Done():
		}
	})
	f := &fakeNotifier{}

	out, err := runAgainstDaemonWith(t, d, []Option{WithNotifier(f)}, "notify", "--timeout", "2s", "-o", "json", "Reply to alice")
	if err != nil {
		t.Fatalf("notify error = %v", err)
	}
	if f.got.Message != "" {
		t.Fatalf("the desktop got %+v, want nothing for a row the daemon holds", f.got)
	}
	if !strings.Contains(out, `"recorded": true`) && !strings.Contains(out, `"recorded":true`) {
		t.Fatalf("output = %s, want recorded true", out)
	}
}
