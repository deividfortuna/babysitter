package prwatch

import (
	"context"
	"errors"
	"slices"
	"sync/atomic"
	"testing"

	"github.com/google/go-github/v91/github"

	"github.com/deividfortuna/babysitter/internal/ghclient/ghfake"
	"github.com/deividfortuna/babysitter/internal/snapshot"
	"github.com/deividfortuna/babysitter/internal/store"
)

func (fx *fixture) startRequest() StartRequest {
	return StartRequest{Target: snapshot.Target{Owner: "octo", Name: "hello", Number: 3}, SourceDir: fx.dir}
}

func TestWorktreeDirNameKeepsTheDirectoryUnderTheWorktreesRoot(t *testing.T) {
	t.Parallel()
	cases := []struct {
		key  store.WatchKey
		want string
	}{
		{store.WatchKey{Owner: "Octo", Name: "Hello.World", Number: 3}, "octo-hello.world-3"},
		{store.WatchKey{Owner: "octo", Name: "my_repo-2", Number: 12}, "octo-my_repo-2-12"},
		{store.WatchKey{Owner: "..", Name: "..", Number: 1}, "..-..-1"},
	}
	for _, c := range cases {
		got, err := worktreeDirName(c.key)
		if err != nil || got != c.want {
			t.Errorf("worktreeDirName(%+v) = %q, %v, want %q", c.key, got, err, c.want)
		}
	}
	for _, key := range []store.WatchKey{
		{Owner: "../../etc", Name: "x", Number: 1},
		{Owner: "octo", Name: `..\..\x`, Number: 1},
		{Owner: "octo", Name: "hello world", Number: 1},
	} {
		if got, err := worktreeDirName(key); err == nil {
			t.Errorf("worktreeDirName(%+v) = %q, want an error", key, got)
		}
	}
}

func TestStartAnswersWithTheAgentSessionItOpened(t *testing.T) {
	t.Parallel()
	fx := newFixture(t)
	w := fx.start()
	if w.AgentSession == "" {
		t.Fatal("Start() answered with a watch that names no agent session")
	}
	stored, err := fx.st.GetWatch(context.Background(), w.ID)
	if err != nil {
		t.Fatal(err)
	}
	if w.AgentSession != stored.AgentSession {
		t.Fatalf("agent session = %q, want %q", w.AgentSession, stored.AgentSession)
	}
}

func TestStartTakesTheConversationIdFromTheRunner(t *testing.T) {
	t.Parallel()
	fx := newFixture(t)
	fx.svc.agents[ProviderClaude] = &fakeRunner{signals: true, sessionID: "conversation-of-the-provider"}

	w := fx.start()

	if w.AgentSession != "conversation-of-the-provider" {
		t.Fatalf("agent session = %q, want the one the runner minted", w.AgentSession)
	}
	argv := fx.host.last().spec.Argv
	if !slices.Contains(argv, "conversation-of-the-provider") {
		t.Fatalf("the command line of the session is %v, and it carries no id of the runner", argv)
	}
}

func TestStartRemovesTheWorktreeWhenTheWatchCannotBeCreated(t *testing.T) {
	t.Parallel()
	fx := newFixture(t)
	fx.git.onCreate = func() {
		if _, err := fx.st.CreateWatch(context.Background(), store.Watch{
			Owner: "octo", Name: "hello", Number: 3, HeadRef: "fix", StartedAt: fx.clock(),
		}); err != nil {
			t.Error(err)
		}
	}
	_, err := fx.svc.Start(context.Background(), fx.startRequest())
	if !errors.Is(err, store.ErrWatchExists) {
		t.Fatalf("Start() error = %v, want %v", err, store.ErrWatchExists)
	}
	created := fx.git.createdDirs()
	if len(created) != 1 {
		t.Fatalf("created = %v", created)
	}
	if got := fx.git.removedDirs(); len(got) != 2 || got[1] != created[0] {
		t.Fatalf("removed = %v, want the worktree of the failed start at %s", got, created[0])
	}
}

func TestStartReadsThePullRequestOnce(t *testing.T) {
	t.Parallel()
	fx := newFixture(t)
	var reads atomic.Int64
	fx.api.Observe(func(a ghfake.Action) {
		if a.Route == ghfake.RoutePull {
			reads.Add(1)
		}
	})

	fx.start()

	if n := reads.Load(); n != 1 {
		t.Fatalf("the start read the pull request %d times, and the snapshot holds all it needs", n)
	}
}

