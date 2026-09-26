package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/deividfortuna/babysitter/internal/events"
)

func proposalWatch(t *testing.T, s *Store, number int) Watch {
	t.Helper()
	w, err := s.CreateWatch(context.Background(), Watch{Owner: "octo", Name: "hello", Number: number, StartedAt: time.Now()})
	if err != nil {
		t.Fatal(err)
	}
	return w
}

func TestOnlyAPendingOrFailedProposalIsRejectable(t *testing.T) {
	t.Parallel()
	for _, st := range ProposalStatuses {
		want := st == ProposalPending || st == ProposalFailed
		if got := st.Rejectable(); got != want {
			t.Errorf("%s.Rejectable() = %v, want %v", st, got, want)
		}
	}
}

func TestDeclineWaitingProposalsDeclinesAllOrNone(t *testing.T) {
	t.Parallel()
	s, _ := openTemp(t)
	ctx := context.Background()
	now := time.Date(2026, 9, 26, 10, 0, 0, 0, time.UTC)
	w := proposalWatch(t, s, 3)
	other := proposalWatch(t, s, 4)
	open := func(watchID int64) Proposal {
		t.Helper()
		p, err := s.OpenProposal(ctx, watchID, "h", "b", "s", now)
		if err != nil {
			t.Fatal(err)
		}
		return p
	}
	released := open(w.ID)
	if err := s.SetProposalOutcome(ctx, released.ID, ProposalReleased, "", now); err != nil {
		t.Fatal(err)
	}
	failed := open(w.ID)
	if err := s.SetProposalOutcome(ctx, failed.ID, ProposalFailed, "push rejected", now); err != nil {
		t.Fatal(err)
	}
	open(w.ID)
	open(other.ID)

	if _, err := s.db.ExecContext(ctx, `
CREATE TRIGGER third_decline_refused BEFORE UPDATE OF status ON proposals WHEN NEW.number = 3
BEGIN SELECT RAISE(ABORT, 'the decline is refused'); END`); err != nil {
		t.Fatalf("create trigger: %v", err)
	}
	if _, err := s.DeclineWaitingProposals(ctx, w.ID); err == nil {
		t.Fatal("DeclineWaitingProposals() gave no error")
	}
	if p, _ := s.GetProposal(ctx, w.ID, 2); p.Status != ProposalFailed {
		t.Fatalf("a failed decline declined proposal 2: %+v", p)
	}
	if _, err := s.db.ExecContext(ctx, "DROP TRIGGER third_decline_refused"); err != nil {
		t.Fatal(err)
	}

	waiting, err := s.DeclineWaitingProposals(ctx, w.ID)
	if err != nil || len(waiting) != 2 || waiting[0].Number != 2 || waiting[0].Status != ProposalFailed || waiting[1].Number != 3 || waiting[1].Status != ProposalOpen {
		t.Fatalf("DeclineWaitingProposals() = %+v, %v", waiting, err)
	}
	for n, want := range map[int]ProposalStatus{1: ProposalReleased, 2: ProposalDeclined, 3: ProposalDeclined} {
		if p, _ := s.GetProposal(ctx, w.ID, n); p.Status != want {
			t.Errorf("proposal %d = %s, want %s", n, p.Status, want)
		}
	}
	if p, _ := s.GetProposal(ctx, w.ID, 2); p.Error != "push rejected" {
		t.Errorf("the decline lost the error of proposal 2: %q", p.Error)
	}
	if p, _ := s.GetProposal(ctx, other.ID, 1); p.Status != ProposalOpen {
		t.Errorf("the decline touched another watch: %+v", p)
	}

	if err := s.RestoreProposals(ctx, waiting); err != nil {
		t.Fatalf("RestoreProposals() error = %v", err)
	}
	for n, want := range map[int]ProposalStatus{1: ProposalReleased, 2: ProposalFailed, 3: ProposalOpen} {
		if p, _ := s.GetProposal(ctx, w.ID, n); p.Status != want {
			t.Errorf("restored proposal %d = %s, want %s", n, p.Status, want)
		}
	}
}

