package store

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
)

const (
	schemaBeforeKeepWorktree     = 28
	schemaBeforeWatchMaxInterval = 30
	schemaBeforeBranchUpdate     = 32
	schemaBeforeScreenReaderOff  = 36
)

func TestAnUpgradedWatchKeepsTheWorktreeRuleOfTheDaemon(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "babysitter.db")
	db, err := sql.Open("sqlite3", "file:"+path+"?_foreign_keys=on")
	if err != nil {
		t.Fatal(err)
	}
	if err := migrateTo(ctx, db, schemaBeforeKeepWorktree); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, "UPDATE settings SET keep_worktree = 1 WHERE id = 1"); err != nil {
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
		t.Fatal(err)
	}
	defer s.Close()
	w, err := s.GetWatch(ctx, 1)
	if err != nil || !w.KeepWorktree {
		t.Fatalf("watch of an upgraded database = %+v, %v, want the worktree kept as the daemon said", w, err)
	}
	set, err := s.Settings(ctx)
	if err != nil || set.Provider != "claude" || set.Model != "" {
		t.Fatalf("settings of an upgraded database = %+v, %v, want claude and its default model", set, err)
	}
}

func TestAnUpgradedWatchUpdatesItsBranchOnGitHubWithARebase(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "babysitter.db")
	db, err := sql.Open("sqlite3", "file:"+path+"?_foreign_keys=on")
	if err != nil {
		t.Fatal(err)
	}
	if err := migrateTo(ctx, db, schemaBeforeBranchUpdate); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO watches (owner, name, number, url, title, author, bot_login, head_ref, base_ref,
		source_dir, worktree_dir, work_branch, git_user_name, git_user_email, provider, model, status, stop_reason,
		include_existing, started_at, head_sha, pr_state, mergeable_state, check_states, green_sha, summary)
		VALUES ('octo', 'hello', 3, '', '', 'alice', 'alice', 'fix', 'main', '/src', '/wt', 'babysitter/fix', 'Alice', 'a@x', 'claude', '',
		'active', '', 0, '2026-09-01T00:00:00Z', 'abc', 'open', 'behind', '{}', '', '{}')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO watch_activity (watch_id, kind, ref, at) VALUES (1, 'behind', 'behind@abc', '2026-09-01T00:00:00Z')`); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	w, err := s.GetWatch(ctx, 1)
	if err != nil || w.BranchUpdate != BranchRebase || !w.UpdateOnGitHub {
		t.Fatalf("watch of an upgraded database = %+v, %v, want a rebase on GitHub first", w, err)
	}
	set, err := s.Settings(ctx)
	if err != nil || set.BranchUpdate != BranchRebase || !set.UpdateOnGitHub {
		t.Fatalf("settings of an upgraded database = %+v, %v, want a rebase on GitHub first", set, err)
	}
	if kept, err := s.HasActivity(ctx, 1, ActivityBehind, "behind@abc"); err != nil || !kept {
		t.Fatalf("HasActivity(behind) = %v, %v, want the activity kept", kept, err)
	}
	if _, _, err := s.InsertActivity(ctx, Activity{WatchID: 1, Kind: ActivityBranchUpdated, Ref: "branch_update@abc", At: w.StartedAt}); err != nil {
		t.Fatalf("InsertActivity(branch_updated) = %v, want the new kind accepted", err)
	}
}

func TestAProposalRebasedBeforeTheUpgradeSaysItWasRebased(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "babysitter.db")
	db, err := sql.Open("sqlite3", "file:"+path+"?_foreign_keys=on")
	if err != nil {
		t.Fatal(err)
	}
	if err := migrateTo(ctx, db, schemaBeforeBranchUpdate); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO watches (owner, name, number, url, title, author, bot_login, head_ref, base_ref,
		source_dir, worktree_dir, work_branch, git_user_name, git_user_email, provider, model, status, stop_reason,
		include_existing, started_at, head_sha, pr_state, mergeable_state, check_states, green_sha, summary)
		VALUES ('octo', 'hello', 3, '', '', 'alice', 'alice', 'fix', 'main', '/src', '/wt', 'babysitter/fix', 'Alice', 'a@x', 'claude', '',
		'active', '', 0, '2026-09-01T00:00:00Z', 'abc', 'open', 'clean', '{}', '', '{}');
		INSERT INTO proposals (watch_id, number, status, head_sha, base_sha, work_sha, opened_at, rebased_from)
		VALUES (1, 1, 'pending', 't1', 't1', 'w1-on-t1', '2026-09-01T00:00:00Z', 'w1');
		INSERT INTO proposals (watch_id, number, status, head_sha, base_sha, work_sha, opened_at)
		VALUES (1, 2, 'released', 't1', 't1', 'w2', '2026-09-01T00:00:00Z');`); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if p, err := s.GetProposal(ctx, 1, 1); err != nil || p.MovedBy != BranchRebase {
		t.Fatalf("rebased proposal of an upgraded database = %+v, %v, want it moved by a rebase", p, err)
	}
	if p, err := s.GetProposal(ctx, 1, 2); err != nil || p.MovedBy != "" {
		t.Fatalf("proposal of an upgraded database = %+v, %v, want it not moved", p, err)
	}
}

func TestTheSettingsRefuseAnUnknownProvider(t *testing.T) {
	t.Parallel()
	s, _ := openTemp(t)
	next := DefaultSettings()
	next.Provider = "self"
	if _, err := s.SaveSettings(context.Background(), next); err == nil {
		t.Fatal("SaveSettings() took provider self, want an error: the daemon cannot run your own session")
	}
}