func TestStartAsksForThePushRightTheUserAndTheRulesAtOnce(t *testing.T) {
	t.Parallel()
	fx := newFixture(t)
	m := newMeeting("repo", "user", "rules")
	parties := map[string]string{
		ghfake.RouteRepo:  "repo",
		ghfake.RouteUser:  "user",
		ghfake.RouteRules: "rules",
	}
	fx.api.Observe(func(a ghfake.Action) {
		if p, ok := parties[a.Route]; ok {
			m.arrive(p)
		}
	})

	fx.start()

	if missed := m.missed(); len(missed) != 0 {
		t.Fatalf("the calls of the start ran one after another, missed: %v", missed)
	}
}

func TestStartFetchesTheHeadBranchWhileItChecksAccess(t *testing.T) {
	t.Parallel()
	fx := newFixture(t)
	m := newMeeting("fetch", "user")
	fx.git.onFetch = func() { m.arrive("fetch") }
	fx.api.Observe(func(a ghfake.Action) {
		if a.Route == ghfake.RouteUser {
			m.arrive("user")
		}
	})

	fx.start()

	if missed := m.missed(); len(missed) != 0 {
		t.Fatalf("the fetch waited for the access checks, missed: %v", missed)
	}
	if got := fx.git.fetched(); !slices.Equal(got, []string{"origin/fix"}) {
		t.Fatalf("fetched = %v, want the head branch once", got)
	}
}

func TestStartThatCannotFetchMakesNoWorktree(t *testing.T) {
	t.Parallel()
	fx := newFixture(t)
	unpushed := errors.New("fetch origin/fix: exit status 128 (was the branch pushed?)")
	fx.git.fetchErr = unpushed

	_, err := fx.svc.Start(context.Background(), fx.startRequest())

	if !errors.Is(err, unpushed) {
		t.Fatalf("Start() error = %v, want %v", err, unpushed)
	}
	if created := fx.git.createdDirs(); len(created) != 0 {
		t.Fatalf("a start that could not fetch made a worktree: %v", created)
	}
}

func TestStartNamesTheMissingPushRightOverAFailedFetch(t *testing.T) {
	t.Parallel()
	fx := newFixture(t)
	fx.git.fetchErr = errors.New("fetch origin/fix: exit status 128")
	fx.update(func() { fx.repo.Push = false })

	_, err := fx.svc.Start(context.Background(), fx.startRequest())

	if !errors.Is(err, ErrNoPushAccess) {
		t.Fatalf("Start() error = %v, want %v", err, ErrNoPushAccess)
	}
	if created := fx.git.createdDirs(); len(created) != 0 {
		t.Fatalf("a rejected start made a worktree: %v", created)
	}
}

func TestStartStopsWhenTheTokenCannotReachTheRepository(t *testing.T) {
	t.Parallel()
	fx := newFixture(t)
	notInstalled := errors.New("the GitHub App is not installed on the repository")
	var asked []string
	fx.access = func(_ context.Context, _ *github.Client, owner, name string) error {
		asked = append(asked, owner+"/"+name)
		return notInstalled
	}
	fx.svc = fx.newService()

	_, err := fx.svc.Start(context.Background(), fx.startRequest())

	if !errors.Is(err, notInstalled) {
		t.Fatalf("Start() error = %v, want %v", err, notInstalled)
	}
	if !slices.Equal(asked, []string{"octo/hello"}) {
		t.Fatalf("access asked for %v, want the repository once", asked)
	}
	if n := fx.api.Count(ghfake.RoutePull); n != 0 {
		t.Fatalf("the start read the pull request %d times before the access check refused it", n)
	}
	if created := fx.git.createdDirs(); len(created) != 0 {
		t.Fatalf("a rejected start made a worktree: %v", created)
	}
}

func TestStartAnswersWithTheWatchWhenALaterStepFails(t *testing.T) {
	t.Parallel()
	fx := newFixture(t)
	fx.update(func() { fx.pr.MergeableState = "dirty" })
	fx.notes.onSend = func() { fx.st.Close() }

	w, err := fx.svc.Start(context.Background(), fx.startRequest())
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	if w.ID == 0 {
		t.Fatal("Start() answered with no watch")
	}
	st, err := store.Open(fx.dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	got, err := st.GetWatch(context.Background(), w.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != store.WatchActive {
		t.Fatalf("the watch the daemon polls is %q, and the author saw a failure", got.Status)
	}
}
