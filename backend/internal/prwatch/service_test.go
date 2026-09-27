package prwatch

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/go-github/v91/github"

	"github.com/deividfortuna/babysitter/internal/agent"
	"github.com/deividfortuna/babysitter/internal/ghclient/ghfake"
	"github.com/deividfortuna/babysitter/internal/gitrelease"
	"github.com/deividfortuna/babysitter/internal/notify"
	"github.com/deividfortuna/babysitter/internal/session"
	"github.com/deividfortuna/babysitter/internal/snapshot"
	"github.com/deividfortuna/babysitter/internal/store"
	"github.com/deividfortuna/babysitter/internal/testutil"
	"github.com/deividfortuna/babysitter/internal/worktree"
)

// newGitHub is the GitHub of every fixture: pull request 3 of octo/hello,
// open and green, into main whose rules ask for one approval.
func newGitHub() (*ghfake.GitHub, *ghfake.Repo, *ghfake.PR) {
	g := ghfake.New()
	repo := g.Repo("octo/hello")
	repo.Approvals["main"] = 1
	pr := g.PR("octo/hello", 3)
	pr.CheckRuns = []ghfake.CheckRun{check(1, "build", "success")}
	other := g.PR("octo/hello", 9)
	other.HeadRef, other.HeadSHA = "other", "o9"
	other.ReviewComments = []ghfake.ReviewComment{{ID: 555, Author: "bob", Body: "a review comment"}}
	other.IssueComments = []ghfake.Comment{{ID: 556, Author: "bob", Body: "a comment on the pull request"}}
	return g, repo, pr
}

// oldReviewComment gives the pull request review comment 31 of bob, from
// before any watch, for a test that replies to it.
func (fx *fixture) oldReviewComment() {
	fx.update(func() {
		fx.pr.ReviewComments = append(fx.pr.ReviewComments, ghfake.ReviewComment{
			ID: 31, Author: "bob", Body: "rename this", CreatedAt: ghfake.At("2026-09-01T00:00:00Z"), URL: "https://c/31",
			Path: "x.go", Line: 4,
		})
	})
}

// newestComment is the comment GitHub made last, for a test that looks for
// what the service posted.
func (fx *fixture) newestComment() ghfake.Comment {
	var out ghfake.Comment
	fx.update(func() {
		for _, c := range fx.pr.IssueComments {
			if c.ID > out.ID {
				out = c
			}
		}
		for _, c := range fx.pr.ReviewComments {
			if c.ID > out.ID {
				out = c.Comment
			}
		}
	})
	return out
}

// openPR gives GitHub one more pull request like number 3.
func (fx *fixture) openPR(n int) {
	pr := fx.api.PR("octo/hello", n)
	fx.update(func() { pr.CheckRuns = slices.Clone(fx.pr.CheckRuns) })
}

// setJobs gives the failed CI run these jobs, which print log.
func (fx *fixture) setJobs(log string, jobs ...*ghfake.Job) {
	fx.update(func() { fx.repo.Runs = []*ghfake.Run{ciRun(log, jobs...)} })
}

// logStatus makes every job log answer status.
func (fx *fixture) logStatus(status int) {
	fx.update(func() {
		for _, run := range fx.repo.Runs {
			for _, j := range run.Jobs {
				j.LogStatus = status
			}
		}
	})
}

// check is a completed check run of the CI suite.
func check(id int64, name, conclusion string) ghfake.CheckRun {
	return ghfake.CheckRun{ID: id, Name: name, Status: "completed", Conclusion: conclusion, CheckSuiteID: 501}
}

// failedJob is a failed job of the CI run.
func failedJob(id int64, name string) *ghfake.Job {
	return &ghfake.Job{ID: id, Name: name, Status: "completed", Conclusion: "failure", URL: fmt.Sprintf("https://ci/77/%d", id)}
}

// ciRun is the failed CI run of the head, whose jobs all print log.
func ciRun(log string, jobs ...*ghfake.Job) *ghfake.Run {
	for _, j := range jobs {
		j.Log = log
	}
	return &ghfake.Run{ID: 77, Name: "CI", CheckSuiteID: 501, Status: "completed", Conclusion: "failure", URL: "https://ci/77", Jobs: jobs}
}

// update changes GitHub while the service may be calling it.
func (fx *fixture) update(fn func()) {
	fx.api.Update(fn)
}

// failBuild turns the build red. The failed job prints log, and with an
// empty log GitHub lists no workflow run for it.
func (fx *fixture) failBuild(log string) {
	fx.update(func() {
		fx.pr.CheckRuns = []ghfake.CheckRun{check(1, "build", "failure")}
		fx.repo.Runs = nil
		if log != "" {
			fx.repo.Runs = []*ghfake.Run{ciRun(log, failedJob(9, "build"))}
		}
	})
}

// gone makes every call answer 404 with message, the way GitHub answers a
// token that lost its access, and an empty message ends it.
func (fx *fixture) gone(message string) {
	fx.goneMu.Lock()
	defer fx.goneMu.Unlock()
	fx.goneMessage = message
}

func (fx *fixture) goneReactor(ghfake.Action) (ghfake.Response, bool) {
	fx.goneMu.Lock()
	defer fx.goneMu.Unlock()
	return ghfake.Response{Status: http.StatusNotFound, Message: fx.goneMessage}, fx.goneMessage != ""
}

// failReplies makes a reply in the thread of one of the comments answer
// 502.
func (fx *fixture) failReplies(ids ...int64) {
	fx.api.React(ghfake.RouteReply, func(a ghfake.Action) (ghfake.Response, bool) {
		var body struct {
			InReplyTo int64 `json:"in_reply_to"`
		}
		a.Decode(fx.t, &body)
		return ghfake.Response{Status: http.StatusBadGateway, Message: "Bad Gateway"}, slices.Contains(ids, body.InReplyTo)
	})
}

// posted lists the comments GitHub took from the service, oldest first,
// as in_reply_to:body with 0 for a comment on the conversation.
func (fx *fixture) posted() []string {
	var out []string
	for _, a := range fx.api.Actions() {
		if (a.Route != ghfake.RouteReply && a.Route != ghfake.RouteComment) || a.Status >= http.StatusBadRequest {
			continue
		}
		var body struct {
			InReplyTo int64  `json:"in_reply_to"`
			Body      string `json:"body"`
		}
		a.Decode(fx.t, &body)
		out = append(out, fmt.Sprintf("%d:%s", body.InReplyTo, body.Body))
	}
	return out
}

// merges returns the merges GitHub received.
func (fx *fixture) merges() []ghfake.Merge {
	var out []ghfake.Merge
	fx.update(func() { out = slices.Clone(fx.pr.Merges) })
	return out
}

type fakeGit struct {
	mu        sync.Mutex
	fetches   []string
	created   []string
	removed   []string
	removeErr error
	fetchErr  error
	onFetch   func()
	onCreate  func()
}

func (g *fakeGit) Fetch(_ context.Context, _, upstream string) error {
	g.mu.Lock()
	g.fetches = append(g.fetches, upstream)
	hook, err := g.onFetch, g.fetchErr
	g.mu.Unlock()
	if hook != nil {
		hook()
	}
	return err
}

func (g *fakeGit) fetched() []string {
	g.mu.Lock()
	defer g.mu.Unlock()
	return append([]string(nil), g.fetches...)
}

func (g *fakeGit) Create(_ context.Context, source, dir, branch, upstream string) error {
	g.mu.Lock()
	g.created = append(g.created, dir+" "+branch+" "+upstream)
	hook := g.onCreate
	g.mu.Unlock()
	if hook != nil {
		hook()
	}
	return nil
}

func (g *fakeGit) Remove(_ context.Context, _, dir, _ string) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.removeErr != nil {
		return g.removeErr
	}
	g.removed = append(g.removed, dir)
	return nil
}

func (g *fakeGit) createdDirs() []string {
	g.mu.Lock()
	defer g.mu.Unlock()
	out := make([]string, 0, len(g.created))
	for _, c := range g.created {
		out = append(out, strings.Fields(c)[0])
	}
	return out
}

func (g *fakeGit) removedDirs() []string {
	g.mu.Lock()
	defer g.mu.Unlock()
	return append([]string(nil), g.removed...)
}

const meetWait = 2 * time.Second

type meeting struct {
	mu     sync.Mutex
	absent map[string]bool
	all    chan struct{}
	alone  []string
}

func newMeeting(parties ...string) *meeting {
	m := &meeting{absent: map[string]bool{}, all: make(chan struct{})}
	for _, p := range parties {
		m.absent[p] = true
	}
	return m
}

func (m *meeting) arrive(party string) {
	m.mu.Lock()
	if m.absent[party] {
		delete(m.absent, party)
		if len(m.absent) == 0 {
			close(m.all)
		}
	}
	m.mu.Unlock()
	select {
	case <-m.all:
	case <-time.After(meetWait):
		m.mu.Lock()
		m.alone = append(m.alone, party)
		m.mu.Unlock()
	}
}

func (m *meeting) missed() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := slices.Clone(m.alone)
	for p := range m.absent {
		out = append(out, p+" (never came)")
	}
	slices.Sort(out)
	return out
}

type fakeRelease struct {
	mu          sync.Mutex
	remote      string
	work        string
	history     map[string][]string
	missing     []string
	dirty       []string
	merges      []string
	pushErr     error
	fetchErr    error
	rebaseErr   error
	logErr      error
	filesErr    error
	diffErr     error
	duringFetch func()
	pushes      []gitrelease.Push
	ffs         []string
	resets      []string
	rebases     []string
	discards    []string
}

func newFakeRelease(head string) *fakeRelease {
	return &fakeRelease{remote: head, work: head, history: map[string][]string{}}
}

