package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/deividfortuna/babysitter/internal/httpd"
	"github.com/deividfortuna/babysitter/internal/runfile"
	"github.com/deividfortuna/babysitter/internal/store"
)

func badSettings(w http.ResponseWriter, reason string) {
	w.WriteHeader(http.StatusBadRequest)
	text, _ := json.Marshal("invalid settings: " + reason)
	fmt.Fprintf(w, `{"error":{"code":"bad_request","message":%s}}`, text)
}

type fakeDaemon struct {
	mux           *http.ServeMux
	starts        []map[string]any
	stops         []string
	sent          []string
	replies       []string
	replyPosted   bool
	retries       []string
	decisions     []string
	hooks         []string
	waits         []string
	polls         int
	watches       string
	activity      string
	settings      map[string]any
	settingsPut   []map[string]any
	settingsGets  int
	notifications string
	posted        []map[string]any
	reads         []map[string]any
	listQuery     string
	rateLimit     string
	takeovers     []httpd.TakeoverRequest
	handbacks     []string
	authorRunning bool
	authorWork    bool
}

func newFakeDaemon() *fakeDaemon {
	d := &fakeDaemon{mux: http.NewServeMux()}
	d.watches = `[{"id":1,"repo":"octo/hello","number":3,"url":"https://github.com/octo/hello/pull/3","title":"Fix the thing","author":"alice",
		"headRef":"fix","baseRef":"main","sourceDir":"/src","worktreeDir":"/data/worktrees/octo-hello-3","status":"active","stopReason":"",
		"includeExisting":false,"startedAt":"2026-09-07T12:00:00Z","lastPollAt":"2026-09-07T12:03:00Z","lastError":"","headSha":"abcdef1234567",
		"prState":"open","mergeableState":"clean","checkStates":{"build":"passed"},"greenSha":"abcdef1234567","agentSession":"sess-1",
		"session":{"state":"idle","pid":4242,"startedAt":"2026-09-07T12:00:00Z","logPath":"/data/sessions/1.log"}}]`
	d.activity = `[{"id":1,"watchId":1,"kind":"watch_started","ref":"start","at":"2026-09-07T12:00:00Z","actor":"","summary":"watching octo/hello#3","url":"","payload":{},"reported":true},
		{"id":2,"watchId":1,"kind":"comment","ref":"11","at":"2026-09-07T12:03:00Z","actor":"bob","summary":"bob commented: hi","url":"","payload":{},"reported":true}]`
	d.settings = map[string]any{
		"pollIntervalSeconds": 60, "watchIntervalSeconds": 180, "watchMaxIntervalSeconds": 900, "mergeMethod": "",
		"includeExisting": false, "includeOwn": false, "keepWorktree": false,
	}
	d.mux.HandleFunc("/api/v1/healthz", func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, `{"status":"ok"}`) })
	d.mux.HandleFunc("/api/v1/settings", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPut {
			var body map[string]any
			_ = json.NewDecoder(r.Body).Decode(&body)
			if n, ok := body["approvalsRequired"].(float64); ok && n < 0 {
				d.settingsPut = append(d.settingsPut, body)
				badSettings(w, fmt.Sprintf("the approvals must be 0 or more, got %d", int(n)))
				return
			}
			if seconds, ok := body["pollIntervalSeconds"].(float64); ok && seconds < 10 {
				badSettings(w, "the repository poll interval must be between 10s and 24h0m0s, got 2s")
				return
			}
			if method, ok := body["mergeMethod"].(string); ok && !slices.Contains([]string{"", "squash", "merge", "rebase"}, method) {
				badSettings(w, fmt.Sprintf("unknown merge method %q: use squash, merge, rebase", method))
				return
			}
			muted, _ := body["mutedNotificationKinds"].([]any)
			silent, _ := body["silentNotificationKinds"].([]any)
			for _, kind := range slices.Concat(muted, silent) {
				if name, _ := kind.(string); !store.NotificationKind(name).Valid() {
					badSettings(w, fmt.Sprintf("unknown notification kind %q: use %s", name, store.JoinKinds()))
					return
				}
			}
			d.settingsPut = append(d.settingsPut, body)
			d.settings = body
		} else {
			d.settingsGets++
		}
		_ = json.NewEncoder(w).Encode(d.settings)
	})
	d.rateLimit = `{"state":"unknown","limit":0,"remaining":0}`
	d.mux.HandleFunc("/api/v1/ratelimit", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, d.rateLimit)
	})
	d.mux.HandleFunc("/api/v1/watches", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			var body map[string]any
			_ = json.NewDecoder(r.Body).Decode(&body)
			d.starts = append(d.starts, body)
			if body["target"] == "octo/hello#4" {
				w.WriteHeader(http.StatusConflict)
				fmt.Fprint(w, `{"error":{"code":"watch_exists","message":"the pull request is already watched as watch 7"}}`)
				return
			}
			w.WriteHeader(http.StatusCreated)
			fmt.Fprint(w, strings.TrimSuffix(strings.TrimPrefix(d.watches, "["), "]"))
			return
		}
		fmt.Fprintf(w, `{"watches":%s}`, d.watches)
	})
	d.mux.HandleFunc("/api/v1/watches/1", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, strings.TrimSuffix(strings.TrimPrefix(d.watches, "["), "]"))
	})
	d.mux.HandleFunc("/api/v1/watches/1/stop", func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		d.stops = append(d.stops, fmt.Sprintf("%s keepWorktree=%v", r.Method, body["keepWorktree"]))
		one := strings.TrimSuffix(strings.TrimPrefix(d.watches, "["), "]")
		one = strings.Replace(one, `"status":"active","stopReason":""`, `"status":"stopped","stopReason":"user","summary":{"prState":"open","headSha":"abcdef1234567","checks":"green","mergeableState":"clean","activity":{"comment":1},"messages":3,"reason":"user","worktreeRemoved":true}`, 1)
		fmt.Fprint(w, one)
	})
	d.mux.HandleFunc("/api/v1/watches/1/activity", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("since") == "1" {
			fmt.Fprint(w, `{"activity":[]}`)
			return
		}
		fmt.Fprintf(w, `{"activity":%s}`, d.activity)
	})
	d.mux.HandleFunc("/api/v1/watches/1/send", func(w http.ResponseWriter, r *http.Request) {
		var body map[string]string
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body["message"] == "busy" {
			w.WriteHeader(http.StatusConflict)
			fmt.Fprint(w, `{"error":{"code":"agent_busy","message":"the agent waits on the author"}}`)
			return
		}
		d.sent = append(d.sent, body["message"])
		w.WriteHeader(http.StatusCreated)
		fmt.Fprintf(w, `{"id":3,"watchId":1,"kind":"nudged","ref":"0","at":"2026-09-07T12:04:00Z","actor":"","summary":"you told the agent: %s","url":"","payload":{},"reported":true}`, body["message"])
	})
	d.mux.HandleFunc("/api/v1/watches/1/poll", func(w http.ResponseWriter, r *http.Request) {
		d.polls++
		w.WriteHeader(http.StatusAccepted)
		fmt.Fprint(w, `{"accepted":true}`)
	})
	d.mux.HandleFunc("/api/v1/watches/1/next", func(w http.ResponseWriter, r *http.Request) {
		d.waits = append(d.waits, r.URL.Query().Get("wait"))
		one := strings.TrimSuffix(strings.TrimPrefix(d.watches, "["), "]")
		one = strings.Replace(one, `"session":{"state":"idle","pid":4242`, `"provider":"self","session":{"state":"none","pid":0`, 1)
		if len(d.waits) == 1 {
			fmt.Fprintf(w, `{"watch":%s,"message":{"id":5,"watchId":1,"kind":"nudged","ref":"2@t","at":"2026-09-07T12:04:00Z","actor":"","summary":"told the agent about 1 comment","url":"","payload":{"message":"bob asks for a test\n"},"reported":true}}`, one)
			return
		}
		fmt.Fprintf(w, `{"watch":%s}`, one)
	})
	d.mux.HandleFunc("/api/v1/watches/1/reply", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			InReplyTo int64  `json:"inReplyTo"`
			Body      string `json:"body"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		d.replies = append(d.replies, fmt.Sprintf("%d:%s", body.InReplyTo, body.Body))
		w.WriteHeader(http.StatusCreated)
		if !d.replyPosted {
			fmt.Fprint(w, `{"proposal":1}`)
			return
		}
		fmt.Fprintf(w, `{"posted":{"id":4,"watchId":1,"kind":"replied","ref":"40","at":"2026-09-07T12:04:00Z","actor":"alice","summary":"the agent commented: %s","url":"https://github.com/octo/hello/pull/3#issuecomment-40","payload":{},"reported":true}}`, body.Body)
	})
	d.mux.HandleFunc("POST /api/v1/watches/1/proposals/{number}/retry", func(w http.ResponseWriter, r *http.Request) {
		d.retries = append(d.retries, r.PathValue("number"))
		switch r.PathValue("number") {
		case "1":
			fmt.Fprint(w, `{"number":1,"status":"released","headSha":"abcdef1234567","baseSha":"abcdef1234567","workSha":"1a2b3c4d5e6f7","hasPush":true,"openedAt":"2026-09-07T12:04:00Z"}`)
		case "2":
			fmt.Fprint(w, `{"number":2,"status":"failed","headSha":"abcdef1234567","baseSha":"abcdef1234567","workSha":"1a2b3c4d5e6f7","hasPush":true,"openedAt":"2026-09-07T12:04:00Z","error":"push proposal 2: the pull request branch moved, so the lease refused the push"}`)
		default:
			w.WriteHeader(http.StatusConflict)
			fmt.Fprint(w, `{"error":{"code":"nothing_to_retry","message":"only a failed proposal can be retried: proposal 3 is released"}}`)
		}
	})
	d.mux.HandleFunc("/api/v1/watches/1/output", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, `{"output":"prompt ❯ (%s lines)\n"}`, r.URL.Query().Get("lines"))
	})
	d.mux.HandleFunc("/api/v1/watches/1/hook", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Event   string          `json:"event"`
			Payload json.RawMessage `json:"payload"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		d.hooks = append(d.hooks, fmt.Sprintf("1 %s %s", body.Event, body.Payload))
		w.WriteHeader(http.StatusNoContent)
	})
	d.mux.HandleFunc("/api/v1/watches/9/hook", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		fmt.Fprint(w, `{"error":{"code":"watch_not_found","message":"watch not found"}}`)
	})
	d.mux.HandleFunc("/api/v1/watches/9", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		fmt.Fprint(w, `{"error":{"code":"watch_not_found","message":"watch not found"}}`)
	})
	return d
}

