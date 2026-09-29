package store

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
)

const schemaBeforeKeepWorktree = 28

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

func TestTheSettingsRefuseAnUnknownProvider(t *testing.T) {
	t.Parallel()
	s, _ := openTemp(t)
	next := DefaultSettings()
	next.Provider = "self"
	if _, err := s.SaveSettings(context.Background(), next); err == nil {
		t.Fatal("SaveSettings() took provider self, want an error: the daemon cannot run your own session")
	}
}