func (f *fakeRelease) commit(parent, child string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.history[child] = append([]string{parent}, f.history[parent]...)
	f.work = child
}

func (f *fakeRelease) moveRemote(parent, sha string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.history[sha] = append([]string{parent}, f.history[parent]...)
	f.remote = sha
}

func (f *fakeRelease) set(fn func(f *fakeRelease)) {
	f.mu.Lock()
	defer f.mu.Unlock()
	fn(f)
}

func (f *fakeRelease) contains(sha, ancestor string) bool {
	return sha == ancestor || slices.Contains(f.history[sha], ancestor)
}

func (f *fakeRelease) Fetch(context.Context, string, string) (string, error) {
	f.mu.Lock()
	during := f.duringFetch
	f.duringFetch = nil
	f.mu.Unlock()
	if during != nil {
		during()
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.remote, f.fetchErr
}

func (f *fakeRelease) Head(context.Context, string) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.work, nil
}

func (f *fakeRelease) Contains(_ context.Context, _, sha, ancestor string) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.contains(sha, ancestor), nil
}

func (f *fakeRelease) MergeBase(_ context.Context, _, a, b string) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, c := range append([]string{a}, f.history[a]...) {
		if f.contains(b, c) {
			return c, nil
		}
	}
	return "", errors.New("no merge base")
}

func (f *fakeRelease) FastForward(_ context.Context, _, sha string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if !f.contains(sha, f.work) {
		return errors.New("not a fast-forward")
	}
	f.ffs = append(f.ffs, sha)
	f.work = sha
	return nil
}

func (f *fakeRelease) Reset(_ context.Context, _, sha string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.resets = append(f.resets, sha)
	f.work = sha
	return nil
}

func (f *fakeRelease) HasMerges(_ context.Context, _, since, sha string) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, c := range append([]string{sha}, f.history[sha]...) {
		if !f.contains(since, c) && slices.Contains(f.merges, c) {
			return true, nil
		}
	}
	return false, nil
}

func (f *fakeRelease) Missing(context.Context, string, string, string, string) ([]string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.missing, nil
}

func (f *fakeRelease) Push(_ context.Context, _ string, p gitrelease.Push) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.pushErr != nil {
		return f.pushErr
	}
	if p.Lease != "" && p.Lease != f.remote {
		return gitrelease.ErrLeaseRefused
	}
	f.pushes = append(f.pushes, p)
	f.remote = p.SHA
	return nil
}

func (f *fakeRelease) Rebase(_ context.Context, _, onto string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.rebaseErr != nil {
		return f.rebaseErr
	}
	f.rebases = append(f.rebases, onto)
	rebased := f.work + "-on-" + onto
	f.history[rebased] = append([]string{onto}, f.history[onto]...)
	f.work = rebased
	f.missing = nil
	return nil
}

func (f *fakeRelease) Discard(_ context.Context, _, sha string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.discards = append(f.discards, sha)
	f.work = sha
	return nil
}

func (f *fakeRelease) Log(_ context.Context, _, from, to string) ([]gitrelease.Commit, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.logErr != nil {
		return nil, f.logErr
	}
	var out []gitrelease.Commit
	for _, c := range append([]string{to}, f.history[to]...) {
		if f.contains(from, c) {
			break
		}
		out = append([]gitrelease.Commit{{SHA: c, Subject: "commit " + c}}, out...)
	}
	return out, nil
}

func (f *fakeRelease) Files(context.Context, string, string, string) ([]gitrelease.File, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.filesErr != nil {
		return nil, f.filesErr
	}
	return []gitrelease.File{{Path: "x.go", Status: "M", Added: 3, Deleted: 1}}, nil
}

func (f *fakeRelease) Diff(_ context.Context, _, from, to string, _ int) (string, bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.diffErr != nil {
		return "", false, f.diffErr
	}
	return "diff --git a/x.go b/x.go\n-" + from + "\n+" + to + "\n", false, nil
}

func (f *fakeRelease) Dirty(context.Context, string) ([]string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.dirty), nil
}

func (f *fakeRelease) pushed() []gitrelease.Push {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]gitrelease.Push(nil), f.pushes...)
}

type fakeNotifier struct {
	mu     sync.Mutex
	items  []notify.Item
	onSend func()
}

func (n *fakeNotifier) Post(_ context.Context, item notify.Item) (store.Notification, error) {
	n.mu.Lock()
	n.items = append(n.items, item)
	hook := n.onSend
	n.mu.Unlock()
	if hook != nil {
		hook()
	}
	return store.Notification{ID: 1, Kind: item.Kind, Title: item.Title, Body: item.Message}, nil
}

func (n *fakeNotifier) messages() []string {
	n.mu.Lock()
	defer n.mu.Unlock()
	out := make([]string, 0, len(n.items))
	for _, item := range n.items {
		out = append(out, item.Message)
	}
	return out
}

func (n *fakeNotifier) noteKinds() []string {
	n.mu.Lock()
	defer n.mu.Unlock()
	out := make([]string, 0, len(n.items))
	for _, item := range n.items {
		out = append(out, string(item.Kind))
	}
	return out
}

type fakeRunner struct {
	signals   bool
	err       error
	authorErr error
	sessionID string
}

func (r *fakeRunner) NewSessionID() string {
	if r.sessionID != "" {
		return r.sessionID
	}
	return agent.NewSessionID()
}

func (r *fakeRunner) Command(l agent.Launch) ([]string, []string, error) {
	if r.err != nil {
		return nil, nil, r.err
	}
	argv := []string{"fake-agent", "--worktree", l.WorktreeDir}
	if l.Resume {
		argv = append(argv, "--resume", l.SessionID)
	} else {
		argv = append(argv, "--session-id", l.SessionID)
	}
	if l.Model != "" {
		argv = append(argv, "--model", l.Model)
	}
	argv = append(argv, l.Hook...)
	return argv, []string{"FAKE=1"}, nil
}

func (r *fakeRunner) AuthorCommand(l agent.Launch) ([]string, error) {
	if r.authorErr != nil {
		return nil, r.authorErr
	}
	return agent.AuthorArgs("fake-agent", "", l)
}
func (r *fakeRunner) Doctor(context.Context) error { return nil }
func (r *fakeRunner) Signals() bool                { return r.signals }
func (r *fakeRunner) Prelude() string {
	if r.signals {
		return ""
	}
	return agent.SystemPrompt()
}

type fakeHandle struct {
	mu       sync.Mutex
	spec     session.Spec
	sent     []string
	sendErr  error
	done     chan struct{}
	err      error
	pid      int
	stopped  bool
	stopErr  error
	screen   string
	onSend   func()
	onResize func(TerminalSize)
	size     TerminalSize
}

func (h *fakeHandle) Send(_ context.Context, text string) error {
	h.mu.Lock()
	select {
	case <-h.done:
		h.mu.Unlock()
		return session.ErrExited
	default:
	}
	if h.sendErr != nil {
		h.mu.Unlock()
		return h.sendErr
	}
	h.sent = append(h.sent, text)
	hook := h.onSend
	h.mu.Unlock()
	if hook != nil {
		hook()
	}
	return nil
}

func (h *fakeHandle) Ready(context.Context) error { return nil }

func (h *fakeHandle) Interrupt() error { return nil }

func (h *fakeHandle) Resize(rows, cols uint16) error {
	size := TerminalSize{Rows: rows, Cols: cols}
	h.mu.Lock()
	hook := h.onResize
	h.mu.Unlock()
	if hook != nil {
		hook(size)
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	select {
	case <-h.done:
		return session.ErrExited
	default:
	}
	h.size = size
	return nil
}

func (h *fakeHandle) terminalSize() TerminalSize {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.size
}

func (h *fakeHandle) Output(int) string {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.screen + fmt.Sprintf("fake prompt, %d messages\n", len(h.sent))
}

func (h *fakeHandle) draw(text string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.screen += text
}
func (h *fakeHandle) Done() <-chan struct{} { return h.done }
func (h *fakeHandle) Err() error            { return h.err }
func (h *fakeHandle) PID() int              { return h.pid }
func (h *fakeHandle) Stop(context.Context) error {
	h.mu.Lock()
	h.stopped = true
	stopErr := h.stopErr
	h.mu.Unlock()
	if stopErr != nil {
		return stopErr
	}
	h.exit(nil)
	return nil
}

func (h *fakeHandle) exit(err error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	select {
	case <-h.done:
	default:
		h.err = err
		close(h.done)
	}
}

func (h *fakeHandle) wasStopped() bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.stopped
}

func (h *fakeHandle) messages() []string {
	h.mu.Lock()
	defer h.mu.Unlock()
	return append([]string(nil), h.sent...)
}

type fakeHost struct {
	mu       sync.Mutex
	handles  []*fakeHandle
	startErr error
}

func (f *fakeHost) Start(_ context.Context, spec session.Spec) (session.Handle, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.startErr != nil {
		return nil, f.startErr
	}
	h := &fakeHandle{spec: spec, done: make(chan struct{}), pid: 1000 + len(f.handles)}
	f.handles = append(f.handles, h)
	return h, nil
}

func (f *fakeHost) last() *fakeHandle {
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.handles) == 0 {
		return nil
	}
	return f.handles[len(f.handles)-1]
}

func (f *fakeHost) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.handles)
}