func runWatch(t *testing.T, d *fakeDaemon, args ...string) (string, error) {
	t.Helper()
	return runAgainstDaemon(t, d, "watch", args...)
}

func runAgainstDaemon(t *testing.T, d *fakeDaemon, group string, args ...string) (string, error) {
	t.Helper()
	return runAgainstDaemonWith(t, d, nil, group, args...)
}

func runAgainstDaemonWith(t *testing.T, d *fakeDaemon, opts []Option, group string, args ...string) (string, error) {
	t.Helper()
	run := runInTerminal(t, d, opts, "", group, args...)
	return run.out, run.err
}

type terminalRun struct {
	out    string
	errOut string
	err    error
}

func runInTerminal(t *testing.T, d *fakeDaemon, opts []Option, stdin, group string, args ...string) terminalRun {
	t.Helper()
	srv := httptest.NewServer(d.mux)
	t.Cleanup(srv.Close)
	var port int
	fmt.Sscanf(strings.TrimPrefix(srv.URL, "http://127.0.0.1:"), "%d", &port)
	dataDir := t.TempDir()
	if err := runfile.Write(runfile.Path(dataDir), runfile.Info{PID: os.Getpid(), Port: port, Owner: runfile.OwnerCLI}); err != nil {
		t.Fatal(err)
	}
	var out, errOut bytes.Buffer
	root := NewRootCmd(opts...)
	root.SetIn(strings.NewReader(stdin))
	root.SetOut(&out)
	root.SetErr(&errOut)
	root.SetArgs(append([]string{group, "--data-dir", dataDir}, args...))
	err := root.ExecuteContext(context.Background())
	return terminalRun{out: out.String(), errOut: errOut.String(), err: err}
}

