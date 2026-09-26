package prwatch

import (
	"context"
	"database/sql"
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/deividfortuna/babysitter/internal/agent"
	"github.com/deividfortuna/babysitter/internal/store"
)

func (fx *fixture) handback(w store.Watch, o HandbackOptions) store.Watch {
	fx.t.Helper()
	got, err := fx.svc.Handback(context.Background(), w.ID, o)
	if err != nil {
		fx.t.Fatalf("Handback() error = %v", err)
	}
	fx.svc.wg.Wait()
	return got
}

func (fx *fixture) lastKind(w store.Watch, kind store.ActivityKind) store.Activity {
	fx.t.Helper()
	var out store.Activity
	for _, a := range fx.activity(w) {
		if a.Kind == kind {
			out = a
		}
	}
	return out
}

func TestAHandBackWithWorkThatIsNotPushedAsksFirst(t *testing.T) {
	t.Parallel()
	fx := newFixture(t)
	w := fx.start()
	fx.takeover(w)
	sessions := fx.host.count()
	fx.rel.commit("abc", "a1")
	fx.rel.commit("a1", "a2")
	fx.rel.set(func(f *fakeRelease) { f.dirty = []string{" M x.go"} })

	_, err := fx.svc.Handback(context.Background(), w.ID, HandbackOptions{})

	var work *WorkError
	if !errors.Is(err, ErrUnconfirmedWork) || !errors.As(err, &work) {
		t.Fatalf("Handback() error = %v, want the work to confirm", err)
	}
	if len(work.Commits) != 2 || work.Commits[0].SHA != "a1" || work.Commits[1].SHA != "a2" || !slices.Equal(work.Files, []string{" M x.go"}) {
		t.Fatalf("work = %+v", work)
	}
	if fx.watch(w).TakenOverAt == nil || fx.host.count() != sessions {
		t.Fatal("the refused hand-back changed the watch")
	}

	got := fx.handback(w, HandbackOptions{Confirm: true})

	if got.TakenOverAt != nil || got.TakenOverPID != 0 {
		t.Fatalf("watch = %+v", got)
	}
	if row := fx.lastKind(w, store.ActivityHandedBack); row.ID == 0 || !strings.Contains(row.Summary, "2 commits") || !strings.Contains(row.Summary, "1 change that is not committed") {
		t.Fatalf("handed_back row = %+v", row)
	}
}

func TestAHandBackWhileTheAgentOfTheAuthorRunsIsRefused(t *testing.T) {
	t.Parallel()
	fx := newFixture(t)
	w := fx.start()
	fx.takeover(w)
	fx.authorAlive.Store(true)

	_, err := fx.svc.Handback(context.Background(), w.ID, HandbackOptions{Confirm: true})

	if !errors.Is(err, ErrAuthorRunning) || !strings.Contains(err.Error(), "pid 4242") {
		t.Fatalf("Handback() error = %v, want ErrAuthorRunning with the pid", err)
	}
	if fx.watch(w).TakenOverAt == nil {
		t.Fatal("the refused hand-back gave the watch back")
	}

	if got := fx.handback(w, HandbackOptions{Force: true}); got.TakenOverAt != nil {
		t.Fatalf("a forced hand-back = %+v", got)
	}
}

func TestAHandBackOfAWatchNotTakenOverIsRefused(t *testing.T) {
	t.Parallel()
	fx := newFixture(t)
	w := fx.start()
	if _, err := fx.svc.Handback(context.Background(), w.ID, HandbackOptions{}); !errors.Is(err, ErrNotTakenOver) {
		t.Fatalf("Handback() error = %v, want ErrNotTakenOver", err)
	}
	fx.takeover(w)
	if _, err := fx.svc.Stop(context.Background(), w.ID, StopOptions{}); err != nil {
		t.Fatal(err)
	}
	if _, err := fx.svc.Handback(context.Background(), w.ID, HandbackOptions{}); !errors.Is(err, ErrWatchStopped) {
		t.Fatalf("Handback() of a stopped watch = %v, want ErrWatchStopped", err)
	}
}