type fixture struct {
	t      *testing.T
	api    *ghfake.GitHub
	repo   *ghfake.Repo
	pr     *ghfake.PR
	st     *store.Store
	dbPath string
	git    *fakeGit
	rel    *fakeRelease
	notes  *fakeNotifier
	host   *fakeHost
	svc    *Service
	nowMu  sync.Mutex
	now    time.Time
	dir    string
	data   string
	client *github.Client

	goneMu      sync.Mutex
	goneMessage string

	authorAlive atomic.Bool
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not installed")
	}
	api, repo, pr := newGitHub()
	client := api.Client(t)
	dbPath := filepath.Join(t.TempDir(), "babysitter.db")
	st, err := store.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })

	dir := t.TempDir()
	for _, a := range [][]string{
		{"init", "-q", "-b", "fix"},
		{"remote", "add", "origin", "git@github.com:octo/hello.git"},
		{"config", "user.name", "Alice"},
		{"config", "user.email", "alice@example.com"},
		{"-c", "commit.gpgsign=false", "commit", "-q", "--allow-empty", "-m", "init"},
	} {
		cmd := exec.Command("git", a...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", a, err, out)
		}
	}

	fx := &fixture{
		t: t, api: api, repo: repo, pr: pr, st: st, dbPath: dbPath, git: &fakeGit{}, rel: newFakeRelease("abc"), notes: &fakeNotifier{}, host: &fakeHost{}, dir: dir,
		data: filepath.Join(t.TempDir(), "data"), client: client,
		now: time.Date(2026, 9, 7, 12, 0, 0, 0, time.UTC),
	}
	api.React(ghfake.AnyRoute, fx.goneReactor)
	set := store.DefaultSettings()
	set.ApprovalMode = store.ApprovalAuto
	if _, err := st.SaveSettings(context.Background(), set); err != nil {
		t.Fatal(err)
	}
	fx.svc = fx.newService()
	return fx
}

func (fx *fixture) newService() *Service {
	return New(Deps{
		Log:           testutil.Logger(fx.t),
		Store:         fx.st,
		NewClient:     func(context.Context) (*github.Client, error) { return fx.client, nil },
		Git:           fx.git,
		Release:       fx.rel,
		Notifications: fx.notes,
		Agents:        map[string]agent.Runner{ProviderClaude: &fakeRunner{signals: true}, ProviderCopilot: &fakeRunner{}},
		Host:          fx.host,
		Exe:           "/opt/babysitter",
		DataDir:       fx.data,
	}, WithClock(func() time.Time { return fx.clock() }), WithInterval(time.Minute),
		WithProcessAlive(func(int) bool { return fx.authorAlive.Load() }))
}

func (fx *fixture) start() store.Watch {
	fx.t.Helper()
	w, err := fx.svc.Start(context.Background(), StartRequest{
		Target: snapshot.Target{Owner: "octo", Name: "hello", Number: 3}, SourceDir: fx.dir,
	})
	if err != nil {
		fx.t.Fatalf("Start() error = %v", err)
	}
	return w
}

func (fx *fixture) poll(w store.Watch) {
	fx.t.Helper()
	fx.advance(time.Minute)
	if err := fx.svc.Poll(context.Background(), w.ID); err != nil {
		fx.t.Fatalf("Poll() error = %v", err)
	}
}

func (fx *fixture) activity(w store.Watch) []store.Activity {
	fx.t.Helper()
	rows, err := fx.st.ListActivity(context.Background(), w.ID, 0, 0)
	if err != nil {
		fx.t.Fatal(err)
	}
	return rows
}

func (fx *fixture) kinds(w store.Watch) []string {
	fx.t.Helper()
	var out []string
	for _, a := range fx.activity(w) {
		if !a.Reported {
			fx.t.Errorf("activity %d (%s) is not marked reported", a.ID, a.Kind)
		}
		out = append(out, string(a.Kind))
	}
	return out
}

func (fx *fixture) waitKinds(w store.Watch, want []string) {
	fx.t.Helper()
	testutil.Within(testutil.Timeout, func() bool {
		var got []string
		allReported := true
		for _, a := range fx.activity(w) {
			got = append(got, string(a.Kind))
			allReported = allReported && a.Reported
		}
		return allReported && strings.Join(got, ",") == strings.Join(want, ",")
	})
	equal(fx.t, fx.kinds(w), want)
}

func (fx *fixture) waitNote(text string) []string {
	fx.t.Helper()
	var msgs []string
	if testutil.Within(testutil.Timeout, func() bool {
		msgs = fx.notes.messages()
		return len(msgs) > 0 && strings.Contains(msgs[len(msgs)-1], text)
	}) {
		return msgs
	}
	fx.t.Fatalf("no notification about %q, got %v", text, fx.notes.messages())
	return nil
}

func equal(t *testing.T, got, want []string) {
	t.Helper()
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestStartOpensASessionAndIntroducesThePullRequest(t *testing.T) {
	t.Parallel()
	fx := newFixture(t)
	fx.update(func() {
		fx.pr.IssueComments = []ghfake.Comment{{ID: 10, Author: "bob", CreatedAt: ghfake.At("2026-09-01T00:00:00Z"), Body: "old comment", URL: "https://c/10"}}
	})
	w := fx.start()
	if w.Status != store.WatchActive || w.HeadSHA != "abc" || w.BotLogin != "alice" || w.HeadRef != "fix" || w.BaseRef != "main" ||
		w.GitUserName != "Alice" || w.GitUserEmail != "alice@example.com" || w.CheckStates["build"] != "passed" || w.WorkBranch != "babysitter/fix" {
		t.Fatalf("watch = %+v", w)
	}
	if len(fx.git.created) != 1 || !strings.HasSuffix(fx.git.created[0], "octo-hello-3 babysitter/fix origin/fix") || !strings.Contains(fx.git.created[0], "worktrees") {
		t.Fatalf("created = %v", fx.git.created)
	}
	equal(t, fx.kinds(w), []string{"watch_started", "session_started", "nudged"})
	if msgs := fx.notes.messages(); len(msgs) != 1 || !strings.Contains(msgs[0], "Watching fix") {
		t.Fatalf("notifications = %v", msgs)
	}
	equal(t, fx.notes.noteKinds(), []string{"watch"})

	h := fx.host.last()
	if h == nil {
		t.Fatal("no session started")
	}
	stored, _ := fx.st.GetWatch(context.Background(), w.ID)
	if stored.AgentSession == "" || len(stored.AgentSession) != 36 {
		t.Fatalf("agent session = %q", stored.AgentSession)
	}
	argv := strings.Join(h.spec.Argv, " ")
	if h.spec.Dir != w.WorktreeDir || !strings.Contains(argv, "--session-id "+stored.AgentSession) || strings.Contains(argv, "--resume") ||
		!strings.Contains(argv, "/opt/babysitter watch hook --data-dir "+fx.data+" --watch "+fmt.Sprint(w.ID)) {
		t.Fatalf("launch = %s in %s", argv, h.spec.Dir)
	}
	env := strings.Join(h.spec.Env, "\n")
	if !strings.Contains(env, "FAKE=1") || !strings.Contains(env, "BABYSITTER_WATCH="+fmt.Sprint(w.ID)) || !strings.Contains(env, "BABYSITTER_DATA_DIR="+fx.data) {
		t.Fatalf("env = %v", h.spec.Env)
	}
	if h.spec.LogPath != filepath.Join(fx.data, "sessions", fmt.Sprintf("%d.log", w.ID)) {
		t.Fatalf("log path = %q", h.spec.LogPath)
	}
	msgs := h.messages()
	if len(msgs) != 1 || !strings.Contains(msgs[0], "babysitting PR #3 (fix -> main) of octo/hello") || !strings.Contains(msgs[0], "every 1 minute") {
		t.Fatalf("messages = %q", msgs)
	}
	if strings.Contains(msgs[0], "Standing rules") {
		t.Fatal("the runner with a system prompt got the rules in the first message")
	}
	rows := fx.activity(w)
	if rows[2].Summary != "told the agent about the pull request" || !strings.Contains(string(rows[2].Payload), `"source":"daemon"`) {
		t.Fatalf("nudged row = %+v", rows[2])
	}
	info, err := fx.svc.Session(context.Background(), w)
	if err != nil || info.State != agent.StateStarting || info.PID != h.pid || info.AgentSession != stored.AgentSession || info.StartedAt == nil {
		t.Fatalf("session = %+v, %v", info, err)
	}

	if _, err := fx.svc.Start(context.Background(), StartRequest{Target: snapshot.Target{Owner: "octo", Name: "hello", Number: 3}, SourceDir: fx.dir}); !errors.Is(err, store.ErrWatchExists) {
		t.Fatalf("second Start() error = %v, want ErrWatchExists", err)
	}
	if fx.host.count() != 1 {
		t.Fatalf("sessions = %d", fx.host.count())
	}

	fx.poll(w)
	equal(t, fx.kinds(w), []string{"watch_started", "session_started", "nudged"})
	got, _ := fx.st.GetWatch(context.Background(), w.ID)
	if got.LastPollAt == nil || got.ConsecutiveErrors != 0 {
		t.Fatalf("watch after poll = %+v", got)
	}
}

func TestStartWithARunnerWithoutASystemPromptSendsTheRulesFirst(t *testing.T) {
	t.Parallel()
	fx := newFixture(t)
	w, err := fx.svc.Start(context.Background(), StartRequest{
		Target:    snapshot.Target{Owner: "octo", Name: "hello", Number: 3},
		SourceDir: fx.dir,
		Provider:  ProviderCopilot,
	})
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	if w.Provider != ProviderCopilot {
		t.Fatalf("watch provider = %q", w.Provider)
	}
	msgs := fx.host.last().messages()
	if len(msgs) != 1 || !strings.HasPrefix(msgs[0], "You babysit one pull request") || !strings.Contains(msgs[0], "babysitting PR #3") {
		t.Fatalf("messages = %q", msgs)
	}
	info, _ := fx.svc.Session(context.Background(), w)
	if info.State != agent.StateIdle {
		t.Fatalf("state = %q", info.State)
	}
}

func TestStartOnConflictTellsTheAgent(t *testing.T) {
	t.Parallel()
	fx := newFixture(t)
	fx.update(func() { fx.pr.MergeableState = "dirty" })
	w := fx.start()
	if w.MergeableState != "dirty" {
		t.Fatalf("watch = %+v", w)
	}
	equal(t, fx.kinds(w), []string{"watch_started", "conflict", "session_started", "nudged", "nudged"})
	msgs := fx.host.last().messages()
	if len(msgs) != 2 || !strings.Contains(msgs[1], "There are merge conflicts on PR #3") || !strings.Contains(msgs[1], "the daemon pushes it when your turn ends") {
		t.Fatalf("messages = %q", msgs)
	}
	rows := fx.activity(w)
	if rows[1].NudgedAt == nil || rows[4].Summary != "told the agent about a merge conflict" {
		t.Fatalf("rows = %+v", rows)
	}

	fx.poll(w)
	equal(t, fx.kinds(w), []string{"watch_started", "conflict", "session_started", "nudged", "nudged"})
	if len(fx.host.last().messages()) != 2 {
		t.Fatal("the conflict was told twice")
	}
}

func TestStartOnRedChecksTellsTheAgent(t *testing.T) {
	t.Parallel()
	fx := newFixture(t)
	fx.failBuild(stampedLog("##[group]Run go test ./...", "##[endgroup]", "--- FAIL: TestThing", "##[error]Process completed with exit code 1."))
	w := fx.start()
	if w.CheckStates["build"] != "failed" {
		t.Fatalf("watch = %+v", w)
	}
	equal(t, fx.kinds(w), []string{"watch_started", "check_failed", "session_started", "nudged", "nudged"})
	msgs := fx.host.last().messages()
	if len(msgs) != 2 || !strings.Contains(msgs[1], "Failed: build (failure)") || !strings.Contains(msgs[1], "--- FAIL: TestThing") {
		t.Fatalf("messages = %q", msgs)
	}
	rows := fx.activity(w)
	if rows[1].NudgedAt == nil || rows[4].Summary != "told the agent about 1 failed check" {
		t.Fatalf("rows = %+v", rows)
	}

	fx.poll(w)
	equal(t, fx.kinds(w), []string{"watch_started", "check_failed", "session_started", "nudged", "nudged"})
	if len(fx.host.last().messages()) != 2 {
		t.Fatal("the failed check was told twice")
	}

	stopped, err := fx.svc.Stop(context.Background(), w.ID, StopOptions{})
	if err != nil {
		t.Fatalf("Stop() error = %v", err)
	}
	if !strings.Contains(string(stopped.Summary), `"check_failed":1`) {
		t.Fatalf("summary = %s", stopped.Summary)
	}
}

func TestStartRejections(t *testing.T) {
	t.Parallel()
	fx := newFixture(t)
	ctx := context.Background()
	target := snapshot.Target{Owner: "octo", Name: "hello", Number: 3}

	fx.update(func() { fx.pr.State, fx.pr.Merged = "closed", true })
	if _, err := fx.svc.Start(ctx, StartRequest{Target: target, SourceDir: fx.dir}); err == nil || !strings.Contains(err.Error(), ErrNotOpen.Error()) {
		t.Fatalf("merged Start() error = %v", err)
	}
	fx.update(func() { fx.pr.State, fx.pr.Merged, fx.repo.Push = "open", false, false })
	if _, err := fx.svc.Start(ctx, StartRequest{Target: target, SourceDir: fx.dir}); err == nil || !strings.Contains(err.Error(), ErrNoPushAccess.Error()) {
		t.Fatalf("no push Start() error = %v", err)
	}
	fx.update(func() { fx.repo.Push = true })
	if _, err := fx.svc.Start(ctx, StartRequest{Target: target, SourceDir: t.TempDir()}); err == nil || !strings.Contains(err.Error(), "origin remote") {
		t.Fatalf("no repo Start() error = %v", err)
	}
	other := t.TempDir()
	for _, a := range [][]string{{"init", "-q"}, {"remote", "add", "origin", "git@github.com:someone/else.git"}} {
		cmd := exec.Command("git", a...)
		cmd.Dir = other
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", a, err, out)
		}
	}
	if _, err := fx.svc.Start(ctx, StartRequest{Target: target, SourceDir: other}); err == nil || !strings.Contains(err.Error(), ErrWrongRepo.Error()) {
		t.Fatalf("wrong repo Start() error = %v", err)
	}
	delete(fx.svc.agents, ProviderCopilot)
	if _, err := fx.svc.Start(ctx, StartRequest{Target: target, SourceDir: fx.dir, Provider: ProviderCopilot}); !errors.Is(err, ErrNoAgent) {
		t.Fatalf("Start() without the runner error = %v, want ErrNoAgent", err)
	}
	if len(fx.git.created) != 0 {
		t.Fatalf("a rejected start made a worktree: %v", fx.git.created)
	}
	if ws, _ := fx.st.ListWatches(ctx, store.ListWatchesOptions{}); len(ws) != 0 {
		t.Fatalf("watches after rejections = %v", ws)
	}
	if fx.host.count() != 0 {
		t.Fatal("a rejected start opened a session")
	}
}

