package store

import (
	"context"
	"testing"
	"time"
)

func TestSchemaAcceptsEveryVocabularyValue(t *testing.T) {
	t.Parallel()
	s, _ := openTemp(t)
	ctx := context.Background()
	now := time.Now()

	w, err := s.CreateWatch(ctx, Watch{Owner: "octo", Name: "hello", Number: 100, StartedAt: now})
	if err != nil {
		t.Fatal(err)
	}
	for _, status := range WatchStatuses {
		if _, err := s.db.ExecContext(ctx, "UPDATE watches SET status = ? WHERE id = ?", status, w.ID); err != nil {
			t.Errorf("watch status %q rejected: %v", status, err)
		}
	}
	if _, err := s.db.ExecContext(ctx, "UPDATE watches SET status = ? WHERE id = ?", WatchActive, w.ID); err != nil {
		t.Fatal(err)
	}
	for _, mode := range ApprovalModes {
		if _, err := s.db.ExecContext(ctx, "UPDATE watches SET approval_mode = ? WHERE id = ?", mode, w.ID); err != nil {
			t.Errorf("watch approval mode %q rejected: %v", mode, err)
		}
		if _, err := s.db.ExecContext(ctx, "UPDATE settings SET approval_mode = ? WHERE id = 1", mode); err != nil {
			t.Errorf("settings approval mode %q rejected: %v", mode, err)
		}
	}
	for _, reason := range StopReasons {
		if _, err := s.db.ExecContext(ctx, "UPDATE watches SET stop_reason = ? WHERE id = ?", reason, w.ID); err != nil {
			t.Errorf("stop reason %q rejected: %v", reason, err)
		}
	}
	for _, kind := range ActivityKinds {
		a := Activity{WatchID: w.ID, Kind: kind, Ref: string(kind), At: now}
		if _, _, err := s.InsertActivity(ctx, a); err != nil {
			t.Errorf("activity kind %q rejected: %v", kind, err)
		}
	}
	for _, kind := range NotificationKinds {
		n := Notification{WatchID: w.ID, Kind: kind, Title: string(kind), Body: "body", CreatedAt: now}
		if _, err := s.AddNotification(ctx, n); err != nil {
			t.Errorf("notification kind %q rejected: %v", kind, err)
		}
	}
	for _, state := range PRStates {
		pr := PullRequest{RepoID: repoID(t, s), Number: 1, State: state, SyncedAt: now, CreatedAt: now, UpdatedAt: now}
		if err := s.UpsertPR(ctx, pr); err != nil {
			t.Errorf("pull request state %q rejected: %v", state, err)
		}
	}
	for i, status := range ProposalStatuses {
		p, err := s.OpenProposal(ctx, w.ID, "h", "h", "h", now)
		if err != nil {
			t.Fatalf("open proposal %d: %v", i, err)
		}
		if err := s.SetProposalOutcome(ctx, p.ID, status, "", now); err != nil {
			t.Errorf("proposal status %q rejected: %v", status, err)
		}
		if err := s.SetProposalOutcome(ctx, p.ID, ProposalDeclined, "", now); err != nil {
			t.Fatal(err)
		}
	}
	for _, kind := range ReviewItemKinds {
		p, err := s.OpenProposal(ctx, w.ID, "h", "h", "h", now)
		if err != nil {
			t.Fatal(err)
		}
		r, err := s.AddProposalReply(ctx, p.ID, 1, "body", now)
		if err != nil {
			t.Fatal(err)
		}
		if err := s.MarkReplyPosted(ctx, r.ID, kind, 2, "", now); err != nil {
			t.Errorf("posted reply kind %q rejected: %v", kind, err)
		}
		if err := s.SetProposalOutcome(ctx, p.ID, ProposalDeclined, "", now); err != nil {
			t.Fatal(err)
		}
	}
	for _, kind := range ReviewItemKinds {
		key := WatchKey{Owner: "octo", Name: "hello", Number: 100}
		if err := s.TouchWatch(ctx, key, "abc", now); err != nil {
			t.Fatal(err)
		}
		if err := s.MarkReviewItemsSeen(ctx, key, []SeenItem{{Kind: kind, ID: 1}}, now); err != nil {
			t.Errorf("review item kind %q rejected: %v", kind, err)
		}
	}
}

func repoID(t *testing.T, s *Store) int64 {
	t.Helper()
	r, err := s.GetRepo(context.Background(), "octo", "hello")
	if err == nil {
		return r.ID
	}
	r, err = s.AddRepo(context.Background(), "octo", "hello")
	if err != nil {
		t.Fatal(err)
	}
	return r.ID
}