func TestWatchCommands(t *testing.T) {
	t.Parallel()
	d := newFakeDaemon()

	out, err := runWatch(t, d, "start", "octo/hello#3")
	if err != nil || !strings.Contains(out, "Watching octo/hello#3 (fix) as watch 1") || !strings.Contains(out, "Worktree: /data/worktrees/octo-hello-3") {
		t.Fatalf("start = %q, %v", out, err)
	}
	if len(d.starts) != 1 || d.starts[0]["target"] != "octo/hello#3" || d.starts[0]["sourceDir"] == "" {
		t.Fatalf("start body = %v", d.starts)
	}
	if _, ok := d.starts[0]["includeExisting"]; ok {
		t.Fatalf("start body carries includeExisting without the flag: %v", d.starts[0])
	}
	if _, err := runWatch(t, d, "start", "octo/hello#4", "--include-existing"); err == nil || !strings.Contains(err.Error(), "already watched as watch 7") {
		t.Fatalf("duplicate start error = %v", err)
	}
	if d.starts[1]["includeExisting"] != true {
		t.Fatalf("include existing not sent: %v", d.starts[1])
	}
	if _, err := runWatch(t, d, "start", "octo/hello#3", "--no-checkout"); err != nil {
		t.Fatalf("start --no-checkout = %v", err)
	}
	if _, ok := d.starts[2]["sourceDir"]; ok {
		t.Fatalf("start --no-checkout sent the current folder: %v", d.starts[2])
	}
	if _, err := runWatch(t, d, "start", "nonsense"); err == nil {
		t.Fatal("bad target expected an error before the daemon call")
	}

	out, err = runWatch(t, d, "list")
	if err != nil || !strings.Contains(out, "ID") || !strings.Contains(out, "octo/hello#3") || !strings.Contains(out, "abcdef1") || !strings.Contains(out, "green") {
		t.Fatalf("list = %q, %v", out, err)
	}
	out, err = runWatch(t, d, "-o", "json", "list")
	var l struct {
		Watches []struct{ ID int64 } `json:"watches"`
	}
	if err != nil || json.Unmarshal([]byte(out), &l) != nil || len(l.Watches) != 1 || l.Watches[0].ID != 1 {
		t.Fatalf("list json = %q, %v", out, err)
	}

	out, err = runWatch(t, d, "status", "1")
	if err != nil || !strings.Contains(out, "PR:        octo/hello#3 Fix the thing") || !strings.Contains(out, "Checks:    green") {
		t.Fatalf("status = %q, %v", out, err)
	}
	out, err = runWatch(t, d, "status", "https://github.com/octo/hello/pull/3")
	if err != nil || !strings.Contains(out, "Watch:     1") {
		t.Fatalf("status by url = %q, %v", out, err)
	}
	if _, err := runWatch(t, d, "status", "octo/hello#8"); err == nil || !strings.Contains(err.Error(), "no watch for octo/hello#8") {
		t.Fatalf("status unknown = %v", err)
	}
	if _, err := runWatch(t, d, "status", "9"); err == nil || !strings.Contains(err.Error(), "watch not found") {
		t.Fatalf("status missing id = %v", err)
	}

	out, err = runWatch(t, d, "activity", "1")
	if err != nil || !strings.Contains(out, "watch_started") || !strings.Contains(out, "bob commented: hi") {
		t.Fatalf("activity = %q, %v", out, err)
	}
	out, err = runWatch(t, d, "activity", "1", "--since", "1")
	if err != nil || !strings.Contains(out, "No activity") {
		t.Fatalf("activity since = %q, %v", out, err)
	}

	out, err = runWatch(t, d, "stop", "octo/hello#3")
	if err != nil || !strings.Contains(out, "Status:    stopped (user)") || !strings.Contains(out, "3 messages to the agent") {
		t.Fatalf("stop = %q, %v (stops %v)", out, err, d.stops)
	}
	if !strings.Contains(out, "worktree removed from /data/worktrees/octo-hello-3") {
		t.Fatalf("stop said nothing about the worktree: %q", out)
	}
	if _, err := runWatch(t, d, "stop", "octo/hello#3", "--keep-worktree"); err != nil {
		t.Fatalf("stop --keep-worktree = %v", err)
	}
	if want := []string{"POST keepWorktree=<nil>", "POST keepWorktree=true"}; !slices.Equal(d.stops, want) {
		t.Fatalf("stops = %v, want %v", d.stops, want)
	}
}