func TestPollTellsTheAgentOnce(t *testing.T) {
	t.Parallel()
	fx := newFixture(t)
	w := fx.start()
	h := fx.host.last()

	fx.update(func() {
		fx.pr.IssueComments = []ghfake.Comment{{ID: 11, Author: "bob", CreatedAt: ghfake.At("2026-09-07T12:01:00Z"), Body: "Please return the error", URL: "https://c/11"}, {ID: 12, Author: "alice", CreatedAt: ghfake.At("2026-09-07T12:02:00Z"), Body: "On it", URL: "https://c/12"}}
		fx.pr.ReviewComments = []ghfake.ReviewComment{{ID: 31, Author: "bob", CreatedAt: ghfake.At("2026-09-07T12:01:00Z"), Body: "rename", URL: "https://c/31", Path: "x.go", Line: 4}}
	})
	fx.poll(w)
	equal(t, fx.kinds(w), []string{"watch_started", "session_started", "nudged", "comment", "review_comment", "nudged"})
	if msgs := fx.notes.messages(); len(msgs) != 1 {
		t.Fatalf("notifications = %v", msgs)
	}
	msgs := h.messages()
	if len(msgs) != 2 || !strings.Contains(msgs[1], "2 unresolved review comments") || !strings.Contains(msgs[1], "Please return the error") ||
		!strings.Contains(msgs[1], "x.go:4 (@bob), comment id 31") || !strings.Contains(msgs[1], "PR: https://github.com/octo/hello/pull/3") {
		t.Fatalf("messages = %q", msgs)
	}
	rows := fx.activity(w)
	if rows[3].NudgedAt == nil || rows[4].NudgedAt == nil || rows[5].Summary != "told the agent about 2 comments" || !strings.Contains(string(rows[5].Payload), `"activity_ids":[4,5]`) {
		t.Fatalf("rows = %+v", rows[3:])
	}
	fx.poll(w)
	equal(t, fx.kinds(w), []string{"watch_started", "session_started", "nudged", "comment", "review_comment", "nudged"})
	if len(h.messages()) != 2 {
		t.Fatal("the comments were told twice")
	}

	fx.update(func() {
		fx.pr.CheckRuns = []ghfake.CheckRun{{ID: 1, Name: "build", Status: "completed", Conclusion: "failure", URL: "https://ci/1"}}
	})
	fx.poll(w)
	fx.poll(w)
	fx.update(func() {
		fx.pr.CheckRuns = []ghfake.CheckRun{{ID: 1, Name: "build", Status: "completed", Conclusion: "success"}}
	})
	fx.poll(w)
	fx.update(func() {
		fx.pr.HeadSHA = "def"
		fx.pr.CheckRuns = []ghfake.CheckRun{{ID: 2, Name: "build", Status: "in_progress"}}
	})
	fx.poll(w)
	fx.update(func() {
		fx.pr.CheckRuns = []ghfake.CheckRun{{ID: 2, Name: "build", Status: "completed", Conclusion: "success"}}
	})
	fx.poll(w)
	equal(t, fx.kinds(w), []string{
		"watch_started", "session_started", "nudged", "comment", "review_comment", "nudged",
		"check_failed", "nudged", "check_recovered", "commit", "checks_green",
	})
	msgs = h.messages()
	if len(msgs) != 3 || !strings.Contains(msgs[2], "CI is failing on PR #3") || !strings.Contains(msgs[2], "Failed: build (failure)") || !strings.Contains(msgs[2], "Failure URL: https://ci/1") {
		t.Fatalf("messages = %q", msgs)
	}
	notes := fx.notes.messages()
	if len(notes) != 3 || !strings.Contains(notes[1], "build failed") || !strings.Contains(notes[2], "all 1 check passed on def") {
		t.Fatalf("notifications = %v", notes)
	}
	got, _ := fx.st.GetWatch(context.Background(), w.ID)
	if got.HeadSHA != "def" || got.GreenSHA != "def" {
		t.Fatalf("watch = %+v", got)
	}

	fx.update(func() { fx.pr.MergeableState = "behind" })
	fx.poll(w)
	msgs = h.messages()
	if len(msgs) != 4 || !strings.Contains(msgs[3], "is behind main") {
		t.Fatalf("messages = %q", msgs)
	}
	fx.update(func() { fx.pr.State, fx.pr.Merged = "closed", true })
	fx.poll(w)
	equal(t, fx.kinds(w), []string{
		"watch_started", "session_started", "nudged", "comment", "review_comment", "nudged",
		"check_failed", "nudged", "check_recovered", "commit", "checks_green", "behind", "nudged", "merged", "watch_stopped",
	})
	got, _ = fx.st.GetWatch(context.Background(), w.ID)
	if got.Status != store.WatchStopped || got.StopReason != store.StopMerged || !strings.Contains(string(got.Summary), `"headSha":"def"`) || !strings.Contains(string(got.Summary), `"messages":4`) {
		t.Fatalf("stopped watch = %+v %s", got, got.Summary)
	}
	if !strings.Contains(string(got.Summary), `"worktreeRemoved":true`) {
		t.Fatalf("stopped watch summary = %s", got.Summary)
	}
	if removed := fx.git.removedDirs(); removed[len(removed)-1] != got.WorktreeDir {
		t.Fatalf("removed = %v, want %s last", removed, got.WorktreeDir)
	}
	h.mu.Lock()
	stopped := h.stopped
	h.mu.Unlock()
	if !stopped {
		t.Fatal("the session outlived the watch")
	}
	if info, _ := fx.svc.Session(context.Background(), w); info.State != agent.StateNone {
		t.Fatalf("session after stop = %+v", info)
	}
	last := fx.notes.messages()
	if !strings.Contains(last[len(last)-1], "stopped watching (merged)") {
		t.Fatalf("last notification = %q", last[len(last)-1])
	}
	before := fx.api.Total()
	fx.poll(w)
	if fx.api.Total() != before {
		t.Fatal("a stopped watch polled GitHub")
	}
}

