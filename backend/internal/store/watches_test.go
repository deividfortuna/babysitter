package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/deividfortuna/babysitter/internal/checks"

	"github.com/deividfortuna/babysitter/internal/events"
)

func TestMigration4KeepsWatchTables(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "babysitter.db")
	db, err := sql.Open("sqlite3", "file:"+path+"?_foreign_keys=on")
	if err != nil {
		t.Fatal(err)
	}
	if err := migrateTo(ctx, db, 3); err != nil {
		t.Fatal(err)
	}
	for _, q := range []string{
		"INSERT INTO pr_watch VALUES ('octo', 'hello', 3, '2026-09-01T00:00:00Z', '2026-09-02T00:00:00Z', 'abc')",
		"INSERT INTO pr_watch_seen VALUES ('octo', 'hello', 3, 'review', 1, '2026-09-02T00:00:00Z')",
	} {
		if _, err := db.ExecContext(ctx, q); err != nil {
			t.Fatalf("%s: %v", q, err)
		}
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	s, err := Open(path)
	if err != nil {
		t.Fatalf("Open() after version 3 error = %v", err)
	}
	defer s.Close()
	seen, err := s.SeenReviewItems(ctx, WatchKey{Owner: "octo", Name: "hello", Number: 3})
	if err != nil || len(seen) != 1 {
		t.Fatalf("seen = %v, %v", seen, err)
	}
	if ws, err := s.ListWatches(ctx, ListWatchesOptions{}); err != nil || len(ws) != 0 {
		t.Fatalf("watches = %v, %v", ws, err)
	}
}

type recordingPublisher struct {
	types  []events.Type
	events []events.Event
}

func (p *recordingPublisher) Publish(t events.Type, repo string, number int) events.Event {
	p.types = append(p.types, t)
	e := events.Event{Type: t, Repo: repo, Number: number}
	p.events = append(p.events, e)
	return e
}

func (p *recordingPublisher) last() events.Event {
	if len(p.events) == 0 {
		return events.Event{}
	}
	return p.events[len(p.events)-1]
}

func newWatch(now time.Time) Watch {
	return Watch{
		Owner: "Octo", Name: "Hello", Number: 3, URL: "https://github.com/octo/hello/pull/3",
		Title: "Fix the thing", Author: "alice", BotLogin: "alice", HeadRef: "fix", BaseRef: "main",
		SourceDir: "/src", WorktreeDir: "/wt", WorkBranch: "babysitter/fix",
		GitUserName: "Alice", GitUserEmail: "alice@example.com", StartedAt: now, HeadSHA: "abc",
	}
}