func TestProposalsAreNumberedPerWatchAndOneIsActive(t *testing.T) {
	t.Parallel()
	s, _ := openTemp(t)
	ctx := context.Background()
	now := time.Date(2026, 9, 23, 10, 0, 0, 0, time.UTC)
	w := proposalWatch(t, s, 3)
	other := proposalWatch(t, s, 4)

	first, err := s.OpenProposal(ctx, w.ID, "h1", "b1", "s1", now)
	if err != nil {
		t.Fatalf("OpenProposal() error = %v", err)
	}
	if first.Number != 1 || first.Status != ProposalOpen || first.HeadSHA != "h1" || first.BaseSHA != "b1" || first.StartSHA != "s1" || !first.OpenedAt.Equal(now) {
		t.Fatalf("first = %+v", first)
	}
	if _, err := s.OpenProposal(ctx, w.ID, "h1", "b1", "s1", now); !errors.Is(err, ErrProposalActive) {
		t.Fatalf("a second active proposal error = %v, want ErrProposalActive", err)
	}
	if p, err := s.OpenProposal(ctx, other.ID, "x", "x", "x", now); err != nil || p.Number != 1 {
		t.Fatalf("the proposal of another watch = %+v, %v", p, err)
	}

	active, ok, err := s.ActiveProposal(ctx, w.ID)
	if err != nil || !ok || active.ID != first.ID {
		t.Fatalf("ActiveProposal() = %+v, %v, %v", active, ok, err)
	}
	if err := s.EndProposal(ctx, first.ID, "w1", true, now.Add(time.Minute)); err != nil {
		t.Fatalf("EndProposal() error = %v", err)
	}
	if err := s.SetProposalOutcome(ctx, first.ID, ProposalReleased, "", now.Add(2*time.Minute)); err != nil {
		t.Fatalf("SetProposalOutcome() error = %v", err)
	}
	got, err := s.GetProposal(ctx, w.ID, 1)
	if err != nil {
		t.Fatal(err)
	}
	released := now.Add(2 * time.Minute)
	if got.Status != ProposalReleased || got.WorkSHA != "w1" || !got.HasPush || got.EndedAt == nil || got.ReleasedAt == nil || !got.ReleasedAt.Equal(released) {
		t.Fatalf("released proposal = %+v", got)
	}
	if _, ok, err := s.ActiveProposal(ctx, w.ID); err != nil || ok {
		t.Fatalf("a released proposal is still active: %v, %v", ok, err)
	}

	second, err := s.OpenProposal(ctx, w.ID, "h2", "h2", "h2", now)
	if err != nil || second.Number != 2 {
		t.Fatalf("second = %+v, %v", second, err)
	}
	if err := s.DeleteProposal(ctx, second.ID); err != nil {
		t.Fatalf("DeleteProposal() error = %v", err)
	}
	third, err := s.OpenProposal(ctx, w.ID, "h3", "h3", "h3", now)
	if err != nil || third.Number != 2 {
		t.Fatalf("the number of an empty turn is not free again: %+v, %v", third, err)
	}
	if _, err := s.GetProposal(ctx, w.ID, 9); !errors.Is(err, ErrProposalNotFound) {
		t.Fatalf("GetProposal() of a missing number error = %v", err)
	}

	list, err := s.ListProposals(ctx, w.ID)
	if err != nil || len(list) != 2 || list[0].Number != 2 || list[1].Number != 1 {
		t.Fatalf("ListProposals() = %+v, %v", list, err)
	}
	if err := s.SetProposalOutcome(ctx, third.ID, ProposalFailed, "the lease refused the push", now); err != nil {
		t.Fatal(err)
	}
	failed, ok, err := s.LatestProposal(ctx, w.ID, ProposalFailed)
	if err != nil || !ok || failed.Number != 2 || failed.Error != "the lease refused the push" || failed.ReleasedAt != nil {
		t.Fatalf("LatestProposal() = %+v, %v, %v", failed, ok, err)
	}
	if err := s.MoveProposal(ctx, third.ID, "h4", "h4", "w4"); err != nil {
		t.Fatalf("MoveProposal() error = %v", err)
	}
	if moved, _ := s.GetProposal(ctx, w.ID, 2); moved.HeadSHA != "h4" || moved.BaseSHA != "h4" || moved.WorkSHA != "w4" {
		t.Fatalf("moved = %+v", moved)
	}
}