func TestStaleFailedChecksAreNotTold(t *testing.T) {
	t.Parallel()
	fx := newFixture(t)
	w := fx.start()
	h := fx.host.last()
	if err := fx.svc.Hook(context.Background(), w.ID, agent.EventPermissionRequest, []byte(`{"tool_name":"Bash"}`)); err != nil {
		t.Fatal(err)
	}
	fx.update(func() {
		fx.pr.CheckRuns = []ghfake.CheckRun{{ID: 1, Name: "build", Status: "completed", Conclusion: "failure", URL: "https://ci/1"}}
	})
	fx.poll(w)
	equal(t, fx.kinds(w), []string{"watch_started", "session_started", "nudged", "check_failed"})
	if len(h.messages()) != 1 {
		t.Fatalf("a blocked agent was told: %q", h.messages())
	}
	fx.update(func() {
		fx.pr.HeadSHA = "def"
		fx.pr.CheckRuns = []ghfake.CheckRun{{ID: 2, Name: "build", Status: "in_progress"}}
	})
	if err := fx.svc.Hook(context.Background(), w.ID, agent.EventStop, []byte(`{}`)); err != nil {
		t.Fatal(err)
	}
	fx.poll(w)
	equal(t, fx.kinds(w), []string{"watch_started", "session_started", "nudged", "check_failed", "commit"})
	if len(h.messages()) != 1 {
		t.Fatalf("a stale failure was told: %q", h.messages())
	}
	rows := fx.activity(w)
	if rows[3].NudgedAt == nil {
		t.Fatal("the stale failure comes back on the next poll")
	}
}

func TestHooksGateTheMessages(t *testing.T) {
	t.Parallel()
	fx := newFixture(t)
	w := fx.start()
	h := fx.host.last()
	ctx := context.Background()

	state := func() agent.State {
		info, err := fx.svc.Session(ctx, w)
		if err != nil {
			t.Fatal(err)
		}
		return info.State
	}
	if err := fx.svc.Hook(ctx, w.ID, agent.EventUserPromptSubmit, []byte(`{}`)); err != nil || state() != agent.StateActive {
		t.Fatalf("state after submit = %q, %v", state(), err)
	}
	if err := fx.svc.Hook(ctx, w.ID, "bogus", nil); err == nil {
		t.Fatal("an unknown event was accepted")
	}
	if err := fx.svc.Hook(ctx, 999, agent.EventStop, nil); err != nil {
		t.Fatalf("a hook of an unknown watch failed: %v", err)
	}
	fx.update(func() {
		fx.pr.IssueComments = []ghfake.Comment{{ID: 11, Author: "bob", CreatedAt: ghfake.At("2026-09-07T12:01:00Z"), Body: "first", URL: "https://c/11"}}
	})
	fx.poll(w)
	if len(h.messages()) != 2 {
		t.Fatalf("messages = %q", h.messages())
	}
	if err := fx.svc.Hook(ctx, w.ID, agent.EventNotification, []byte(`{"notification_type":"agent_needs_input"}`)); err != nil || state() != agent.StateWaitingInput {
		t.Fatalf("state = %q, %v", state(), err)
	}
	fx.update(func() {
		fx.pr.IssueComments = append(fx.pr.IssueComments, ghfake.Comment{ID: 12, Author: "bob", CreatedAt: ghfake.At("2026-09-07T12:02:00Z"), Body: "second", URL: "https://c/12"})
	})
	fx.poll(w)
	equal(t, fx.kinds(w), []string{"watch_started", "session_started", "nudged", "comment", "nudged", "comment"})
	if len(h.messages()) != 2 {
		t.Fatalf("a waiting agent was told: %q", h.messages())
	}
	if _, err := fx.svc.Send(ctx, w.ID, "go with the first option"); err != nil {
		t.Fatalf("Send() to a waiting agent error = %v", err)
	}
	if msgs := h.messages(); len(msgs) != 3 || msgs[2] != "go with the first option" {
		t.Fatalf("messages = %q", msgs)
	}
	rows := fx.activity(w)
	last := rows[len(rows)-1]
	if last.Kind != store.ActivityNudged || last.Summary != "you told the agent: go with the first option" || !strings.Contains(string(last.Payload), `"source":"author"`) {
		t.Fatalf("author message row = %+v", last)
	}
	if err := fx.svc.Hook(ctx, w.ID, agent.EventPermissionRequest, []byte(`{}`)); err != nil || state() != agent.StateBlocked {
		t.Fatalf("state = %q, %v", state(), err)
	}
	if _, err := fx.svc.Send(ctx, w.ID, "yes"); !errors.Is(err, ErrAgentBusy) {
		t.Fatalf("Send() to a blocked agent error = %v", err)
	}
	if _, err := fx.svc.Send(ctx, w.ID, "  "); err == nil {
		t.Fatal("an empty message was sent")
	}
	if err := fx.svc.Hook(ctx, w.ID, agent.EventStop, []byte(`{}`)); err != nil || state() != agent.StateIdle {
		t.Fatalf("state = %q, %v", state(), err)
	}
	fx.poll(w)
	if msgs := h.messages(); len(msgs) != 4 || !strings.Contains(msgs[3], "https://c/12") || strings.Contains(msgs[3], "https://c/11") {
		t.Fatalf("messages = %q", msgs)
	}
	if out, err := fx.svc.Output(ctx, w.ID, 10); err != nil || !strings.Contains(out, "4 messages") {
		t.Fatalf("Output() = %q, %v", out, err)
	}
	if _, err := fx.svc.Send(ctx, 999, "hi"); !errors.Is(err, store.ErrWatchNotFound) {
		t.Fatalf("Send() to a missing watch error = %v", err)
	}
}

func TestSessionExitStartsAgainOnTheNextMessage(t *testing.T) {
	t.Parallel()
	fx := newFixture(t)
	w := fx.start()
	first := fx.host.last()
	ctx := context.Background()
	stored, _ := fx.st.GetWatch(ctx, w.ID)

	fx.advance(time.Hour)
	first.exit(errors.New("exit status 1"))
	fx.waitKinds(w, []string{"watch_started", "session_started", "nudged", "session_exited"})
	rows := fx.activity(w)
	if !strings.Contains(rows[3].Summary, "exit status 1") || !strings.Contains(rows[3].Summary, "starts again with the next message") {
		t.Fatalf("exit row = %+v", rows[3])
	}
	if info, _ := fx.svc.Session(ctx, w); info.State != agent.StateExited {
		t.Fatalf("state = %q", info.State)
	}
	if msgs := fx.waitNote("agent session exited"); len(msgs) != 2 {
		t.Fatalf("notifications = %v", msgs)
	}
	fx.update(func() {
		fx.pr.IssueComments = []ghfake.Comment{{ID: 11, Author: "bob", CreatedAt: ghfake.At("2026-09-07T12:01:00Z"), Body: "first", URL: "https://c/11"}}
	})
	fx.poll(w)
	second := fx.host.last()
	if second == first || fx.host.count() != 2 {
		t.Fatal("no new session")
	}
	if argv := strings.Join(second.spec.Argv, " "); !strings.Contains(argv, "--resume "+stored.AgentSession) {
		t.Fatalf("second launch = %s", argv)
	}
	msgs := second.messages()
	if len(msgs) != 1 || !strings.Contains(msgs[0], "first") || strings.Contains(msgs[0], "babysitting") {
		t.Fatalf("a continued session was introduced again: %q", msgs)
	}
	equal(t, fx.kinds(w), []string{"watch_started", "session_started", "nudged", "session_exited", "comment", "session_started", "nudged"})
	if !strings.Contains(fx.activity(w)[5].Summary, "continued") {
		t.Fatalf("second start row = %+v", fx.activity(w)[5])
	}

	second.exit(errors.New("exit status 1"))
	fx.waitKinds(w, []string{"watch_started", "session_started", "nudged", "session_exited", "comment", "session_started", "nudged", "session_exited"})
	if got, _ := fx.st.GetWatch(ctx, w.ID); got.AgentSession != "" {
		t.Fatalf("agent session after an early exit = %q", got.AgentSession)
	}
	fx.update(func() {
		fx.pr.IssueComments = append(fx.pr.IssueComments, ghfake.Comment{ID: 12, Author: "bob", CreatedAt: ghfake.At("2026-09-07T12:02:00Z"), Body: "second", URL: "https://c/12"})
	})
	fx.poll(w)
	third := fx.host.last()
	if fx.host.count() != 3 || strings.Contains(strings.Join(third.spec.Argv, " "), "--resume") {
		t.Fatalf("third launch = %v", third.spec.Argv)
	}
	if msgs := third.messages(); len(msgs) != 2 || !strings.Contains(msgs[0], "babysitting") || !strings.Contains(msgs[1], "second") {
		t.Fatalf("messages = %q", msgs)
	}
	if got, _ := fx.st.GetWatch(ctx, w.ID); got.AgentSession == "" || got.AgentSession == stored.AgentSession {
		t.Fatalf("agent session of the new conversation = %q", got.AgentSession)
	}
}