func TestWatchLifecycle(t *testing.T) {
	t.Parallel()
	s, _ := openTemp(t)
	pub := &recordingPublisher{}
	s.SetPublisher(pub)
	ctx := context.Background()
	now := time.Date(2026, 9, 7, 12, 0, 0, 0, time.UTC)

	w, err := s.CreateWatch(ctx, newWatch(now))
	if err != nil {
		t.Fatal(err)
	}
	if w.ID == 0 || w.Status != WatchActive || !w.StartedAt.Equal(now) || w.CheckStates == nil || string(w.Summary) != "{}" {
		t.Fatalf("created watch = %+v", w)
	}
	if _, err := s.CreateWatch(ctx, Watch{Owner: "octo", Name: "hello", Number: 3, StartedAt: now}); !errors.Is(err, ErrWatchExists) {
		t.Fatalf("second CreateWatch error = %v, want ErrWatchExists", err)
	}
	found, err := s.FindActiveWatch(ctx, WatchKey{Owner: "OCTO", Name: "hello", Number: 3})
	if err != nil || found.ID != w.ID {
		t.Fatalf("FindActiveWatch = %+v, %v", found, err)
	}
	if _, err := s.FindActiveWatch(ctx, WatchKey{Owner: "octo", Name: "hello", Number: 4}); !errors.Is(err, ErrWatchNotFound) {
		t.Fatalf("FindActiveWatch other error = %v", err)
	}

	polled := now.Add(time.Minute)
	err = s.UpdateWatchState(ctx, w.ID, WatchState{
		HeadSHA: "def", PRState: "open", MergeableState: "clean",
		CheckStates: map[string]checks.State{"build": checks.Passed}, GreenSHA: "def", PolledAt: polled,
	})
	if err != nil {
		t.Fatal(err)
	}
	if n, err := s.SetWatchError(ctx, w.ID, errors.New("boom"), polled); err != nil || n != 1 {
		t.Fatalf("SetWatchError = %d, %v", n, err)
	}
	if n, _ := s.SetWatchError(ctx, w.ID, errors.New("boom"), polled); n != 2 {
		t.Fatalf("second SetWatchError = %d", n)
	}
	got, err := s.GetWatch(ctx, w.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.HeadSHA != "def" || got.MergeableState != "clean" || got.CheckStates["build"] != "passed" || got.GreenSHA != "def" ||
		got.LastPollAt == nil || !got.LastPollAt.Equal(polled) || got.LastError != "boom" || got.ConsecutiveErrors != 2 {
		t.Fatalf("watch after poll = %+v", got)
	}
	if err := s.UpdateWatchState(ctx, w.ID, WatchState{HeadSHA: "def", PolledAt: polled}); err != nil {
		t.Fatal(err)
	}
	if got, _ := s.GetWatch(ctx, w.ID); got.ConsecutiveErrors != 0 || got.LastError != "" {
		t.Fatalf("watch after good poll = %+v", got)
	}
	if err := s.SetWatchHeartbeat(ctx, w.ID, polled); err != nil {
		t.Fatal(err)
	}
	if got, _ := s.GetWatch(ctx, w.ID); got.LastHeartbeatAt == nil || !got.LastHeartbeatAt.Equal(polled) {
		t.Fatalf("heartbeat = %+v", got.LastHeartbeatAt)
	}

	if _, err := s.CreateWatch(ctx, Watch{Owner: "octo", Name: "hello", Number: 4, StartedAt: now}); err != nil {
		t.Fatal(err)
	}
	active, err := s.ListWatches(ctx, ListWatchesOptions{Status: WatchActive})
	if err != nil || len(active) != 2 {
		t.Fatalf("active = %v, %v", active, err)
	}

	stopped, err := s.StopWatch(ctx, w.ID, StopMerged, json.RawMessage(`{"landed":1}`), polled)
	if err != nil {
		t.Fatal(err)
	}
	if stopped.Status != WatchStopped || stopped.StopReason != StopMerged || stopped.StoppedAt == nil || string(stopped.Summary) != `{"landed":1}` {
		t.Fatalf("stopped = %+v", stopped)
	}
	before := len(pub.types)
	if again, err := s.StopWatch(ctx, w.ID, StopUser, nil, polled.Add(time.Hour)); err != nil || again.StopReason != StopMerged {
		t.Fatalf("second stop = %+v, %v", again, err)
	}
	if len(pub.types) != before {
		t.Fatalf("second stop published %v", pub.types[before:])
	}
	if _, err := s.CreateWatch(ctx, newWatch(polled)); err != nil {
		t.Fatalf("CreateWatch after stop error = %v", err)
	}
	active, _ = s.ListWatches(ctx, ListWatchesOptions{Status: WatchActive})
	all, _ := s.ListWatches(ctx, ListWatchesOptions{})
	if len(active) != 2 || len(all) != 3 {
		t.Fatalf("active = %d, all = %d", len(active), len(all))
	}
	if _, err := s.GetWatch(ctx, 999); !errors.Is(err, ErrWatchNotFound) {
		t.Fatalf("GetWatch missing error = %v", err)
	}

	wantTypes := []events.Type{events.WatchStarted, events.WatchStarted, events.WatchStopped, events.WatchStarted}
	if len(pub.types) != len(wantTypes) {
		t.Fatalf("published %v, want %v", pub.types, wantTypes)
	}
	for i := range wantTypes {
		if pub.types[i] != wantTypes[i] {
			t.Fatalf("published %v, want %v", pub.types, wantTypes)
		}
	}
}

func TestActivityDedupeAndReporting(t *testing.T) {
	t.Parallel()
	s, _ := openTemp(t)
	ctx := context.Background()
	now := time.Date(2026, 9, 7, 12, 0, 0, 0, time.UTC)
	w, err := s.CreateWatch(ctx, newWatch(now))
	if err != nil {
		t.Fatal(err)
	}

	a := Activity{WatchID: w.ID, Kind: ActivityComment, Ref: "11", At: now, Actor: "bob", Summary: "please fix", URL: "https://x/1"}
	first, inserted, err := s.InsertActivity(ctx, a)
	if err != nil || !inserted || first.ID == 0 || string(first.Payload) != "{}" {
		t.Fatalf("first insert = %+v, %v, %v", first, inserted, err)
	}
	again, inserted, err := s.InsertActivity(ctx, a)
	if err != nil || inserted || again.ID != first.ID {
		t.Fatalf("second insert = %+v, %v, %v", again, inserted, err)
	}
	if _, inserted, err := s.InsertActivity(ctx, Activity{WatchID: w.ID, Kind: ActivityReview, Ref: "11", At: now}); err != nil || !inserted {
		t.Fatalf("other kind insert = %v, %v", inserted, err)
	}
	if _, _, err := s.InsertActivity(ctx, Activity{WatchID: w.ID, Kind: "bogus", Ref: "1", At: now}); err == nil {
		t.Fatal("bogus kind expected a check constraint error")
	}
	if _, _, err := s.InsertActivity(ctx, Activity{WatchID: 999, Kind: ActivityComment, Ref: "1", At: now}); err == nil {
		t.Fatal("unknown watch expected a foreign key error")
	}

	unreported, err := s.UnreportedActivity(ctx, w.ID)
	if err != nil || len(unreported) != 2 {
		t.Fatalf("unreported = %v, %v", unreported, err)
	}
	if err := s.MarkActivityReported(ctx, []int64{first.ID}, now); err != nil {
		t.Fatal(err)
	}
	unreported, _ = s.UnreportedActivity(ctx, w.ID)
	if len(unreported) != 1 || unreported[0].Kind != ActivityReview {
		t.Fatalf("unreported after mark = %v", unreported)
	}
	all, err := s.ListActivity(ctx, w.ID, 0, 0)
	if err != nil || len(all) != 2 || !all[0].Reported || all[0].ReportedAt == nil || all[1].Reported {
		t.Fatalf("all = %+v, %v", all, err)
	}
	since, _ := s.ListActivity(ctx, w.ID, first.ID, 10)
	if len(since) != 1 || since[0].Kind != ActivityReview {
		t.Fatalf("since = %+v", since)
	}
	counts, err := s.CountActivity(ctx, w.ID)
	if err != nil || counts[ActivityComment] != 1 || counts[ActivityReview] != 1 {
		t.Fatalf("counts = %v, %v", counts, err)
	}

	if _, err := s.StopWatch(ctx, w.ID, StopUser, nil, now); err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteWatch(ctx, w.ID); err != nil {
		t.Fatal(err)
	}
	if rows, _ := s.ListActivity(ctx, w.ID, 0, 0); len(rows) != 0 {
		t.Fatalf("activity after delete = %v", rows)
	}
}

func TestNudgeTracking(t *testing.T) {
	t.Parallel()
	s, _ := openTemp(t)
	ctx := context.Background()
	now := time.Date(2026, 9, 7, 12, 0, 0, 0, time.UTC)
	w, err := s.CreateWatch(ctx, newWatch(now))
	if err != nil {
		t.Fatal(err)
	}
	comment, _, err := s.InsertActivity(ctx, Activity{WatchID: w.ID, Kind: ActivityComment, Ref: "11", At: now, Actor: "bob"})
	if err != nil {
		t.Fatal(err)
	}
	failed, _, err := s.InsertActivity(ctx, Activity{WatchID: w.ID, Kind: ActivityCheckFailed, Ref: "build@abc", At: now})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.InsertActivity(ctx, Activity{WatchID: w.ID, Kind: ActivityCommit, Ref: "def", At: now}); err != nil {
		t.Fatal(err)
	}
	todo, err := s.UnnudgedActionable(ctx, w.ID)
	if err != nil || len(todo) != 2 || todo[0].ID != comment.ID || todo[1].ID != failed.ID {
		t.Fatalf("unnudged = %+v, %v", todo, err)
	}
	if err := s.MarkActivityNudged(ctx, []int64{comment.ID}, now.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	todo, _ = s.UnnudgedActionable(ctx, w.ID)
	if len(todo) != 1 || todo[0].ID != failed.ID {
		t.Fatalf("unnudged after mark = %+v", todo)
	}
	rows, _ := s.ListActivity(ctx, w.ID, 0, 0)
	if rows[0].NudgedAt == nil || !rows[0].NudgedAt.Equal(now.Add(time.Minute)) || rows[1].NudgedAt != nil {
		t.Fatalf("nudged_at = %v, %v", rows[0].NudgedAt, rows[1].NudgedAt)
	}
	if err := s.SetWatchAgentSession(ctx, w.ID, "sess-1"); err != nil {
		t.Fatal(err)
	}
	if got, _ := s.GetWatch(ctx, w.ID); got.AgentSession != "sess-1" {
		t.Fatalf("agent session = %q", got.AgentSession)
	}
	reset := now.Add(3 * time.Hour)
	if err := s.SetWatchAgentLimit(ctx, w.ID, &reset); err != nil {
		t.Fatal(err)
	}
	if got, _ := s.GetWatch(ctx, w.ID); got.AgentLimitedUntil == nil || !got.AgentLimitedUntil.Equal(reset) {
		t.Fatalf("agent limited until = %v", got.AgentLimitedUntil)
	}
	if err := s.SetWatchAgentLimit(ctx, w.ID, nil); err != nil {
		t.Fatal(err)
	}
	if got, _ := s.GetWatch(ctx, w.ID); got.AgentLimitedUntil != nil {
		t.Fatalf("agent limited until after the clear = %v", got.AgentLimitedUntil)
	}
}

func TestMigration12DropsTheProposals(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "babysitter.db")
	db, err := sql.Open("sqlite3", "file:"+path+"?_foreign_keys=on")
	if err != nil {
		t.Fatal(err)
	}
	if err := migrateTo(ctx, db, 11); err != nil {
		t.Fatal(err)
	}
	for _, q := range []string{
		`INSERT INTO watches (owner, name, number, url, title, author, bot_login, head_ref, base_ref,
			source_dir, worktree_dir, work_branch, git_user_name, git_user_email, status, started_at, head_sha)
		 VALUES ('octo', 'hello', 3, 'u', 't', 'alice', 'alice', 'fix', 'main', '/s', '/w', 'b', 'Alice', 'a@e', 'active', '2026-09-07T12:00:00Z', 'abc')`,
		`INSERT INTO watch_activity (id, watch_id, kind, ref, at, summary) VALUES (5, 1, 'review_comment', '31', '2026-09-07T12:01:00Z', 'bob commented')`,
		`INSERT INTO watch_activity (id, watch_id, kind, ref, at, summary) VALUES (6, 1, 'proposal_ready', '9/1', '2026-09-07T12:02:00Z', 'proposal 9 ready')`,
		`INSERT INTO proposals (id, watch_id, kind, status, created_at, updated_at)
		 VALUES (9, 1, 'fix', 'landed', '2026-09-07T12:02:00Z', '2026-09-07T12:02:00Z')`,
		`INSERT INTO proposal_activity (proposal_id, activity_id) VALUES (9, 5)`,
	} {
		if _, err := db.ExecContext(ctx, q); err != nil {
			t.Fatalf("%s: %v", q, err)
		}
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	s, err := Open(path)
	if err != nil {
		t.Fatalf("Open() after version 11 error = %v", err)
	}
	defer s.Close()
	rows, err := s.ListActivity(ctx, 1, 0, 0)
	if err != nil || len(rows) != 1 || rows[0].ID != 5 || rows[0].NudgedAt != nil {
		t.Fatalf("activity = %+v, %v", rows, err)
	}
	todo, err := s.UnnudgedActionable(ctx, 1)
	if err != nil || len(todo) != 1 {
		t.Fatalf("unnudged = %+v, %v", todo, err)
	}
	var n int
	if err := s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM sqlite_master WHERE name = 'proposal_activity'").Scan(&n); err != nil || n != 0 {
		t.Fatalf("proposal tables left = %d, %v", n, err)
	}
	if w, err := s.GetWatch(ctx, 1); err != nil || w.AgentSession != "" {
		t.Fatalf("watch = %+v, %v", w, err)
	}
}

func TestMigration13KeepsTheNudgeMark(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "babysitter.db")
	db, err := sql.Open("sqlite3", "file:"+path+"?_foreign_keys=on")
	if err != nil {
		t.Fatal(err)
	}
	if err := migrateTo(ctx, db, 12); err != nil {
		t.Fatal(err)
	}
	for _, q := range []string{
		`INSERT INTO watches (owner, name, number, url, title, author, bot_login, head_ref, base_ref,
			source_dir, worktree_dir, work_branch, git_user_name, git_user_email, status, started_at, head_sha)
		 VALUES ('octo', 'hello', 3, 'u', 't', 'alice', 'alice', 'fix', 'main', '/s', '/w', 'b', 'Alice', 'a@e', 'active', '2026-09-07T12:00:00Z', 'abc')`,
		`INSERT INTO watch_activity (id, watch_id, kind, ref, at, summary, nudged_at) VALUES (5, 1, 'review_comment', '31', '2026-09-07T12:01:00Z', 'bob commented', '2026-09-07T12:02:00Z')`,
	} {
		if _, err := db.ExecContext(ctx, q); err != nil {
			t.Fatalf("%s: %v", q, err)
		}
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	s, err := Open(path)
	if err != nil {
		t.Fatalf("Open() after version 12 error = %v", err)
	}
	defer s.Close()
	rows, err := s.ListActivity(ctx, 1, 0, 0)
	if err != nil || len(rows) != 1 || rows[0].ID != 5 || rows[0].NudgedAt == nil {
		t.Fatalf("activity = %+v, %v", rows, err)
	}
	w, err := s.GetWatch(ctx, 1)
	if err != nil || w.ApprovalsRequired != 1 || w.MergeMethod != "" || w.ReadySince != nil || len(w.ReadyBlockers) != 0 {
		t.Fatalf("watch = %+v, %v", w, err)
	}
	if _, _, err := s.InsertActivity(ctx, Activity{WatchID: 1, Kind: ActivityMergeReady, Ref: "ready@abc", At: time.Now()}); err != nil {
		t.Fatalf("InsertActivity(merge_ready) error = %v", err)
	}
}

func TestWatchReadinessPublishesEachChange(t *testing.T) {
	t.Parallel()
	s, _ := openTemp(t)
	pub := &recordingPublisher{}
	s.SetPublisher(pub)
	ctx := context.Background()
	now := time.Date(2026, 9, 7, 12, 0, 0, 0, time.UTC)
	w, err := s.CreateWatch(ctx, newWatch(now))
	if err != nil {
		t.Fatal(err)
	}
	if err := s.SetWatchReadiness(ctx, w.ID, nil, []string{"no approval yet"}); err != nil {
		t.Fatal(err)
	}
	if err := s.SetWatchReadiness(ctx, w.ID, nil, []string{"no approval yet"}); err != nil {
		t.Fatal(err)
	}
	if err := s.SetWatchReadiness(ctx, w.ID, &now, nil); err != nil {
		t.Fatal(err)
	}
	if err := s.SetWatchReadiness(ctx, w.ID, &now, nil); err != nil {
		t.Fatal(err)
	}
	want := []events.Type{events.WatchStarted, events.WatchReady, events.WatchReady}
	if !slices.Equal(pub.types, want) {
		t.Fatalf("events = %v, want %v: one per change of the readiness, none for a repeat", pub.types, want)
	}
	if last := pub.last(); last.Repo != w.Repo() || last.Number != w.Number {
		t.Fatalf("last event = %+v", last)
	}
}

func TestWatchReadiness(t *testing.T) {
	t.Parallel()
	s, _ := openTemp(t)
	ctx := context.Background()
	now := time.Date(2026, 9, 7, 12, 0, 0, 0, time.UTC)

	w := newWatch(now)
	w.ApprovalsRequired, w.MergeMethod = 2, "rebase"
	w, err := s.CreateWatch(ctx, w)
	if err != nil {
		t.Fatal(err)
	}
	if w.ApprovalsRequired != 2 || w.MergeMethod != "rebase" || w.ReadySince != nil || len(w.ReadyBlockers) != 0 {
		t.Fatalf("created watch = %+v", w)
	}
	if err := s.SetWatchReadiness(ctx, w.ID, nil, []string{"no approval yet", "2 checks pending"}); err != nil {
		t.Fatal(err)
	}
	w, err = s.GetWatch(ctx, w.ID)
	if err != nil || w.ReadySince != nil || strings.Join(w.ReadyBlockers, "; ") != "no approval yet; 2 checks pending" {
		t.Fatalf("blocked watch = %+v, %v", w, err)
	}
	since := now.Add(time.Minute)
	if err := s.SetWatchReadiness(ctx, w.ID, &since, nil); err != nil {
		t.Fatal(err)
	}
	w, err = s.GetWatch(ctx, w.ID)
	if err != nil || w.ReadySince == nil || !w.ReadySince.Equal(since) || len(w.ReadyBlockers) != 0 {
		t.Fatalf("ready watch = %+v, %v", w, err)
	}
	if _, err := s.CreateWatch(ctx, Watch{Owner: "octo", Name: "hello", Number: 4, StartedAt: now}); err != nil {
		t.Fatal(err)
	}
	other, err := s.FindActiveWatch(ctx, WatchKey{Owner: "octo", Name: "hello", Number: 4})
	if err != nil || other.ApprovalsRequired != 0 {
		t.Fatalf("approvals of a watch without a count = %+v, %v", other, err)
	}
}

func TestTakingBackANudge(t *testing.T) {
	t.Parallel()
	s, _ := openTemp(t)
	ctx := context.Background()
	now := time.Date(2026, 9, 7, 12, 0, 0, 0, time.UTC)
	w, err := s.CreateWatch(ctx, newWatch(now))
	if err != nil {
		t.Fatal(err)
	}
	comment, _, err := s.InsertActivity(ctx, Activity{WatchID: w.ID, Kind: ActivityComment, Ref: "11", At: now, Actor: "bob"})
	if err != nil {
		t.Fatal(err)
	}
	nudge, _, err := s.InsertActivity(ctx, Activity{WatchID: w.ID, Kind: ActivityNudged, Ref: "11@" + now.Format(time.RFC3339Nano), At: now})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.MarkActivityNudged(ctx, []int64{comment.ID}, now); err != nil {
		t.Fatal(err)
	}

	if err := s.UnmarkActivityNudged(ctx, []int64{comment.ID}); err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteActivity(ctx, nudge.ID); err != nil {
		t.Fatal(err)
	}
	todo, err := s.UnnudgedActionable(ctx, w.ID)
	if err != nil || len(todo) != 1 || todo[0].ID != comment.ID {
		t.Fatalf("unnudged after taking the nudge back = %+v, %v", todo, err)
	}
	rows, _ := s.ListActivity(ctx, w.ID, 0, 0)
	if len(rows) != 1 || rows[0].ID != comment.ID || rows[0].NudgedAt != nil {
		t.Fatalf("activity after taking the nudge back = %+v", rows)
	}
	if err := s.UnmarkActivityNudged(ctx, nil); err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteActivity(ctx, nudge.ID); err != nil {
		t.Fatal(err)
	}
}

func TestHasActivity(t *testing.T) {
	t.Parallel()
	s, _ := openTemp(t)
	ctx := context.Background()
	now := time.Date(2026, 9, 20, 12, 0, 0, 0, time.UTC)
	w, err := s.CreateWatch(ctx, newWatch(now))
	if err != nil {
		t.Fatal(err)
	}
	other, err := s.CreateWatch(ctx, Watch{
		Owner: "Octo", Name: "Hello", Number: 4, URL: "https://github.com/octo/hello/pull/4",
		Author: "alice", BotLogin: "alice", HeadRef: "other", BaseRef: "main",
		SourceDir: "/src", WorktreeDir: "/wt2", WorkBranch: "babysitter/other",
		GitUserName: "Alice", GitUserEmail: "alice@example.com", StartedAt: now, HeadSHA: "abc",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.InsertActivity(ctx, Activity{WatchID: w.ID, Kind: ActivityReviewRequested, Ref: "abc", At: now}); err != nil {
		t.Fatal(err)
	}

	has, err := s.HasActivity(ctx, w.ID, ActivityReviewRequested, "abc")
	if err != nil || !has {
		t.Fatalf("HasActivity(the row) = %v, %v, want true", has, err)
	}
	has, err = s.HasActivity(ctx, w.ID, ActivityReviewRequested, "def")
	if err != nil || has {
		t.Fatalf("HasActivity(another ref) = %v, %v, want false", has, err)
	}
	has, err = s.HasActivity(ctx, w.ID, ActivityMergeReady, "abc")
	if err != nil || has {
		t.Fatalf("HasActivity(another kind) = %v, %v, want false", has, err)
	}
	has, err = s.HasActivity(ctx, other.ID, ActivityReviewRequested, "abc")
	if err != nil || has {
		t.Fatalf("HasActivity(another watch) = %v, %v, want false", has, err)
	}
}

func TestSetActivityJobWritesTheUrlWithThePayload(t *testing.T) {
	t.Parallel()
	s, _ := openTemp(t)
	ctx := context.Background()
	now := time.Date(2026, 9, 22, 12, 0, 0, 0, time.UTC)
	w, err := s.CreateWatch(ctx, newWatch(now))
	if err != nil {
		t.Fatal(err)
	}
	row, _, err := s.InsertActivity(ctx, Activity{
		WatchID: w.ID, Kind: ActivityCheckFailed, Ref: "build@abc", At: now, Summary: "build failed on abc",
		URL:     "https://ci/77/9",
		Payload: json.RawMessage(`{"check":"build","job_id":9}`),
	})
	if err != nil {
		t.Fatal(err)
	}

	if err := s.SetActivityJob(ctx, row.ID, json.RawMessage(`{"check":"build","job_id":10}`), "https://ci/77/10"); err != nil {
		t.Fatal(err)
	}
	got := oneActivity(t, s, w.ID)
	if got.URL != "https://ci/77/10" || !strings.Contains(string(got.Payload), `"job_id":10`) {
		t.Fatalf("row = %s %s, want the url and the payload of job 10", got.URL, got.Payload)
	}

	if err := s.SetActivityJob(ctx, row.ID, json.RawMessage(`{"check":"build","job_id":11}`), ""); err != nil {
		t.Fatal(err)
	}
	got = oneActivity(t, s, w.ID)
	if got.URL != "https://ci/77/10" || !strings.Contains(string(got.Payload), `"job_id":11`) {
		t.Fatalf("row = %s %s, want the payload of job 11 and the url it had", got.URL, got.Payload)
	}
}

func oneActivity(t *testing.T, s *Store, watchID int64) Activity {
	t.Helper()
	rows, err := s.ListActivity(context.Background(), watchID, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 {
		t.Fatalf("the watch has %d activity rows, want 1", len(rows))
	}
	return rows[0]
}

func TestWatchAuthorAvatarComesWithThePoll(t *testing.T) {
	t.Parallel()
	s, _ := openTemp(t)
	ctx := context.Background()
	now := time.Date(2026, 9, 7, 12, 0, 0, 0, time.UTC)
	avatar := "https://avatars.githubusercontent.com/in/29110?v=4"

	w, err := s.CreateWatch(ctx, newWatch(now))
	if err != nil {
		t.Fatal(err)
	}
	if w.AuthorAvatarURL != "" {
		t.Fatalf("AuthorAvatarURL = %q, want empty before the first poll", w.AuthorAvatarURL)
	}
	if err := s.UpdateWatchState(ctx, w.ID, WatchState{AuthorAvatarURL: avatar, HeadSHA: "abc", PolledAt: now}); err != nil {
		t.Fatal(err)
	}
	if got, _ := s.GetWatch(ctx, w.ID); got.AuthorAvatarURL != avatar {
		t.Fatalf("AuthorAvatarURL after poll = %q, want %q", got.AuthorAvatarURL, avatar)
	}
	if err := s.UpdateWatchState(ctx, w.ID, WatchState{HeadSHA: "abc", PolledAt: now}); err != nil {
		t.Fatal(err)
	}
	if got, _ := s.GetWatch(ctx, w.ID); got.AuthorAvatarURL != avatar {
		t.Fatalf("AuthorAvatarURL after a poll with no avatar = %q, want %q", got.AuthorAvatarURL, avatar)
	}
}