func TestProposalRepliesAreKeptInOrderAndPostedOnce(t *testing.T) {
	t.Parallel()
	s, _ := openTemp(t)
	ctx := context.Background()
	now := time.Date(2026, 9, 23, 10, 0, 0, 0, time.UTC)
	w := proposalWatch(t, s, 3)
	p, err := s.OpenProposal(ctx, w.ID, "h", "h", "h", now)
	if err != nil {
		t.Fatal(err)
	}

	thread, err := s.AddProposalReply(ctx, p.ID, 31, "fixed in 1a2b", now)
	if err != nil {
		t.Fatalf("AddProposalReply() error = %v", err)
	}
	general, err := s.AddProposalReply(ctx, p.ID, 0, "the failure is on main too", now)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.MarkReplyPosted(ctx, thread.ID, KindReviewComment, 32, "https://c/32", now); err != nil {
		t.Fatalf("MarkReplyPosted() error = %v", err)
	}
	if err := s.SetReplyError(ctx, general.ID, "Server Error"); err != nil {
		t.Fatalf("SetReplyError() error = %v", err)
	}
	replies, err := s.ProposalReplies(ctx, p.ID)
	if err != nil || len(replies) != 2 {
		t.Fatalf("ProposalReplies() = %+v, %v", replies, err)
	}
	if r := replies[0]; r.InReplyTo != 31 || r.Body != "fixed in 1a2b" || !r.Posted() || r.PostedKind != KindReviewComment || r.PostedID != 32 || r.PostedURL != "https://c/32" {
		t.Fatalf("thread reply = %+v", r)
	}
	if r := replies[1]; r.InReplyTo != 0 || r.Posted() || r.Error != "Server Error" {
		t.Fatalf("general reply = %+v", r)
	}
}

func TestANewReplyToACommentTakesThePlaceOfTheOneThatWaits(t *testing.T) {
	t.Parallel()
	s, _ := openTemp(t)
	ctx := context.Background()
	now := time.Date(2026, 9, 23, 10, 0, 0, 0, time.UTC)
	w := proposalWatch(t, s, 3)
	p, err := s.OpenProposal(ctx, w.ID, "h", "h", "h", now)
	if err != nil {
		t.Fatal(err)
	}
	posted, _ := s.AddProposalReply(ctx, p.ID, 40, "posted before", now)
	if err := s.MarkReplyPosted(ctx, posted.ID, KindReviewComment, 41, "https://c/41", now); err != nil {
		t.Fatal(err)
	}
	for _, r := range []struct {
		to   int64
		body string
	}{{31, "done in b920723"}, {0, "the failure is on main too"}, {31, "done in de3edbc"}, {0, "and on the release branch"}, {40, "answered again"}} {
		if _, err := s.AddProposalReply(ctx, p.ID, r.to, r.body, now); err != nil {
			t.Fatalf("AddProposalReply(%d, %q) error = %v", r.to, r.body, err)
		}
	}

	replies, err := s.ProposalReplies(ctx, p.ID)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, r := range replies {
		got = append(got, r.Body)
	}
	want := []string{"posted before", "the failure is on main too", "done in de3edbc", "and on the release branch", "answered again"}
	if !slices.Equal(got, want) {
		t.Fatalf("replies = %q, want %q", got, want)
	}
}