func TestWatchWithoutDaemon(t *testing.T) {
	t.Parallel()
	var out bytes.Buffer
	root := NewRootCmd()
	root.SetOut(&out)
	root.SetErr(&bytes.Buffer{})
	root.SetArgs([]string{"watch", "--data-dir", t.TempDir(), "list"})
	err := root.ExecuteContext(context.Background())
	if err == nil || !strings.Contains(err.Error(), "no daemon is running") {
		t.Fatalf("list without daemon = %v", err)
	}
}

func TestSessionCommands(t *testing.T) {
	t.Parallel()
	d := newFakeDaemon()

	out, err := runWatch(t, d, "send", "octo/hello#3", "look", "at", "the", "tests")
	if err != nil || !strings.Contains(out, "Sent to the agent of watch 1: you told the agent: look at the tests") {
		t.Fatalf("send = %q, %v", out, err)
	}
	if len(d.sent) != 1 || d.sent[0] != "look at the tests" {
		t.Fatalf("sent = %v", d.sent)
	}
	if _, err := runWatch(t, d, "send", "1"); err == nil {
		t.Fatal("send without a message succeeded")
	}
	if _, err := runWatch(t, d, "send", "1", "busy"); err == nil || !strings.Contains(err.Error(), "waits on the author") {
		t.Fatalf("send to a blocked agent = %v", err)
	}

	out, err = runWatch(t, d, "output", "1", "--lines", "3")
	if err != nil || out != "prompt ❯ (3 lines)\n" {
		t.Fatalf("output = %q, %v", out, err)
	}
	out, err = runWatch(t, d, "-o", "json", "output", "1")
	var so struct {
		Output string `json:"output"`
	}
	if err != nil || json.Unmarshal([]byte(out), &so) != nil || so.Output != "prompt ❯ (200 lines)\n" {
		t.Fatalf("output json = %q, %v", out, err)
	}

	out, err = runWatch(t, d, "status", "1")
	if err != nil || !strings.Contains(out, "Agent:     idle (pid 4242)") {
		t.Fatalf("status = %q, %v", out, err)
	}
	out, err = runWatch(t, d, "list")
	if err != nil || !strings.Contains(out, "AGENT") || !strings.Contains(out, "idle") {
		t.Fatalf("list = %q, %v", out, err)
	}
}

