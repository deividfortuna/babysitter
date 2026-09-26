package store

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
	"time"
)

func TestWatchKeysIgnoreCase(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "babysitter.db")
	db, err := sql.Open("sqlite3", "file:"+path+"?_foreign_keys=on")
	if err != nil {
		t.Fatal(err)
	}
	if err := migrateTo(ctx, db, 2); err != nil {
		t.Fatal(err)
	}
	for _, q := range []string{
		"INSERT INTO pr_watch VALUES ('Octo', 'Hello', 3, '2026-09-01T00:00:00Z', '2026-09-02T00:00:00Z', 'abc')",
		"INSERT INTO pr_watch VALUES ('octo', 'hello', 3, '2026-09-02T00:00:00Z', '2026-09-03T00:00:00Z', 'def')",
		"INSERT INTO pr_watch_seen VALUES ('Octo', 'Hello', 3, 'review', 1, '2026-09-02T00:00:00Z')",
		"INSERT INTO pr_watch_seen VALUES ('octo', 'hello', 3, 'review', 1, '2026-09-03T00:00:00Z')",
		"INSERT INTO pr_watch_seen VALUES ('octo', 'hello', 3, 'issue_comment', 2, '2026-09-03T00:00:00Z')",
		"INSERT INTO pr_watch_retries VALUES ('Octo', 'Hello', 3, 'abc', 2)",
		"INSERT INTO pr_watch_retries VALUES ('octo', 'hello', 3, 'abc', 1)",
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
		t.Fatalf("Open() after version 2 error = %v", err)
	}
	defer s.Close()
	for _, key := range []WatchKey{{Owner: "octo", Name: "hello", Number: 3}, {Owner: "OCTO", Name: "HELLO", Number: 3}} {
		seen, err := s.SeenReviewItems(ctx, key)
		if err != nil || len(seen) != 2 || !seen[SeenItem{Kind: KindReview, ID: 1}] || !seen[SeenItem{Kind: KindIssueComment, ID: 2}] {
			t.Errorf("%v: seen = %v, %v", key, seen, err)
		}
		if n, err := s.RetryCount(ctx, key, "abc"); err != nil || n != 2 {
			t.Errorf("%v: retries = %d, %v", key, n, err)
		}
	}
	var rows int
	if err := s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM pr_watch").Scan(&rows); err != nil || rows != 1 {
		t.Errorf("pr_watch rows = %d, %v", rows, err)
	}
	now := time.Date(2026, 9, 7, 12, 0, 0, 0, time.UTC)
	if err := s.TouchWatch(ctx, WatchKey{Owner: "OcTo", Name: "HeLLo", Number: 3}, "ghi", now); err != nil {
		t.Fatal(err)
	}
	if n, err := s.IncrementRetries(ctx, WatchKey{Owner: "OCTO", Name: "hello", Number: 3}, "abc"); err != nil || n != 3 {
		t.Errorf("increment = %d, %v", n, err)
	}
	if err := s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM pr_watch").Scan(&rows); err != nil || rows != 1 {
		t.Errorf("pr_watch rows after touch = %d, %v", rows, err)
	}
}

