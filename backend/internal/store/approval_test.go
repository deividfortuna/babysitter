package store

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/deividfortuna/babysitter/internal/events"
)

func TestAnUpgradedDatabaseKeepsAuto(t *testing.T) {
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
		'active', '', 0, '2026-09-01T00:00:00Z', 'abc', 'open', 'clean', '{}', '', '{}')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO watch_activity (watch_id, kind, ref, at, nudged_at) VALUES (1, 'comment', '11', '2026-09-01T00:00:00Z', '2026-09-01T00:01:00Z')`); err != nil {
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
	set, err := s.Settings(ctx)
	if err != nil || set.ApprovalMode != ApprovalAuto {
		t.Fatalf("settings of an upgraded database = %+v, %v, want auto", set, err)
	}
	w, err := s.GetWatch(ctx, 1)
	if err != nil || w.ApprovalMode != ApprovalAuto || w.AutoApproveRebase {
		t.Fatalf("watch of an upgraded database = %+v, %v", w, err)
	}
	rows, err := s.ListActivity(ctx, 1, 0, 0)
	if err != nil || len(rows) != 1 || rows[0].NudgedAt == nil {
		t.Fatalf("the rebuild lost the activity: %+v, %v", rows, err)
	}
	if _, _, err := s.InsertActivity(ctx, Activity{WatchID: 1, Kind: ActivityProposal, Ref: "1", At: time.Now()}); err != nil {
		t.Fatalf("a proposal row on the upgraded database: %v", err)
	}
}

func TestAWatchKeepsItsApprovalModeAndChangesIt(t *testing.T) {
	t.Parallel()
	s, _ := openTemp(t)
	ctx := context.Background()
	w, err := s.CreateWatch(ctx, Watch{Owner: "octo", Name: "hello", Number: 3, StartedAt: time.Now(), ApprovalMode: ApprovalManual, AutoApproveRebase: true})
	if err != nil {
		t.Fatal(err)
	}
	if w.ApprovalMode != ApprovalManual || !w.AutoApproveRebase {
		t.Fatalf("created watch = %+v", w)
	}
	pub := &recordingPublisher{}
	s.SetPublisher(pub)

	w, err = s.SetWatchApproval(ctx, w.ID, ApprovalAuto, false)
	if err != nil || w.ApprovalMode != ApprovalAuto || w.AutoApproveRebase {
		t.Fatalf("SetWatchApproval() = %+v, %v", w, err)
	}
	if e := pub.last(); e.Type != events.WatchChanged || e.Number != 3 {
		t.Fatalf("event = %+v", e)
	}
	n := len(pub.types)
	if _, err := s.SetWatchApproval(ctx, w.ID, ApprovalAuto, false); err != nil || len(pub.types) != n {
		t.Fatalf("a change to the same values published %v, %v", pub.types[n:], err)
	}
	if _, err := s.SetWatchApproval(ctx, w.ID, "sometimes", false); err == nil {
		t.Fatal("SetWatchApproval() took an unknown mode")
	}
	if _, err := s.CreateWatch(ctx, Watch{Owner: "octo", Name: "hello", Number: 4, StartedAt: time.Now()}); err != nil {
		t.Fatalf("a watch that names no mode: %v", err)
	}
}

func TestTheDecisionOnAProposal(t *testing.T) {
	t.Parallel()
	s, _ := openTemp(t)
	ctx := context.Background()
	now := time.Date(2026, 9, 23, 10, 0, 0, 0, time.UTC)
	w := proposalWatch(t, s, 3)
	p, err := s.OpenProposal(ctx, w.ID, "h", "h", "h", now)
	if err != nil {
		t.Fatal(err)
	}
	thread, _ := s.AddProposalReply(ctx, p.ID, 31, "blunt answer", now)
	general, _ := s.AddProposalReply(ctx, p.ID, 0, "a claim", now)
	if err := s.EndProposal(ctx, p.ID, "w1", true, now); err != nil {
		t.Fatal(err)
	}
	if err := s.SetProposalOutcome(ctx, p.ID, ProposalPending, "", now); err != nil {
		t.Fatal(err)
	}

	pending, err := s.PendingProposals(ctx)
	if err != nil || pending[w.ID] != 1 {
		t.Fatalf("PendingProposals() = %v, %v", pending, err)
	}
	if _, err := s.OpenProposal(ctx, w.ID, "h", "h", "h", now); !errors.Is(err, ErrProposalActive) {
		t.Fatalf("a turn opened beside a pending proposal: %v", err)
	}
	if err := s.EditReply(ctx, p.ID, thread.ID, "kind answer"); err != nil {
		t.Fatalf("EditReply() error = %v", err)
	}
	if err := s.WithdrawReply(ctx, p.ID, general.ID); err != nil {
		t.Fatalf("WithdrawReply() error = %v", err)
	}
	if err := s.EditReply(ctx, p.ID, 999, "x"); !errors.Is(err, ErrReplyNotFound) {
		t.Fatalf("EditReply() of a reply of no proposal error = %v", err)
	}
	if err := s.ApproveProposal(ctx, p.ID, true, now.Add(time.Minute)); err != nil {
		t.Fatalf("ApproveProposal() error = %v", err)
	}
	got, _ := s.GetProposal(ctx, w.ID, 1)
	if got.ApprovedAt == nil || !got.PushRejected || got.Status != ProposalPending {
		t.Fatalf("approved proposal = %+v", got)
	}
	replies, _ := s.ProposalReplies(ctx, p.ID)
	if r := replies[0]; r.Edited != "kind answer" || r.Text() != "kind answer" || r.Dropped {
		t.Fatalf("edited reply = %+v", r)
	}
	if r := replies[1]; !r.Dropped || r.Text() != "a claim" {
		t.Fatalf("dropped reply = %+v", r)
	}

	next, _ := s.OpenProposal(ctx, proposalWatch(t, s, 4).ID, "h", "h", "h", now)
	if err := s.RejectProposal(ctx, next.ID, "use a table test", now); err != nil {
		t.Fatalf("RejectProposal() error = %v", err)
	}
	rejected, _ := s.GetProposal(ctx, next.WatchID, 1)
	if rejected.Status != ProposalRejected || rejected.Reason != "use a table test" || rejected.DecidedAt == nil {
		t.Fatalf("rejected proposal = %+v", rejected)
	}
	if err := s.MarkProposalMoved(ctx, p.ID, "h2", "w2", BranchMerge, now); err != nil {
		t.Fatalf("MarkProposalMoved() error = %v", err)
	}
	moved, _ := s.GetProposal(ctx, w.ID, 1)
	if moved.HeadSHA != "h2" || moved.BaseSHA != "h2" || moved.WorkSHA != "w2" || moved.RebasedFrom != "w1" || moved.MovedBy != BranchMerge || moved.ApprovedAt != nil {
		t.Fatalf("moved proposal = %+v", moved)
	}
}

func TestActivityByRefFindsTheCommentAReplyAnswers(t *testing.T) {
	t.Parallel()
	s, _ := openTemp(t)
	ctx := context.Background()
	w := proposalWatch(t, s, 3)
	if _, _, err := s.InsertActivity(ctx, Activity{WatchID: w.ID, Kind: ActivityReviewComment, Ref: "31", Actor: "bob", At: time.Now()}); err != nil {
		t.Fatal(err)
	}
	a, ok, err := s.ActivityByRef(ctx, w.ID, ActivityReviewComment, "31")
	if err != nil || !ok || a.Actor != "bob" {
		t.Fatalf("ActivityByRef() = %+v, %v, %v", a, ok, err)
	}
	if _, ok, err := s.ActivityByRef(ctx, w.ID, ActivityReviewComment, "32"); err != nil || ok {
		t.Fatalf("ActivityByRef() of a missing row = %v, %v", ok, err)
	}
}

func TestForgetReviewItemForgetsOne(t *testing.T) {
	t.Parallel()
	s, _ := openTemp(t)
	ctx := context.Background()
	key := WatchKey{Owner: "octo", Name: "hello", Number: 3}
	now := time.Now()
	if err := s.TouchWatch(ctx, key, "abc", now); err != nil {
		t.Fatal(err)
	}
	items := []SeenItem{{Kind: KindReviewComment, ID: 31}, {Kind: KindReviewComment, ID: 32}}
	if err := s.MarkReviewItemsSeen(ctx, key, items, now); err != nil {
		t.Fatal(err)
	}
	if err := s.ForgetReviewItem(ctx, key, items[0]); err != nil {
		t.Fatalf("ForgetReviewItem() error = %v", err)
	}
	seen, err := s.SeenReviewItems(ctx, key)
	if err != nil || seen[items[0]] || !seen[items[1]] {
		t.Fatalf("seen = %v, %v", seen, err)
	}
}

func TestAWatchChangesItsMergeRules(t *testing.T) {
	t.Parallel()
	s, _ := openTemp(t)
	ctx := context.Background()
	w, err := s.CreateWatch(ctx, Watch{Owner: "octo", Name: "hello", Number: 3, StartedAt: time.Now(), ApprovalsRequired: 1, MergeMethod: "squash"})
	if err != nil {
		t.Fatal(err)
	}
	pub := &recordingPublisher{}
	s.SetPublisher(pub)

	w, err = s.SetWatchMergeRules(ctx, w.ID, MergeRules{ApprovalsRequired: 2, MergeMethod: "rebase", BranchUpdate: BranchMerge, UpdateOnGitHub: true})
	if err != nil || w.ApprovalsRequired != 2 || w.MergeMethod != "rebase" || w.BranchUpdate != BranchMerge || !w.UpdateOnGitHub {
		t.Fatalf("SetWatchMergeRules() = %+v, %v", w, err)
	}
	if e := pub.last(); e.Type != events.WatchChanged || e.Number != 3 {
		t.Fatalf("event = %+v", e)
	}
	n := len(pub.types)
	if _, err := s.SetWatchMergeRules(ctx, w.ID, MergeRules{ApprovalsRequired: 2, MergeMethod: "rebase", BranchUpdate: BranchMerge, UpdateOnGitHub: true}); err != nil || len(pub.types) != n {
		t.Fatalf("a change to the same values published %v, %v", pub.types[n:], err)
	}
	if _, err := s.SetWatchMergeRules(ctx, w.ID+100, MergeRules{ApprovalsRequired: 2, MergeMethod: "rebase", BranchUpdate: BranchMerge}); !errors.Is(err, ErrWatchNotFound) {
		t.Fatalf("SetWatchMergeRules() of no watch = %v, want ErrWatchNotFound", err)
	}
}