func TestNextAndPollCommands(t *testing.T) {
	t.Parallel()
	d := newFakeDaemon()

	out, err := runWatch(t, d, "next", "1", "--wait", "5m")
	if err != nil || !strings.Contains(out, "Message 5 for watch 1:\n\nbob asks for a test\n") {
		t.Fatalf("next = %q, %v", out, err)
	}
	out, err = runWatch(t, d, "next", "octo/hello#3")
	if err != nil || !strings.Contains(out, "Nothing to do on watch 1: head abcdef1, checks") {
		t.Fatalf("next without a message = %q, %v", out, err)
	}
	if len(d.waits) != 2 || d.waits[0] != "5m0s" || d.waits[1] != "0s" {
		t.Fatalf("waits = %v", d.waits)
	}
	out, err = runWatch(t, d, "-o", "json", "next", "1")
	var nm struct {
		Watch   struct{ ID int64 }
		Message *struct{ ID int64 }
	}
	if err != nil || json.Unmarshal([]byte(out), &nm) != nil || nm.Watch.ID != 1 || nm.Message != nil {
		t.Fatalf("next json = %q, %v", out, err)
	}
	if strings.Contains(out, `"message"`) {
		t.Fatalf("the answer without a message carries the key: %q", out)
	}
	if _, err := runWatch(t, d, "next", "1", "--wait", "-1s"); err == nil {
		t.Fatal("a negative wait was taken")
	}
	if _, err := runWatch(t, d, "next", "1", "--wait", "30m"); err != nil || d.waits[len(d.waits)-1] != "10m0s" {
		t.Fatalf("a wait past the cap of the daemon was sent as is: %v, %v", d.waits, err)
	}

	out, err = runWatch(t, d, "poll", "1")
	if err != nil || !strings.Contains(out, "Polling watch 1 now") || d.polls != 1 {
		t.Fatalf("poll = %q, %v, polls %d", out, err, d.polls)
	}
	out, err = runWatch(t, d, "-o", "json", "poll", "1")
	if err != nil || !strings.Contains(out, `"accepted": true`) || !strings.Contains(out, `"watchId": 1`) {
		t.Fatalf("poll json = %q, %v", out, err)
	}
}