func TestWatchState(t *testing.T) {
	t.Parallel()
	s, _ := openTemp(t)
	ctx := context.Background()
	key := WatchKey{Owner: "octo", Name: "hello", Number: 3}
	now := time.Date(2026, 9, 7, 12, 0, 0, 0, time.UTC)

	if err := s.MarkReviewItemsSeen(ctx, key, []SeenItem{{Kind: KindReview, ID: 1}}, now); err == nil {
		t.Fatal("MarkReviewItemsSeen before TouchWatch expected a foreign key error")
	}
	if err := s.TouchWatch(ctx, key, "abc", now); err != nil {
		t.Fatal(err)
	}
	seen, err := s.SeenReviewItems(ctx, key)
	if err != nil || len(seen) != 0 {
		t.Fatalf("seen = %v, %v", seen, err)
	}
	items := []SeenItem{{Kind: KindReview, ID: 1}, {Kind: KindIssueComment, ID: 1}, {Kind: KindReviewComment, ID: 7}}
	if err := s.MarkReviewItemsSeen(ctx, key, items, now); err != nil {
		t.Fatal(err)
	}
	if err := s.MarkReviewItemsSeen(ctx, key, items[:1], now); err != nil {
		t.Fatal(err)
	}
	seen, err = s.SeenReviewItems(ctx, key)
	if err != nil || len(seen) != 3 || !seen[SeenItem{Kind: KindReviewComment, ID: 7}] {
		t.Fatalf("seen = %v, %v", seen, err)
	}
	if err := s.MarkReviewItemsSeen(ctx, key, []SeenItem{{Kind: "bogus", ID: 1}}, now); err == nil {
		t.Fatal("bogus kind expected a check constraint error")
	}

	if n, err := s.RetryCount(ctx, key, "abc"); err != nil || n != 0 {
		t.Fatalf("RetryCount = %d, %v", n, err)
	}
	for want := 1; want <= 2; want++ {
		if n, err := s.IncrementRetries(ctx, key, "abc"); err != nil || n != want {
			t.Fatalf("IncrementRetries = %d, %v, want %d", n, err, want)
		}
	}
	if n, _ := s.RetryCount(ctx, key, "abc"); n != 2 {
		t.Fatalf("RetryCount = %d, want 2", n)
	}
	if n, _ := s.RetryCount(ctx, key, "def"); n != 0 {
		t.Fatalf("RetryCount other sha = %d, want 0", n)
	}

	later := now.Add(time.Hour)
	if err := s.TouchWatch(ctx, key, "def", later); err != nil {
		t.Fatal(err)
	}
	var started, last, sha string
	if err := s.db.QueryRow("SELECT started_at, last_snapshot_at, last_head_sha FROM pr_watch").Scan(&started, &last, &sha); err != nil {
		t.Fatal(err)
	}
	if started != timeToDB(now) || last != timeToDB(later) || sha != "def" {
		t.Fatalf("pr_watch = %s %s %s", started, last, sha)
	}

	other := WatchKey{Owner: "octo", Name: "hello", Number: 4}
	if seen, _ := s.SeenReviewItems(ctx, other); len(seen) != 0 {
		t.Fatalf("other seen = %v", seen)
	}
}

func TestWatchModelDefaultsToEmpty(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "babysitter.db")
	db, err := sql.Open("sqlite3", "file:"+path+"?_foreign_keys=on")
	if err != nil {
		t.Fatal(err)
	}
	if err := migrateTo(ctx, db, 6); err != nil {
		t.Fatal(err)
	}
	_, err = db.ExecContext(ctx, `
INSERT INTO watches (owner, name, number, head_ref, status, started_at, provider)
VALUES ('octo', 'hello', 3, 'fix', 'active', '2026-09-07T12:00:00Z', 'copilot')`)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	s, err := Open(path)
	if err != nil {
		t.Fatalf("Open() after version 6 error = %v", err)
	}
	defer s.Close()
	w, err := s.GetWatch(ctx, 1)
	if err != nil {
		t.Fatal(err)
	}
	if w.Provider != "copilot" || w.Model != "" {
		t.Fatalf("provider = %q, model = %q", w.Provider, w.Model)
	}
}

func TestCreateWatchStoresTheModel(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	s, err := Open(filepath.Join(t.TempDir(), "babysitter.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	w, err := s.CreateWatch(ctx, Watch{
		Owner: "octo", Name: "hello", Number: 3, HeadRef: "fix",
		Provider: "claude", Model: "sonnet", StartedAt: time.Date(2026, 9, 7, 12, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatal(err)
	}
	if w.Model != "sonnet" {
		t.Fatalf("model = %q", w.Model)
	}
	list, err := s.ListWatches(ctx, ListWatchesOptions{Status: WatchActive})
	if err != nil || len(list) != 1 || list[0].Model != "sonnet" {
		t.Fatalf("list = %+v, %v", list, err)
	}
}