func TestARebaseRewritesTheTextOfAReplyAndOfItsEdit(t *testing.T) {
	t.Parallel()
	s, _ := openTemp(t)
	ctx := context.Background()
	now := time.Date(2026, 9, 23, 10, 0, 0, 0, time.UTC)
	w := proposalWatch(t, s, 3)
	p, err := s.OpenProposal(ctx, w.ID, "h", "h", "h", now)
	if err != nil {
		t.Fatal(err)
	}
	r, _ := s.AddProposalReply(ctx, p.ID, 31, "added in b50aed2", now)
	if err := s.EditReply(ctx, p.ID, r.ID, "Added in b50aed2, thanks."); err != nil {
		t.Fatal(err)
	}

	if err := s.RewriteReply(ctx, p.ID, r.ID, "added in 07bdda7", "Added in 07bdda7, thanks."); err != nil {
		t.Fatalf("RewriteReply() error = %v", err)
	}
	replies, _ := s.ProposalReplies(ctx, p.ID)
	if got := replies[0]; got.Body != "added in 07bdda7" || got.Edited != "Added in 07bdda7, thanks." {
		t.Fatalf("reply = %+v", got)
	}
	if err := s.RewriteReply(ctx, p.ID, r.ID+1, "x", ""); !errors.Is(err, ErrReplyNotFound) {
		t.Fatalf("RewriteReply() of no reply error = %v, want ErrReplyNotFound", err)
	}
}

func TestSupersedeCarriesTheRepliesNotPostedYet(t *testing.T) {
	t.Parallel()
	s, _ := openTemp(t)
	ctx := context.Background()
	now := time.Date(2026, 9, 23, 10, 0, 0, 0, time.UTC)
	w := proposalWatch(t, s, 3)
	old, err := s.OpenProposal(ctx, w.ID, "h", "h", "s0", now)
	if err != nil {
		t.Fatal(err)
	}
	posted, _ := s.AddProposalReply(ctx, old.ID, 31, "posted", now)
	if _, err := s.AddProposalReply(ctx, old.ID, 0, "waiting", now); err != nil {
		t.Fatal(err)
	}
	if err := s.MarkReplyPosted(ctx, posted.ID, KindReviewComment, 32, "https://c/32", now); err != nil {
		t.Fatal(err)
	}
	if err := s.SetProposalOutcome(ctx, old.ID, ProposalFailed, "push refused", now); err != nil {
		t.Fatal(err)
	}
	next, err := s.OpenProposal(ctx, w.ID, "h2", "h", "w1", now)
	if err != nil {
		t.Fatal(err)
	}

	if err := s.SupersedeProposal(ctx, old.ID, next.ID); err != nil {
		t.Fatalf("SupersedeProposal() error = %v", err)
	}
	if got, _ := s.GetProposal(ctx, w.ID, old.Number); got.Status != ProposalSuperseded {
		t.Fatalf("old = %+v", got)
	}
	if got, _ := s.GetProposal(ctx, w.ID, next.Number); got.StartSHA != "s0" {
		t.Fatalf("next = %+v, want it to start where the work that never went out started", got)
	}
	oldReplies, _ := s.ProposalReplies(ctx, old.ID)
	newReplies, _ := s.ProposalReplies(ctx, next.ID)
	if len(oldReplies) != 1 || oldReplies[0].Body != "posted" || len(newReplies) != 1 || newReplies[0].Body != "waiting" {
		t.Fatalf("old replies = %+v, new replies = %+v", oldReplies, newReplies)
	}
}

func TestADroppedReplyStaysWithItsProposal(t *testing.T) {
	t.Parallel()
	s, _ := openTemp(t)
	ctx := context.Background()
	now := time.Date(2026, 9, 23, 10, 0, 0, 0, time.UTC)
	w := proposalWatch(t, s, 3)
	old, err := s.OpenProposal(ctx, w.ID, "h", "h", "h", now)
	if err != nil {
		t.Fatal(err)
	}
	gone, _ := s.AddProposalReply(ctx, old.ID, 123, "renamed it", now)
	if err := s.DropReply(ctx, gone.ID, "no such review comment: 123", now); err != nil {
		t.Fatalf("DropReply() error = %v", err)
	}
	if err := s.SetProposalOutcome(ctx, old.ID, ProposalFailed, "push refused", now); err != nil {
		t.Fatal(err)
	}
	next, err := s.OpenProposal(ctx, w.ID, "h2", "h", "h", now)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.SupersedeProposal(ctx, old.ID, next.ID); err != nil {
		t.Fatal(err)
	}

	oldReplies, _ := s.ProposalReplies(ctx, old.ID)
	newReplies, _ := s.ProposalReplies(ctx, next.ID)
	if len(oldReplies) != 1 || len(newReplies) != 0 {
		t.Fatalf("old replies = %+v, new replies = %+v", oldReplies, newReplies)
	}
	r := oldReplies[0]
	if r.Waiting() || r.Posted() || r.DroppedAt == nil || !r.DroppedAt.Equal(now) || r.Error != "no such review comment: 123" {
		t.Fatalf("dropped reply = %+v", r)
	}
}