func TestHookCommand(t *testing.T) {
	t.Parallel()
	d := newFakeDaemon()
	srv := httptest.NewServer(d.mux)
	t.Cleanup(srv.Close)
	var port int
	fmt.Sscanf(strings.TrimPrefix(srv.URL, "http://127.0.0.1:"), "%d", &port)
	dataDir := t.TempDir()
	if err := runfile.Write(runfile.Path(dataDir), runfile.Info{PID: os.Getpid(), Port: port, Owner: runfile.OwnerCLI}); err != nil {
		t.Fatal(err)
	}
	run := func(stdin string, args ...string) (string, error) {
		var out, errOut bytes.Buffer
		root := NewRootCmd()
		root.SetOut(&out)
		root.SetErr(&errOut)
		root.SetIn(strings.NewReader(stdin))
		root.SetArgs(append([]string{"watch", "--data-dir", dataDir, "hook"}, args...))
		err := root.ExecuteContext(context.Background())
		return errOut.String(), err
	}
	if errOut, err := run(`{"notification_type":"idle_prompt"}`, "notification", "--watch", "1"); err != nil || errOut != "" {
		t.Fatalf("hook = %q, %v", errOut, err)
	}
	if errOut, err := run("", "stop", "--watch", "1"); err != nil || errOut != "" {
		t.Fatalf("hook without payload = %q, %v", errOut, err)
	}
	if want := []string{`1 notification {"notification_type":"idle_prompt"}`, `1 stop {}`}; !slices.Equal(d.hooks, want) {
		t.Fatalf("hooks = %v, want %v", d.hooks, want)
	}
	if errOut, err := run("", "stop"); err != nil || !strings.Contains(errOut, "a watch is required") {
		t.Fatalf("hook without a watch = %q, %v", errOut, err)
	}
	if errOut, err := run("", "stop", "--watch", "9"); err != nil || !strings.Contains(errOut, "watch not found") {
		t.Fatalf("hook of a missing watch = %q, %v", errOut, err)
	}
	var out, errOut bytes.Buffer
	root := NewRootCmd()
	root.SetOut(&out)
	root.SetErr(&errOut)
	root.SetArgs([]string{"watch", "--data-dir", t.TempDir(), "hook", "stop", "--watch", "1"})
	if err := root.ExecuteContext(context.Background()); err != nil || !strings.Contains(errOut.String(), "no daemon is running") {
		t.Fatalf("hook without a daemon = %q, %v", errOut.String(), err)
	}
}