func TestAHandBackOfDependabotRefusesCommitsTheDaemonNeverPushes(t *testing.T) {
	t.Parallel()
	fx := newFixture(t)
	fx.update(func() { fx.pr.Author = "dependabot[bot]" })
	w := fx.start()
	fx.takeover(w)
	fx.rel.commit("abc", "a1")

	_, err := fx.svc.Handback(context.Background(), w.ID, HandbackOptions{Confirm: true})

	if !errors.Is(err, ErrUnpushedCommits) || !strings.Contains(err.Error(), "push") {
		t.Fatalf("Handback() error = %v, want ErrUnpushedCommits", err)
	}
	if fx.watch(w).TakenOverAt == nil {
		t.Fatal("the refused hand-back gave the watch back")
	}
}

func TestTheAgentContinuesTheConversationAfterAHandBack(t *testing.T) {
	t.Parallel()
	fx := newFixture(t)
	w := fx.start()
	fx.takeover(w)
	fx.comment()
	fx.poll(w)
	sessions := fx.host.count()

	fx.handback(w, HandbackOptions{})

	if fx.host.count() != sessions+1 {
		t.Fatalf("the hand-back started %d sessions", fx.host.count()-sessions)
	}
	h := fx.host.last()
	if !slices.Contains(h.spec.Argv, "--resume") || !slices.Contains(h.spec.Argv, fx.watch(w).AgentSession) {
		t.Fatalf("argv = %v", h.spec.Argv)
	}
	if msgs := h.messages(); len(msgs) != 1 || !strings.Contains(msgs[0], "gave the session of PR #3") {
		t.Fatalf("messages = %q", msgs)
	}

	fx.hook(w, agent.EventUserPromptSubmit, `{}`)
	fx.hook(w, agent.EventStop, `{}`)
	fx.poll(w)

	msgs := h.messages()
	if len(msgs) != 2 || !strings.Contains(msgs[1], "rename this") {
		t.Fatalf("messages = %q", msgs)
	}
	if fx.host.count() != sessions+1 {
		t.Fatal("the held comment started another session")
	}
}

func TestATurnAfterAHandBackPushesTheCommitsOfTheAuthor(t *testing.T) {
	t.Parallel()
	fx := newFixture(t)
	w := fx.start()
	fx.takeover(w)
	fx.rel.commit("abc", "a1")
	fx.handback(w, HandbackOptions{Confirm: true})

	fx.hook(w, agent.EventUserPromptSubmit, `{}`)
	fx.hook(w, agent.EventStop, `{}`)

	if pushes := fx.rel.pushed(); len(pushes) != 1 || pushes[0].SHA != "a1" {
		t.Fatalf("pushes = %+v", pushes)
	}
	if got := fx.watch(w).HandbackStart; got != "" {
		t.Fatalf("the start of the hand-back stays: %q", got)
	}
}

func (fx *fixture) refuse(name, event string) (allow func()) {
	fx.t.Helper()
	db, err := sql.Open("sqlite3", "file:"+fx.dbPath+"?_busy_timeout=5000")
	if err != nil {
		fx.t.Fatal(err)
	}
	fx.t.Cleanup(func() { db.Close() })
	if _, err := db.Exec("CREATE TRIGGER " + name + " BEFORE " + event + " BEGIN SELECT RAISE(ABORT, 'the write is refused'); END"); err != nil {
		fx.t.Fatalf("create trigger: %v", err)
	}
	return func() {
		if _, err := db.Exec("DROP TRIGGER " + name); err != nil {
			fx.t.Fatalf("drop trigger: %v", err)
		}
	}
}

func (fx *fixture) refuseProposals() (allow func()) {
	return fx.refuse("proposals_refused", "INSERT ON proposals")
}

func TestAHandBackThatFailsToStoreTheStartOfTheTurnKeepsTheSessionWithTheAuthor(t *testing.T) {
	t.Parallel()
	fx := newFixture(t)
	w := fx.start()
	fx.takeover(w)
	fx.rel.commit("abc", "a1")

	allow := fx.refuse("handback_start_refused", "UPDATE OF handback_start ON watches WHEN NEW.handback_start != ''")
	if _, err := fx.svc.Handback(context.Background(), w.ID, HandbackOptions{Confirm: true}); err == nil {
		t.Fatal("Handback() stored no start and gave no error")
	}
	if fx.watch(w).TakenOverAt == nil {
		t.Fatal("the failed hand-back took the session from the author")
	}
	allow()

	fx.handback(w, HandbackOptions{Confirm: true})
	fx.hook(w, agent.EventUserPromptSubmit, `{}`)
	fx.hook(w, agent.EventStop, `{}`)

	if pushes := fx.rel.pushed(); len(pushes) != 1 || pushes[0].SHA != "a1" {
		t.Fatalf("pushes = %+v", pushes)
	}
}