func TestProposalWritesPublishAnEvent(t *testing.T) {
	t.Parallel()
	s, _ := openTemp(t)
	ctx := context.Background()
	now := time.Now()
	w := proposalWatch(t, s, 3)
	pub := &recordingPublisher{}
	s.SetPublisher(pub)

	p, err := s.OpenProposal(ctx, w.ID, "h", "h", "h", now)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.AddProposalReply(ctx, p.ID, 0, "hi", now); err != nil {
		t.Fatal(err)
	}
	if err := s.SetProposalOutcome(ctx, p.ID, ProposalReleased, "", now); err != nil {
		t.Fatal(err)
	}
	if n := len(slices.DeleteFunc(slices.Clone(pub.types), func(t events.Type) bool { return t != events.WatchProposal })); n != 3 {
		t.Fatalf("events = %v", pub.types)
	}
	if e := pub.last(); e.Repo != "octo/hello" || e.Number != 3 {
		t.Fatalf("last event = %+v", e)
	}
}

func TestMigration20KeepsTheWatches(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "babysitter.db")
	db, err := sql.Open("sqlite3", "file:"+path+"?_foreign_keys=on")
	if err != nil {
		t.Fatal(err)
	}
	if err := migrateTo(ctx, db, 19); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO watches (owner, name, number, url, title, author, bot_login, head_ref, base_ref,
		source_dir, worktree_dir, work_branch, git_user_name, git_user_email, provider, model, status, stop_reason,
		include_existing, started_at, head_sha, pr_state, mergeable_state, check_states, green_sha, summary)
		VALUES ('octo', 'hello', 3, '', '', 'alice', 'alice', 'fix', 'main', '/src', '/wt', 'babysitter/fix', 'Alice', 'a@x', 'claude', '',
		'active', '', 0, '2026-09-01T00:00:00Z', 'abc', 'open', 'clean', '{}', '', '{}')`); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	s, err := Open(path)
	if err != nil {
		t.Fatalf("Open() after version 19 error = %v", err)
	}
	defer s.Close()
	ws, err := s.ListWatches(ctx, ListWatchesOptions{})
	if err != nil || len(ws) != 1 {
		t.Fatalf("watches = %v, %v", ws, err)
	}
	p, err := s.OpenProposal(ctx, ws[0].ID, "abc", "abc", "abc", time.Now())
	if err != nil || p.Number != 1 {
		t.Fatalf("OpenProposal() on the upgraded database = %+v, %v", p, err)
	}
}

func TestMigration21KeepsTheReplies(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "babysitter.db")
	db, err := sql.Open("sqlite3", "file:"+path+"?_foreign_keys=on")
	if err != nil {
		t.Fatal(err)
	}
	if err := migrateTo(ctx, db, 20); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO watches (owner, name, number, url, title, author, bot_login, head_ref, base_ref,
		source_dir, worktree_dir, work_branch, git_user_name, git_user_email, provider, model, status, stop_reason,
		include_existing, started_at, head_sha, pr_state, mergeable_state, check_states, green_sha, summary)
		VALUES ('octo', 'hello', 3, '', '', 'alice', 'alice', 'fix', 'main', '/src', '/wt', 'babysitter/fix', 'Alice', 'a@x', 'claude', '',
		'active', '', 0, '2026-09-01T00:00:00Z', 'abc', 'open', 'clean', '{}', '', '{}');
		INSERT INTO proposals (watch_id, number, status, head_sha, base_sha, opened_at) VALUES (1, 1, 'failed', 'abc', 'abc', '2026-09-01T00:00:00Z');
		INSERT INTO proposal_replies (proposal_id, in_reply_to, body, recorded_at) VALUES (1, 31, 'renamed it', '2026-09-01T00:00:00Z');`); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	s, err := Open(path)
	if err != nil {
		t.Fatalf("Open() after version 20 error = %v", err)
	}
	defer s.Close()
	replies, err := s.ProposalReplies(ctx, 1)
	if err != nil || len(replies) != 1 || !replies[0].Waiting() || replies[0].DroppedAt != nil {
		t.Fatalf("replies on the upgraded database = %+v, %v", replies, err)
	}
}