func TestWatchMergeCommand(t *testing.T) {
	t.Parallel()
	d := newFakeDaemon()
	var merges []string
	ready := false
	d.mux.HandleFunc("/api/v1/watches/1/merge", func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		merges = append(merges, fmt.Sprintf("%s method=%v", r.Method, body["method"]))
		if !ready {
			w.WriteHeader(http.StatusConflict)
			fmt.Fprint(w, `{"error":{"code":"not_ready","message":"the pull request is not ready to merge: no approval yet; 2 checks pending"}}`)
			return
		}
		one := strings.TrimSuffix(strings.TrimPrefix(d.watches, "["), "]")
		one = strings.Replace(one, `"status":"active","stopReason":""`, `"status":"stopped","stopReason":"merged","summary":{"prState":"merged","headSha":"abcdef1234567","checks":"green","mergeableState":"clean","activity":{},"messages":1,"reason":"merged","detail":"squash","worktreeRemoved":true}`, 1)
		fmt.Fprint(w, one)
	})

	out, err := runWatch(t, d, "start", "octo/hello#3", "--approvals", "2", "--merge-method", "rebase")
	if err != nil || !strings.Contains(out, "as watch 1") {
		t.Fatalf("start = %q, %v", out, err)
	}
	if d.starts[0]["approvalsRequired"] != float64(2) || d.starts[0]["mergeMethod"] != "rebase" {
		t.Fatalf("start body = %v", d.starts[0])
	}

	out, err = runWatch(t, d, "status", "1")
	if err != nil || !strings.Contains(out, "Merge:     not assessed yet") {
		t.Fatalf("status = %q, %v", out, err)
	}
	d.watches = strings.Replace(d.watches, `"prState":"open"`, `"readyBlockers":["no approval yet","2 checks pending"],"prState":"open"`, 1)
	out, err = runWatch(t, d, "status", "1")
	if err != nil || !strings.Contains(out, "Merge:     not ready: no approval yet; 2 checks pending") {
		t.Fatalf("status blocked = %q, %v", out, err)
	}
	d.watches = strings.Replace(d.watches, `"readyBlockers":["no approval yet","2 checks pending"],`, `"readySince":"2026-09-07T12:06:00Z","readyBlockers":[],"mergeMethod":"squash",`, 1)
	out, err = runWatch(t, d, "status", "1")
	if err != nil || !strings.Contains(out, "Merge:     ready since 2026-09-07T12:06:00Z, squash; run `babysitter watch merge 1`") {
		t.Fatalf("status ready = %q, %v", out, err)
	}

	_, err = runWatch(t, d, "merge", "octo/hello#3")
	if err == nil || err.Error() != "the pull request is not ready to merge:\n  no approval yet\n  2 checks pending" {
		t.Fatalf("merge while blocked error = %q", err)
	}
	ready = true
	out, err = runWatch(t, d, "merge", "1", "--method", "merge")
	if err != nil || !strings.Contains(out, "Status:    stopped (merged)") || !strings.Contains(out, "Summary:   merged at abcdef1") {
		t.Fatalf("merge = %q, %v", out, err)
	}
	if want := []string{"POST method=<nil>", "POST method=merge"}; !slices.Equal(merges, want) {
		t.Fatalf("merges = %v, want %v", merges, want)
	}
}

func TestWatchReplyCommand(t *testing.T) {
	t.Parallel()
	d := newFakeDaemon()

	out, err := runWatch(t, d, "reply", "octo/hello#3", "--to", "31", "done,", "see", "1a2b3c")
	if err != nil || out != "Recorded the reply of watch 1 in proposal 1. The daemon posts it when your turn ends.\n" {
		t.Fatalf("reply = %q, %v", out, err)
	}
	d.replyPosted = true
	out, err = runWatch(t, d, "reply", "1", "on", "the", "pull", "request")
	if err != nil || !strings.Contains(out, "Posted on the pull request of watch 1: https://github.com/octo/hello/pull/3#issuecomment-40") {
		t.Fatalf("comment = %q, %v", out, err)
	}
	if len(d.replies) != 2 || d.replies[0] != "31:done, see 1a2b3c" || d.replies[1] != "0:on the pull request" {
		t.Fatalf("replies = %v", d.replies)
	}
	if _, err := runWatch(t, d, "reply", "1"); err == nil {
		t.Fatal("reply without a text succeeded")
	}
}

func TestWatchRetryCommand(t *testing.T) {
	t.Parallel()
	d := newFakeDaemon()

	out, err := runWatch(t, d, "retry", "octo/hello#3", "1")
	if err != nil || out != "Proposal 1 of watch 1 went out: 1a2b3c4 is on fix.\n" {
		t.Fatalf("retry = %q, %v", out, err)
	}
	out, err = runWatch(t, d, "retry", "1", "2")
	if err == nil || !strings.Contains(err.Error(), "proposal 2 of watch 1 failed again: push proposal 2: the pull request branch moved") {
		t.Fatalf("a retry that failed = %q, %v", out, err)
	}
	if _, err := runWatch(t, d, "retry", "1", "3"); err == nil || !strings.Contains(err.Error(), "only a failed proposal can be retried") {
		t.Fatalf("a retry of a released proposal error = %v", err)
	}
	if _, err := runWatch(t, d, "retry", "1", "x"); err == nil {
		t.Fatal("a retry of proposal x succeeded")
	}
	if !slices.Equal(d.retries, []string{"1", "2", "3"}) {
		t.Fatalf("retries = %v", d.retries)
	}
}
