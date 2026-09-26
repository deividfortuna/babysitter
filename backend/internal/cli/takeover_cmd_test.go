package cli

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/deividfortuna/babysitter/internal/httpd"
)

type execCall struct {
	dir  string
	argv []string
}

func (d *fakeDaemon) takeoverRoutes() {
	d.mux.HandleFunc("POST /api/v1/watches/1/takeover", func(w http.ResponseWriter, r *http.Request) {
		var body httpd.TakeoverRequest
		_ = json.NewDecoder(r.Body).Decode(&body)
		d.takeovers = append(d.takeovers, body)
		one := strings.TrimSuffix(strings.TrimPrefix(d.watches, "["), "]")
		fmt.Fprintf(w, `{"watch":%s,"worktreeDir":"/data/worktrees/octo-hello-3","workBranch":"babysitter/fix","headRef":"fix",
			"argv":["claude","--resume","sess-1"],"declined":[4],"newConversation":false}`, one)
	})
	d.mux.HandleFunc("POST /api/v1/watches/1/handback", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Confirm bool `json:"confirm"`
			Force   bool `json:"force"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		d.handbacks = append(d.handbacks, fmt.Sprintf("confirm=%v force=%v", body.Confirm, body.Force))
		switch {
		case d.authorRunning && !body.Force:
			w.WriteHeader(http.StatusConflict)
			fmt.Fprint(w, `{"error":{"code":"author_running","message":"the session is still open in your terminal (pid 51920)"}}`)
		case d.authorWork && !body.Confirm:
			w.WriteHeader(http.StatusConflict)
			fmt.Fprint(w, `{"error":{"code":"unconfirmed_work","message":"the worktree has work that is not on the pull request"},
				"commits":[{"sha":"5d0b7f1aaaaaa","subject":"Start the webhook sink"},{"sha":"9a1c3e2bbbbbb","subject":"Wait for the sink"}],
				"files":[" M internal/webhook/deliver_test.go"]}`)
		default:
			fmt.Fprint(w, strings.TrimSuffix(strings.TrimPrefix(d.watches, "["), "]"))
		}
	})
}

func (d *fakeDaemon) takenOver() {
	d.watches = strings.Replace(d.watches, `"agentSession":"sess-1",`, `"agentSession":"sess-1","takenOverAt":"2026-09-26T12:00:00Z",`, 1)
}

func withExec(calls *[]execCall) Option {
	return WithExec(func(dir string, argv []string) error {
		*calls = append(*calls, execCall{dir: dir, argv: argv})
		return nil
	})
}

func TestTakeoverGivesTheSessionToTheTerminal(t *testing.T) {
	t.Parallel()
	d := newFakeDaemon()
	d.takeoverRoutes()
	var calls []execCall

	run := runInTerminal(t, d, []Option{withExec(&calls)}, "", "watch", "takeover", "octo/hello#3")

	if run.err != nil {
		t.Fatalf("takeover error = %v", run.err)
	}
	if !slices.Equal(d.takeovers, []httpd.TakeoverRequest{{PID: os.Getpid()}}) {
		t.Fatalf("takeovers = %v", d.takeovers)
	}
	for _, want := range []string{
		"The session of octo/hello#3 is now yours.",
		"worktree  /data/worktrees/octo-hello-3",
		"branch    babysitter/fix",
		"push      git push origin HEAD:fix",
		"Proposal 4 was declined; nothing of it went out.",
		"Give it back with: babysitter watch handback octo/hello#3",
	} {
		if !strings.Contains(run.errOut, want) {
			t.Errorf("banner lacks %q:\n%s", want, run.errOut)
		}
	}
	want := []execCall{{dir: "/data/worktrees/octo-hello-3", argv: []string{"claude", "--resume", "sess-1"}}}
	if len(calls) != 1 || calls[0].dir != want[0].dir || !slices.Equal(calls[0].argv, want[0].argv) {
		t.Fatalf("exec = %+v, want %+v", calls, want)
	}
}

func TestTakeoverWithShellRunsTheShellOfTheAuthor(t *testing.T) {
	t.Setenv("SHELL", "/bin/zsh")
	d := newFakeDaemon()
	d.takeoverRoutes()
	var calls []execCall

	run := runInTerminal(t, d, []Option{withExec(&calls)}, "", "watch", "takeover", "1", "--shell")

	if run.err != nil {
		t.Fatalf("takeover error = %v", run.err)
	}
	if len(calls) != 1 || !slices.Equal(calls[0].argv, []string{"/bin/zsh"}) {
		t.Fatalf("exec = %+v", calls)
	}
	if !slices.Equal(d.takeovers, []httpd.TakeoverRequest{{PID: os.Getpid(), Shell: true}}) {
		t.Fatalf("takeovers = %+v", d.takeovers)
	}
}

func TestHandbackGivesTheSessionBack(t *testing.T) {
	t.Parallel()
	d := newFakeDaemon()
	d.takeoverRoutes()

	run := runInTerminal(t, d, nil, "", "watch", "handback", "1")

	if run.err != nil || !strings.Contains(run.out, "The session of octo/hello#3 is back with the agent") {
		t.Fatalf("handback = %q, %v", run.out, run.err)
	}
	if !slices.Equal(d.handbacks, []string{"confirm=false force=false"}) {
		t.Fatalf("handbacks = %v", d.handbacks)
	}
}

func TestHandbackAsksBeforeItGivesBackWorkThatIsNotPushed(t *testing.T) {
	t.Parallel()
	d := newFakeDaemon()
	d.takeoverRoutes()
	d.authorWork = true

	run := runInTerminal(t, d, nil, "y\n", "watch", "handback", "1")

	if run.err != nil {
		t.Fatalf("handback error = %v", run.err)
	}
	for _, want := range []string{
		"2 commits the pull request does not have:",
		"5d0b7f1 Start the webhook sink",
		"9a1c3e2 Wait for the sink",
		"1 file changed and not committed:",
		" M internal/webhook/deliver_test.go",
		"Hand back with this work? [y/N]",
	} {
		if !strings.Contains(run.errOut, want) {
			t.Errorf("question lacks %q:\n%s", want, run.errOut)
		}
	}
	if !slices.Equal(d.handbacks, []string{"confirm=false force=false", "confirm=true force=false"}) {
		t.Fatalf("handbacks = %v", d.handbacks)
	}
}

func TestHandbackKeepsTheSessionWhenTheAuthorSaysNo(t *testing.T) {
	t.Parallel()
	d := newFakeDaemon()
	d.takeoverRoutes()
	d.authorWork = true

	run := runInTerminal(t, d, nil, "\n", "watch", "handback", "1")

	if run.err == nil || !strings.Contains(run.err.Error(), "--yes") {
		t.Fatalf("handback error = %v", run.err)
	}
	if len(d.handbacks) != 1 {
		t.Fatalf("handbacks = %v", d.handbacks)
	}
}

func TestHandbackWithYesAsksNothing(t *testing.T) {
	t.Parallel()
	d := newFakeDaemon()
	d.takeoverRoutes()
	d.authorWork = true

	run := runInTerminal(t, d, nil, "", "watch", "handback", "1", "--yes")

	if run.err != nil || strings.Contains(run.errOut, "[y/N]") {
		t.Fatalf("handback = %q, %v", run.errOut, run.err)
	}
	if !slices.Equal(d.handbacks, []string{"confirm=true force=false"}) {
		t.Fatalf("handbacks = %v", d.handbacks)
	}
}

func TestHandbackWhileTheAgentOfTheAuthorRunsNeedsForce(t *testing.T) {
	t.Parallel()
	d := newFakeDaemon()
	d.takeoverRoutes()
	d.authorRunning = true

	run := runInTerminal(t, d, nil, "", "watch", "handback", "1")
	if run.err == nil || !strings.Contains(run.err.Error(), "pid 51920") || !strings.Contains(run.err.Error(), "--force") {
		t.Fatalf("handback error = %v", run.err)
	}

	run = runInTerminal(t, d, nil, "", "watch", "handback", "1", "--force")
	if run.err != nil {
		t.Fatalf("a forced handback error = %v", run.err)
	}
	if d.handbacks[len(d.handbacks)-1] != "confirm=false force=true" {
		t.Fatalf("handbacks = %v", d.handbacks)
	}
}

func TestStatusAndListSayTheSessionIsWithYou(t *testing.T) {
	t.Parallel()
	d := newFakeDaemon()
	d.takenOver()

	out, err := runWatch(t, d, "status", "1")
	if err != nil || !strings.Contains(out, "Agent:     with you since 2026-09-26T12:00:00Z") {
		t.Fatalf("status = %q, %v", out, err)
	}
	out, err = runWatch(t, d, "list")
	if err != nil || !strings.Contains(out, "with you") || strings.Contains(out, "idle") {
		t.Fatalf("list = %q, %v", out, err)
	}
}

func TestStatusAndListOfAStoppedWatchDoNotSayTheSessionIsWithYou(t *testing.T) {
	t.Parallel()
	d := newFakeDaemon()
	d.takenOver()
	d.watches = strings.Replace(d.watches, `"status":"active"`, `"status":"stopped"`, 1)

	out, err := runWatch(t, d, "status", "1")
	if err != nil || strings.Contains(out, "with you") {
		t.Fatalf("status = %q, %v", out, err)
	}
	out, err = runWatch(t, d, "list")
	if err != nil || strings.Contains(out, "with you") {
		t.Fatalf("list = %q, %v", out, err)
	}
}