func TestATurnAfterAHandBackKeepsItsStartWhenTheProposalFailsToOpen(t *testing.T) {
	t.Parallel()
	fx := newFixture(t)
	w := fx.start()
	fx.takeover(w)
	fx.rel.commit("abc", "a1")
	fx.handback(w, HandbackOptions{Confirm: true})

	allow := fx.refuseProposals()
	fx.hook(w, agent.EventUserPromptSubmit, `{}`)
	fx.hook(w, agent.EventStop, `{}`)
	allow()
	fx.hook(w, agent.EventUserPromptSubmit, `{}`)
	fx.hook(w, agent.EventStop, `{}`)

	if pushes := fx.rel.pushed(); len(pushes) != 1 || pushes[0].SHA != "a1" {
		t.Fatalf("pushes = %+v", pushes)
	}
}

func TestAHandBackThatFailsToRecordKeepsTheSessionWithTheAuthor(t *testing.T) {
	t.Parallel()
	fx := newFixture(t)
	w := fx.start()
	fx.takeover(w)

	allow := fx.refuse("handed_back_refused", "INSERT ON watch_activity WHEN NEW.kind = 'handed_back'")
	if _, err := fx.svc.Handback(context.Background(), w.ID, HandbackOptions{}); err == nil {
		t.Fatal("Handback() recorded nothing and gave no error")
	}
	if fx.watch(w).TakenOverAt == nil {
		t.Fatal("the failed hand-back took the session from the author")
	}
	allow()

	fx.handback(w, HandbackOptions{})

	if msgs := fx.host.last().messages(); len(msgs) != 1 || !strings.Contains(msgs[0], "gave the session of PR #3") {
		t.Fatalf("messages = %q", msgs)
	}
}

func TestAHandBackThatTheAgentMissedGoesOutWithTheNextPoll(t *testing.T) {
	t.Parallel()
	fx := newFixture(t)
	w := fx.start()
	fx.takeover(w)
	fx.svc.wg.Wait()
	runner := fx.svc.agents[ProviderClaude].(*fakeRunner)
	runner.err = errors.New("the agent cannot start")
	sessions := fx.host.count()

	fx.handback(w, HandbackOptions{})
	if fx.host.count() != sessions {
		t.Fatal("a session started although the agent cannot start")
	}
	runner.err = nil
	fx.poll(w)
	fx.poll(w)

	if fx.host.count() != sessions+1 {
		t.Fatalf("the polls started %d sessions", fx.host.count()-sessions)
	}
	if msgs := fx.host.last().messages(); len(msgs) != 1 || !strings.Contains(msgs[0], "gave the session of PR #3") {
		t.Fatalf("messages = %q", msgs)
	}
}

func TestAHandBackThatIsDoneIsNotReportedAsAFailure(t *testing.T) {
	t.Parallel()
	fx := newFixture(t)
	w := fx.start()
	fx.takeover(w)
	fx.svc.wg.Wait()
	fx.svc.agents[ProviderClaude].(*fakeRunner).err = errors.New("the agent cannot start")
	fx.notes.onSend = func() { fx.st.Close() }

	got, err := fx.svc.Handback(context.Background(), w.ID, HandbackOptions{})
	fx.svc.wg.Wait()

	if err != nil || got.TakenOverAt != nil {
		t.Fatalf("Handback() = %+v, %v; the hand-back is done", got, err)
	}
}

func TestATurnAfterAHandBackPushesItsCommitWithThoseOfTheAuthor(t *testing.T) {
	t.Parallel()
	fx := newFixture(t)
	w := fx.start()
	fx.takeover(w)
	fx.rel.commit("abc", "a1")
	fx.handback(w, HandbackOptions{Confirm: true})

	fx.hook(w, agent.EventUserPromptSubmit, `{}`)
	fx.rel.commit("a1", "w1")
	fx.hook(w, agent.EventStop, `{}`)

	if pushes := fx.rel.pushed(); len(pushes) != 1 || pushes[0].SHA != "w1" {
		t.Fatalf("pushes = %+v", pushes)
	}
}