func TestSessionFailuresAreRecordedAndRetried(t *testing.T) {
	t.Parallel()
	fx := newFixture(t)
	fx.host.startErr = errors.New("no pty")
	w := fx.start()
	equal(t, fx.kinds(w), []string{"watch_started", "agent_failed"})
	rows := fx.activity(w)
	if !strings.Contains(rows[1].Summary, "could not start the agent session: no pty") {
		t.Fatalf("failure row = %+v", rows[1])
	}
	if msgs := fx.notes.messages(); len(msgs) != 2 || !strings.Contains(msgs[1], "could not start") {
		t.Fatalf("notifications = %v", msgs)
	}
	fx.host.startErr = nil
	fx.update(func() {
		fx.pr.IssueComments = []ghfake.Comment{{ID: 11, Author: "bob", CreatedAt: ghfake.At("2026-09-07T12:01:00Z"), Body: "first", URL: "https://c/11"}}
	})
	fx.poll(w)
	equal(t, fx.kinds(w), []string{"watch_started", "agent_failed", "comment", "session_started", "nudged", "nudged"})
	if msgs := fx.host.last().messages(); len(msgs) != 2 || !strings.Contains(msgs[0], "babysitting") || !strings.Contains(msgs[1], "first") {
		t.Fatalf("messages = %q", msgs)
	}

	h := fx.host.last()
	h.mu.Lock()
	h.sendErr = errors.New("terminal gone")
	h.mu.Unlock()
	fx.update(func() {
		fx.pr.IssueComments = append(fx.pr.IssueComments, ghfake.Comment{ID: 12, Author: "bob", CreatedAt: ghfake.At("2026-09-07T12:02:00Z"), Body: "second", URL: "https://c/12"})
	})
	fx.poll(w)
	equal(t, fx.kinds(w), []string{"watch_started", "agent_failed", "comment", "session_started", "nudged", "nudged", "comment", "agent_failed"})
	todo, _ := fx.st.UnnudgedActionable(context.Background(), w.ID)
	if len(todo) != 1 || todo[0].Ref != "12" {
		t.Fatalf("unnudged = %+v", todo)
	}
	h.mu.Lock()
	h.sendErr = nil
	h.mu.Unlock()
	fx.poll(w)
	if msgs := h.messages(); len(msgs) != 3 || !strings.Contains(msgs[2], "second") {
		t.Fatalf("messages = %q", msgs)
	}
}

func TestPollStopsAfterLostAccess(t *testing.T) {
	t.Parallel()
	fx := newFixture(t)
	w := fx.start()
	before := len(fx.git.removedDirs())
	fx.gone("Not Found")
	for i := range lostAccessAfter {
		fx.advance(time.Minute)
		if err := fx.svc.Poll(context.Background(), w.ID); err == nil {
			t.Fatal("Poll() of a gone pull request expected an error")
		}
		got, _ := fx.st.GetWatch(context.Background(), w.ID)
		if i < lostAccessAfter-1 && (got.Status != store.WatchActive || got.ConsecutiveErrors != i+1) {
			t.Fatalf("after %d errors: %+v", i+1, got)
		}
	}
	got, _ := fx.st.GetWatch(context.Background(), w.ID)
	if got.Status != store.WatchStopped || got.StopReason != store.StopLostAccess {
		t.Fatalf("watch = %+v", got)
	}
	if len(fx.git.removedDirs()) != before {
		t.Fatalf("removed = %v", fx.git.removedDirs())
	}
	if !strings.Contains(string(got.Summary), `"worktreeRemoved":false`) {
		t.Fatalf("summary = %s", got.Summary)
	}
	fx.host.last().mu.Lock()
	stopped := fx.host.last().stopped
	fx.host.last().mu.Unlock()
	if !stopped {
		t.Fatal("the session outlived the watch")
	}
}

func TestHeartbeatAndRecovery(t *testing.T) {
	t.Parallel()
	fx := newFixture(t)
	WithHeartbeat(30 * time.Minute)(fx.svc)
	w := fx.start()
	opened := []string{"watch_started", "session_started", "nudged"}

	for range 20 {
		fx.poll(w)
	}
	equal(t, fx.kinds(w), opened)
	for range 15 {
		fx.poll(w)
	}
	equal(t, fx.kinds(w), append(opened, "heartbeat"))
	rows := fx.activity(w)
	if !strings.Contains(rows[3].Summary, "still watching, head abc, checks green, mergeable") {
		t.Fatalf("heartbeat = %q", rows[3].Summary)
	}
	if n := len(fx.notes.messages()); n != 1 {
		t.Fatalf("heartbeat notified: %d messages", n)
	}

	ctx := context.Background()
	if _, _, err := fx.st.InsertActivity(ctx, store.Activity{WatchID: w.ID, Kind: store.ActivityCheckFailed, Ref: "lint@abc", At: fx.clock(), Summary: "lint failed on abc", Payload: json.RawMessage(`{"check":"lint","sha":"abc"}`)}); err != nil {
		t.Fatal(err)
	}
	stored, _ := fx.st.GetWatch(ctx, w.ID)
	fx.svc.stopSessions()
	next := fx.newService()
	WithHeartbeat(30 * time.Minute)(next)
	runCtx, cancel := context.WithCancel(ctx)
	done := make(chan error, 1)
	go func() { done <- next.Run(runCtx) }()
	testutil.Within(testutil.Timeout, func() bool {
		rows, _ := fx.st.UnreportedActivity(ctx, w.ID)
		todo, _ := fx.st.UnnudgedActionable(ctx, w.ID)
		return len(rows) == 0 && len(todo) == 0
	})
	cancel()
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatalf("Run() = %v", err)
	}
	if rows, _ := fx.st.UnreportedActivity(ctx, w.ID); len(rows) != 0 {
		t.Fatalf("unreported after Run = %v", rows)
	}
	if msgs := fx.notes.messages(); len(msgs) != 2 || !strings.Contains(msgs[1], "lint failed") {
		t.Fatalf("notifications = %v", msgs)
	}
	h := fx.host.last()
	if fx.host.count() != 2 || !strings.Contains(strings.Join(h.spec.Argv, " "), "--resume "+stored.AgentSession) {
		t.Fatalf("the new daemon did not continue the session: %d sessions, %v", fx.host.count(), h.spec.Argv)
	}
	if msgs := h.messages(); len(msgs) != 1 || !strings.Contains(msgs[0], "Failed: lint") {
		t.Fatalf("messages = %q", msgs)
	}
	equal(t, fx.kinds(w), append(opened, "heartbeat", "check_failed", "session_started", "nudged"))
	h.mu.Lock()
	stopped := h.stopped
	h.mu.Unlock()
	if !stopped {
		t.Fatal("the session outlived the daemon")
	}

	stopped2, err := next.Stop(ctx, w.ID, StopOptions{})
	if err != nil || stopped2.Status != store.WatchStopped || stopped2.StopReason != store.StopUser {
		t.Fatalf("Stop() = %+v, %v", stopped2, err)
	}
	if again, err := next.Stop(ctx, w.ID, StopOptions{}); err != nil || again.StoppedAt == nil || !again.StoppedAt.Equal(*stopped2.StoppedAt) {
		t.Fatalf("second Stop() = %+v, %v", again, err)
	}
	if _, err := next.Stop(ctx, 999, StopOptions{}); !errors.Is(err, store.ErrWatchNotFound) {
		t.Fatalf("Stop(missing) = %v", err)
	}
	if _, err := next.Send(ctx, w.ID, "hello"); !errors.Is(err, ErrWatchStopped) {
		t.Fatalf("Send() to a stopped watch error = %v", err)
	}
}