func TestMigration25KeepsTheRepliesInTheirThreads(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "babysitter.db")
	db, err := sql.Open("sqlite3", "file:"+path+"?_foreign_keys=on")
	if err != nil {
		t.Fatal(err)
	}
	if err := migrateTo(ctx, db, 24); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO watches (owner, name, number, url, title, author, bot_login, head_ref, base_ref,
		source_dir, worktree_dir, work_branch, git_user_name, git_user_email, provider, model, status, stop_reason,
		include_existing, started_at, head_sha, pr_state, mergeable_state, check_states, green_sha, summary)
		VALUES ('octo', 'hello', 3, '', '', 'alice', 'alice', 'fix', 'main', '/src', '/wt', 'babysitter/fix', 'Alice', 'a@x', 'claude', '',
		'active', '', 0, '2026-09-01T00:00:00Z', 'abc', 'open', 'clean', '{}', '', '{}');
		INSERT INTO proposals (watch_id, number, status, head_sha, base_sha, opened_at) VALUES (1, 1, 'pending', 'abc', 'abc', '2026-09-01T00:00:00Z');
		INSERT INTO proposal_replies (proposal_id, in_reply_to, body, recorded_at) VALUES (1, 31, 'renamed it', '2026-09-01T00:00:00Z');
		INSERT INTO proposal_replies (proposal_id, in_reply_to, body, recorded_at) VALUES (1, 0, 'the failure is on main', '2026-09-01T00:00:00Z');`); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	s, err := Open(path)
	if err != nil {
		t.Fatalf("Open() after version 24 error = %v", err)
	}
	defer s.Close()
	replies, err := s.ProposalReplies(ctx, 1)
	if err != nil || len(replies) != 2 || replies[0].InReplyKind != KindReviewComment || replies[1].InReplyKind != "" {
		t.Fatalf("replies on the upgraded database = %+v, %v", replies, err)
	}
}

func TestAnAnswerToAConversationCommentTakesThePlaceOfTheOneThatWaits(t *testing.T) {
	t.Parallel()
	s, _ := openTemp(t)
	ctx := context.Background()
	now := time.Date(2026, 9, 24, 10, 0, 0, 0, time.UTC)
	w := proposalWatch(t, s, 3)
	p, err := s.OpenProposal(ctx, w.ID, "h", "h", "h", now)
	if err != nil {
		t.Fatal(err)
	}
	conversation := SeenItem{Kind: KindIssueComment, ID: 422}
	for _, r := range []struct {
		to   SeenItem
		body string
	}{{conversation, "added a README section"}, {SeenItem{Kind: KindReviewComment, ID: 422}, "renamed it"}, {conversation, "put it in a JSDoc comment"}} {
		if _, err := s.AddProposalReplyTo(ctx, p.ID, r.to, r.body, now); err != nil {
			t.Fatalf("AddProposalReplyTo(%+v, %q) error = %v", r.to, r.body, err)
		}
	}

	replies, err := s.ProposalReplies(ctx, p.ID)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, r := range replies {
		got = append(got, fmt.Sprintf("%s:%d:%s", r.InReplyKind, r.InReplyTo, r.Body))
	}
	want := []string{"review_comment:422:renamed it", "issue_comment:422:put it in a JSDoc comment"}
	if !slices.Equal(got, want) {
		t.Fatalf("replies = %q, want %q", got, want)
	}
}