func TestOutputReadsTheLogWithoutASession(t *testing.T) {
	t.Parallel()
	fx := newFixture(t)
	w := fx.start()
	ctx := context.Background()
	if out, err := fx.svc.Output(ctx, w.ID, 0); err != nil || !strings.Contains(out, "fake prompt") {
		t.Fatalf("Output() = %q, %v", out, err)
	}
	if _, err := fx.svc.Stop(ctx, w.ID, StopOptions{}); err != nil {
		t.Fatal(err)
	}
	if out, err := fx.svc.Output(ctx, w.ID, 0); err != nil || out != "" {
		t.Fatalf("Output() without a log = %q, %v", out, err)
	}
	log := filepath.Join(fx.data, "sessions", fmt.Sprintf("%d.log", w.ID))
	if err := os.MkdirAll(filepath.Dir(log), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(log, []byte("one\ntwo\nthree\n"), 0o640); err != nil {
		t.Fatal(err)
	}
	if out, err := fx.svc.Output(ctx, w.ID, 2); err != nil || out != "two\nthree\n" {
		t.Fatalf("Output() from the log = %q, %v", out, err)
	}
	if _, err := fx.svc.Output(ctx, 999, 2); !errors.Is(err, store.ErrWatchNotFound) {
		t.Fatalf("Output() of a missing watch error = %v", err)
	}
}

func TestOutputCarriesNoSecretOfTheScreen(t *testing.T) {
	t.Parallel()
	fx := newFixture(t)
	w := fx.start()
	ctx := context.Background()
	drawn := "\x1b[32mgh api\x1b[0m: Authorization: Bearer " + secretToken + "\n"
	fx.host.last().draw(drawn)
	out, err := fx.svc.Output(ctx, w.ID, 0)
	if err != nil {
		t.Fatalf("Output() error = %v", err)
	}
	if strings.Contains(out, secretToken) {
		t.Fatalf("the screen of the session carried the token: %q", out)
	}
	if !strings.Contains(out, "\x1b[32mgh api\x1b[0m") {
		t.Fatalf("the escape sequences of the terminal are gone: %q", out)
	}

	if _, err := fx.svc.Stop(ctx, w.ID, StopOptions{}); err != nil {
		t.Fatal(err)
	}
	log := filepath.Join(fx.data, "sessions", fmt.Sprintf("%d.log", w.ID))
	if err := os.MkdirAll(filepath.Dir(log), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(log, []byte(drawn), 0o640); err != nil {
		t.Fatal(err)
	}
	out, err = fx.svc.Output(ctx, w.ID, 0)
	if err != nil || strings.Contains(out, secretToken) {
		t.Fatalf("the log of the session carried the token: %q, %v", out, err)
	}
}

func TestDependabotBranchIsNotPushed(t *testing.T) {
	t.Parallel()
	fx := newFixture(t)
	fx.update(func() { fx.pr.Author, fx.pr.MergeableState = "dependabot[bot]", "behind" })
	w := fx.start()
	h := fx.host.last()
	msgs := h.messages()
	if len(msgs) != 2 || !strings.Contains(msgs[0], "Dependabot opened this pull request") || !strings.Contains(msgs[1], "@dependabot rebase") || strings.Contains(msgs[1], "force-with-lease") {
		t.Fatalf("messages = %q", msgs)
	}
	if w.Author != "dependabot[bot]" {
		t.Fatalf("watch = %+v", w)
	}
}

func TestWithoutAgentsReviewItemsNotify(t *testing.T) {
	t.Parallel()
	fx := newFixture(t)
	fx.svc = New(Deps{
		Log:           testutil.Logger(fx.t),
		Store:         fx.st,
		NewClient:     func(context.Context) (*github.Client, error) { return fx.client, nil },
		Git:           fx.git,
		Notifications: fx.notes,
		Host:          fx.host,
		DataDir:       fx.data,
	}, WithClock(func() time.Time { return fx.clock() }), WithInterval(time.Minute))
	w := fx.start()
	equal(t, fx.kinds(w), []string{"watch_started"})
	fx.update(func() {
		fx.pr.IssueComments = []ghfake.Comment{{ID: 11, Author: "bob", CreatedAt: ghfake.At("2026-09-07T12:01:00Z"), Body: "Please return the error", URL: "https://c/11"}}
	})
	fx.poll(w)
	equal(t, fx.kinds(w), []string{"watch_started", "comment"})
	if msgs := fx.notes.messages(); len(msgs) != 2 || !strings.Contains(msgs[1], "bob commented") {
		t.Fatalf("notifications = %v", msgs)
	}
	if fx.host.count() != 0 {
		t.Fatal("a daemon without agents opened a session")
	}
	if _, err := fx.svc.Send(context.Background(), w.ID, "hi"); !errors.Is(err, ErrNoAgent) {
		t.Fatalf("Send() without an agent error = %v", err)
	}
}

func TestStartStoresModel(t *testing.T) {
	t.Parallel()
	fx := newFixture(t)
	w, err := fx.svc.Start(context.Background(), StartRequest{
		Target:    snapshot.Target{Owner: "octo", Name: "hello", Number: 3},
		SourceDir: fx.dir,
		Provider:  ProviderClaude,
		Model:     "Sonnet",
	})
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	if w.Model != "sonnet" {
		t.Fatalf("watch model = %q", w.Model)
	}
	if argv := strings.Join(fx.host.last().spec.Argv, " "); !strings.Contains(argv, "--model sonnet") {
		t.Fatalf("launch = %s", argv)
	}
}

func TestStartRejectsAModelTheProviderDoesNotOffer(t *testing.T) {
	t.Parallel()
	fx := newFixture(t)
	_, err := fx.svc.Start(context.Background(), StartRequest{
		Target:    snapshot.Target{Owner: "octo", Name: "hello", Number: 3},
		SourceDir: fx.dir,
		Provider:  ProviderClaude,
		Model:     "gpt-5.3-codex",
	})
	if !errors.Is(err, ErrBadModel) {
		t.Fatalf("Start() error = %v, want ErrBadModel", err)
	}
}

func TestProvidersReportWhichRunnersTheDaemonHas(t *testing.T) {
	t.Parallel()
	svc := New(Deps{Log: testutil.Logger(t), Agents: map[string]agent.Runner{ProviderClaude: &fakeRunner{}}})
	got := svc.Providers()
	if len(got) != len(Catalog()) {
		t.Fatalf("providers = %+v", got)
	}
	available := map[string]bool{}
	for _, p := range got {
		available[p.ID] = p.Available
		if len(p.Models) == 0 {
			t.Errorf("provider %q offers no model", p.ID)
		}
	}
	if !available[ProviderClaude] || available[ProviderCopilot] {
		t.Fatalf("available = %v", available)
	}
}

func TestIntervalWords(t *testing.T) {
	t.Parallel()
	cases := map[time.Duration]string{time.Minute: "1 minute", 3 * time.Minute: "3 minutes", 2 * time.Hour: "2 hours", 90 * time.Second: "90 seconds", 500 * time.Millisecond: "500 milliseconds", time.Millisecond: "1 millisecond"}
	for d, want := range cases {
		if got := interval(d); got != want {
			t.Errorf("interval(%s) = %q, want %q", d, got, want)
		}
	}
}

func TestStopRemovesTheWorktree(t *testing.T) {
	t.Parallel()
	fx := newFixture(t)
	w := fx.start()
	before := len(fx.git.removedDirs())

	stopped, err := fx.svc.Stop(context.Background(), w.ID, StopOptions{})
	if err != nil {
		t.Fatalf("Stop() error = %v", err)
	}
	removed := fx.git.removedDirs()
	if len(removed) != before+1 || removed[len(removed)-1] != w.WorktreeDir {
		t.Fatalf("removed = %v, want %s last", removed, w.WorktreeDir)
	}
	if !strings.Contains(string(stopped.Summary), `"worktreeRemoved":true`) {
		t.Fatalf("summary = %s", stopped.Summary)
	}
	msgs := fx.notes.messages()
	if !strings.Contains(msgs[len(msgs)-1], "worktree removed") {
		t.Fatalf("last notification = %q", msgs[len(msgs)-1])
	}
}

func TestStopKeepsTheWorktreeWhenAsked(t *testing.T) {
	t.Parallel()
	fx := newFixture(t)
	w := fx.start()
	before := fx.git.removedDirs()

	stopped, err := fx.svc.Stop(context.Background(), w.ID, StopOptions{KeepWorktree: new(true)})
	if err != nil {
		t.Fatalf("Stop() error = %v", err)
	}
	if got := fx.git.removedDirs(); len(got) != len(before) {
		t.Fatalf("removed = %v, want no removal", got)
	}
	if !strings.Contains(string(stopped.Summary), `"worktreeRemoved":false`) {
		t.Fatalf("summary = %s", stopped.Summary)
	}
	msgs := fx.notes.messages()
	if !strings.Contains(msgs[len(msgs)-1], "worktree kept at "+w.WorktreeDir) {
		t.Fatalf("last notification = %q", msgs[len(msgs)-1])
	}
}

func TestStopSaysSoWhenTheRemovalFails(t *testing.T) {
	t.Parallel()
	fx := newFixture(t)
	w := fx.start()
	fx.git.mu.Lock()
	fx.git.removeErr = errors.New("worktree is locked")
	fx.git.mu.Unlock()

	stopped, err := fx.svc.Stop(context.Background(), w.ID, StopOptions{})
	if err != nil {
		t.Fatalf("Stop() error = %v", err)
	}
	if stopped.Status != store.WatchStopped {
		t.Fatalf("Stop() = %+v", stopped)
	}
	if !strings.Contains(string(stopped.Summary), `"worktreeRemoved":false`) {
		t.Fatalf("summary = %s", stopped.Summary)
	}
	msgs := fx.notes.messages()
	if !strings.Contains(msgs[len(msgs)-1], "its removal failed") {
		t.Fatalf("last notification = %q", msgs[len(msgs)-1])
	}
}

func TestWorktreeWordWithoutGit(t *testing.T) {
	t.Parallel()
	s := &Service{log: testutil.Logger(t)}
	w := store.Watch{ID: 1, WorktreeDir: "/tmp/wt"}
	fate := s.removeWorktree(context.Background(), w)
	if fate != worktreeNotTried {
		t.Fatalf("removeWorktree() without git = %v", fate)
	}
	if got := worktreeWord(w, false, fate); got != "worktree left at /tmp/wt" {
		t.Fatalf("worktreeWord() = %q", got)
	}
	if got := worktreeWord(w, false, worktreeRemovalFailed); got != "worktree left at /tmp/wt, its removal failed" {
		t.Fatalf("worktreeWord() after a failed removal = %q", got)
	}
}

type statusGit struct {
	fakeGit
	st     *store.Store
	id     int64
	status store.WatchStatus
}

func (g *statusGit) Remove(ctx context.Context, source, dir, branch string) error {
	if w, err := g.st.GetWatch(ctx, g.id); err == nil {
		g.status = w.Status
	}
	return g.fakeGit.Remove(ctx, source, dir, branch)
}

func TestStopWritesTheStopBeforeItRemovesTheWorktree(t *testing.T) {
	t.Parallel()
	fx := newFixture(t)
	w := fx.start()
	g := &statusGit{st: fx.st, id: w.ID}
	fx.svc.git = g

	stopped, err := fx.svc.Stop(context.Background(), w.ID, StopOptions{})
	if err != nil {
		t.Fatalf("Stop() error = %v", err)
	}
	if g.status != store.WatchStopped {
		t.Fatalf("the worktree went while the watch was %q", g.status)
	}
	if !strings.Contains(string(stopped.Summary), `"worktreeRemoved":true`) {
		t.Fatalf("summary = %s", stopped.Summary)
	}
	got, _ := fx.st.GetWatch(context.Background(), w.ID)
	if !strings.Contains(string(got.Summary), `"worktreeRemoved":true`) {
		t.Fatalf("stored summary = %s", got.Summary)
	}
}

type branchLeftGit struct {
	fakeGit
}

func (g *branchLeftGit) Remove(ctx context.Context, source, dir, branch string) error {
	_ = g.fakeGit.Remove(ctx, source, dir, branch)
	return fmt.Errorf("%w: %s", worktree.ErrBranchLeft, branch)
}

func TestStopReportsTheWorktreeGoneWhenOnlyTheBranchStays(t *testing.T) {
	t.Parallel()
	fx := newFixture(t)
	w := fx.start()
	fx.svc.git = &branchLeftGit{}

	stopped, err := fx.svc.Stop(context.Background(), w.ID, StopOptions{})
	if err != nil {
		t.Fatalf("Stop() error = %v", err)
	}
	if !strings.Contains(string(stopped.Summary), `"worktreeRemoved":true`) {
		t.Fatalf("summary = %s", stopped.Summary)
	}
	if !strings.Contains(string(stopped.Summary), `"workBranchLeft":"babysitter/fix"`) {
		t.Fatalf("the summary names no branch that stays: %s", stopped.Summary)
	}
	msgs := fx.notes.messages()
	last := msgs[len(msgs)-1]
	if !strings.Contains(last, "worktree removed") {
		t.Fatalf("last notification = %q", last)
	}
	if !strings.Contains(last, w.WorkBranch) || !strings.Contains(last, w.SourceDir) {
		t.Fatalf("the stop says nothing about the branch that stays in the checkout: %q", last)
	}
}

func TestStopAgreesWithItsActivityAboutTheWorktree(t *testing.T) {
	t.Parallel()
	fx := newFixture(t)
	w := fx.start()

	stopped, err := fx.svc.Stop(context.Background(), w.ID, StopOptions{})
	if err != nil {
		t.Fatalf("Stop() error = %v", err)
	}
	rows := fx.activity(w)
	last := rows[len(rows)-1]
	if last.Kind != store.ActivityWatchStopped {
		t.Fatalf("last activity = %s", last.Kind)
	}
	var sum, payload Summary
	if err := json.Unmarshal(stopped.Summary, &sum); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(last.Payload, &payload); err != nil {
		t.Fatal(err)
	}
	if sum.WorktreeRemoved != payload.WorktreeRemoved {
		t.Fatalf("the watch says worktreeRemoved=%v, its activity says %v", sum.WorktreeRemoved, payload.WorktreeRemoved)
	}
	if !sum.WorktreeRemoved || sum.Messages != 1 {
		t.Fatalf("summary = %s", stopped.Summary)
	}
}

func TestPollPutsTheLogOfAFailedJobInTheMessage(t *testing.T) {
	t.Parallel()
	fx := newFixture(t)
	w, err := fx.svc.Start(context.Background(), StartRequest{Target: snapshot.Target{Owner: "octo", Name: "hello", Number: 3}, SourceDir: fx.dir})
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	setup := []string{"##[group]Run actions/checkout@v4", "##[endgroup]"}
	for i := range 200 {
		setup = append(setup, fmt.Sprintf("Syncing repository, object %d", i))
	}
	fx.failBuild(stampedLog(append(setup,
		"##[group]Run go test ./...",
		"##[endgroup]",
		"--- FAIL: TestThing",
		"    thing_test.go:12: want 3, got 4",
		"##[error]Process completed with exit code 1.",
	)...))

	fx.poll(w)

	msgs := fx.host.last().messages()
	last := msgs[len(msgs)-1]
	for _, want := range []string{
		"Failed: build",
		"Failing step: Run go test ./...",
		agent.BeginData,
		"--- FAIL: TestThing",
		"thing_test.go:12: want 3, got 4",
		"##[error]Process completed with exit code 1.",
		agent.EndData,
		"gh api repos/octo/hello/actions/jobs/9/logs",
	} {
		if !strings.Contains(last, want) {
			t.Errorf("the message lacks %q:\n%s", want, last)
		}
	}
	if strings.Contains(last, "2026-09-18T10:23") {
		t.Errorf("the message keeps the timestamps of the log:\n%s", last)
	}
	if strings.Contains(last, "Syncing repository, object 0") {
		t.Errorf("the message keeps a step that passed:\n%s", last)
	}
	if n := strings.Count(last, "--- FAIL: TestThing"); n != 1 {
		t.Errorf("the message carries the log of one job %d times:\n%s", n, last)
	}
	if n := strings.Count(last, "Failed: "); n != 1 {
		t.Errorf("the message reports one failure %d times:\n%s", n, last)
	}
}

func TestPollKeepsTheMessageWhenTheLogCannotBeRead(t *testing.T) {
	t.Parallel()
	fx := newFixture(t)
	w, err := fx.svc.Start(context.Background(), StartRequest{Target: snapshot.Target{Owner: "octo", Name: "hello", Number: 3}, SourceDir: fx.dir})
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	fx.failBuild("")
	fx.update(func() {
		fx.pr.CheckRuns = []ghfake.CheckRun{{ID: 1, Name: "build", Status: "completed", Conclusion: "failure"}}
	})

	fx.poll(w)

	msgs := fx.host.last().messages()
	last := msgs[len(msgs)-1]
	if !strings.Contains(last, "Failed: build") {
		t.Errorf("the message lacks the failed check:\n%s", last)
	}
	if strings.Contains(last, "Failing step:") {
		t.Errorf("the message names a step it could not read:\n%s", last)
	}
}

func TestPollDropsTheLogCommandWhenGitHubServesNoLog(t *testing.T) {
	t.Parallel()
	fx := newFixture(t)
	w, err := fx.svc.Start(context.Background(), fx.startRequest())
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	fx.failBuild(stampedLog("##[error]Process completed with exit code 1."))
	fx.logStatus(http.StatusNotFound)

	fx.poll(w)

	msgs := fx.host.last().messages()
	last := msgs[len(msgs)-1]
	if !strings.Contains(last, "Failed: build") {
		t.Fatalf("the message lacks the failed check:\n%s", last)
	}
	if strings.Contains(last, "gh api") {
		t.Errorf("the message hands the agent a command that answers 404:\n%s", last)
	}
	if !strings.Contains(last, "GitHub serves no log for this job") {
		t.Errorf("the message does not say that the log is not there:\n%s", last)
	}
}

func TestPollKeepsTheLogCommandWhenTheLogReadFailedForAnotherReason(t *testing.T) {
	t.Parallel()
	fx := newFixture(t)
	w, err := fx.svc.Start(context.Background(), fx.startRequest())
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	fx.failBuild(stampedLog("##[error]Process completed with exit code 1."))
	fx.logStatus(http.StatusBadGateway)

	fx.poll(w)

	msgs := fx.host.last().messages()
	last := msgs[len(msgs)-1]
	if !strings.Contains(last, "gh api repos/octo/hello/actions/jobs/9/logs") {
		t.Errorf("the message drops the log command of a read that may work again:\n%s", last)
	}
}

func stampedLog(lines ...string) string {
	var b strings.Builder
	for i, l := range lines {
		fmt.Fprintf(&b, "2026-09-18T10:23:%02d.1234567Z %s\n", i%60, l)
	}
	return b.String()
}

func (fx *fixture) startNumber(n int) store.Watch {
	fx.t.Helper()
	fx.openPR(n)
	w, err := fx.svc.Start(context.Background(), StartRequest{
		Target: snapshot.Target{Owner: "octo", Name: "hello", Number: n}, SourceDir: fx.dir,
	})
	if err != nil {
		fx.t.Fatalf("Start(#%d) error = %v", n, err)
	}
	return w
}

func prNumberOf(path string) string {
	for _, sep := range []string{"/pulls/", "/issues/"} {
		if _, after, ok := strings.Cut(path, sep); ok {
			rest := after
			if before, _, ok := strings.Cut(rest, "/"); ok {
				return before
			}
			return rest
		}
	}
	return ""
}

func TestOnePassPollsTheWatchesBesideEachOther(t *testing.T) {
	t.Parallel()
	fx := newFixture(t)
	numbers := []int{3, 4, 5}
	for _, n := range numbers {
		fx.startNumber(n)
	}

	var mu sync.Mutex
	seen := map[string]bool{}
	alone := false
	together := make(chan struct{})
	var once sync.Once
	fx.api.Observe(func(a ghfake.Action) {
		n := prNumberOf(a.Path)
		if n == "" {
			return
		}
		mu.Lock()
		seen[n] = true
		full := len(seen) == len(numbers)
		mu.Unlock()
		if full {
			once.Do(func() { close(together) })
		}
		select {
		case <-together:
		case <-time.After(2 * time.Second):
			mu.Lock()
			alone = true
			mu.Unlock()
		}
	})

	fx.advance(time.Minute)
	fx.svc.pass(context.Background())

	mu.Lock()
	defer mu.Unlock()
	if alone {
		t.Fatalf("a poll waited on its own; the pass reached %d of %d watches at a time", len(seen), len(numbers))
	}
}

func (fx *fixture) clock() time.Time {
	fx.nowMu.Lock()
	defer fx.nowMu.Unlock()
	return fx.now
}

func (fx *fixture) advance(d time.Duration) {
	fx.nowMu.Lock()
	defer fx.nowMu.Unlock()
	fx.now = fx.now.Add(d)
}
