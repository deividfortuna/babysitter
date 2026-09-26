package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"slices"
	"time"
)

type ReviewItemKind string

const (
	KindIssueComment  ReviewItemKind = "issue_comment"
	KindReviewComment ReviewItemKind = "review_comment"
	KindReview        ReviewItemKind = "review"
)

var ReviewItemKinds = []ReviewItemKind{KindIssueComment, KindReviewComment, KindReview}

func (k ReviewItemKind) Valid() bool { return slices.Contains(ReviewItemKinds, k) }

func (k ReviewItemKind) Activity() ActivityKind {
	switch k {
	case KindIssueComment:
		return ActivityComment
	case KindReviewComment:
		return ActivityReviewComment
	default:
		return ActivityReview
	}
}

type WatchKey struct {
	Owner  string
	Name   string
	Number int
}

func (k WatchKey) Repo() string {
	return k.Owner + "/" + k.Name
}

type SeenItem struct {
	Kind ReviewItemKind
	ID   int64
}

func (s *Store) TouchWatch(ctx context.Context, key WatchKey, headSHA string, now time.Time) error {
	_, err := s.db.ExecContext(ctx, `
INSERT INTO pr_watch (owner, name, number, started_at, last_snapshot_at, last_head_sha)
VALUES (?, ?, ?, ?, ?, ?)
ON CONFLICT (owner, name, number) DO UPDATE SET
	last_snapshot_at = excluded.last_snapshot_at,
	last_head_sha = excluded.last_head_sha`,
		key.Owner, key.Name, key.Number, timeToDB(now), timeToDB(now), headSHA)
	if err != nil {
		return fmt.Errorf("touch watch: %w", err)
	}
	return nil
}

func (s *Store) SeenReviewItems(ctx context.Context, key WatchKey) (map[SeenItem]bool, error) {
	rows, err := s.db.QueryContext(ctx,
		"SELECT kind, item_id FROM pr_watch_seen WHERE owner = ? AND name = ? AND number = ?",
		key.Owner, key.Name, key.Number)
	if err != nil {
		return nil, fmt.Errorf("seen review items: %w", err)
	}
	defer rows.Close()

	seen := make(map[SeenItem]bool)
	for rows.Next() {
		var it SeenItem
		if err := rows.Scan(&it.Kind, &it.ID); err != nil {
			return nil, fmt.Errorf("seen review items: %w", err)
		}
		seen[it] = true
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("seen review items: %w", err)
	}
	return seen, nil
}

func (s *Store) MarkReviewItemsSeen(ctx context.Context, key WatchKey, items []SeenItem, now time.Time) error {
	if len(items) == 0 {
		return nil
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("mark review items seen: %w", err)
	}
	defer tx.Rollback()

	for _, it := range items {
		_, err := tx.ExecContext(ctx, `
INSERT INTO pr_watch_seen (owner, name, number, kind, item_id, seen_at)
VALUES (?, ?, ?, ?, ?, ?)
ON CONFLICT (owner, name, number, kind, item_id) DO NOTHING`,
			key.Owner, key.Name, key.Number, it.Kind, it.ID, timeToDB(now))
		if err != nil {
			return fmt.Errorf("mark review items seen: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("mark review items seen: %w", err)
	}
	return nil
}

func (s *Store) ForgetReviewItems(ctx context.Context, key WatchKey) error {
	_, err := s.db.ExecContext(ctx,
		"DELETE FROM pr_watch_seen WHERE owner = ? AND name = ? AND number = ?",
		key.Owner, key.Name, key.Number)
	if err != nil {
		return fmt.Errorf("forget review items: %w", err)
	}
	return nil
}

func (s *Store) ForgetReviewItem(ctx context.Context, key WatchKey, item SeenItem) error {
	_, err := s.db.ExecContext(ctx,
		"DELETE FROM pr_watch_seen WHERE owner = ? AND name = ? AND number = ? AND kind = ? AND item_id = ?",
		key.Owner, key.Name, key.Number, item.Kind, item.ID)
	if err != nil {
		return fmt.Errorf("forget review item: %w", err)
	}
	return nil
}

func (s *Store) RetryCount(ctx context.Context, key WatchKey, headSHA string) (int, error) {
	var n int
	err := s.db.QueryRowContext(ctx,
		"SELECT retries FROM pr_watch_retries WHERE owner = ? AND name = ? AND number = ? AND head_sha = ?",
		key.Owner, key.Name, key.Number, headSHA).Scan(&n)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("retry count: %w", err)
	}
	return n, nil
}

func (s *Store) IncrementRetries(ctx context.Context, key WatchKey, headSHA string) (int, error) {
	var n int
	err := s.db.QueryRowContext(ctx, `
INSERT INTO pr_watch_retries (owner, name, number, head_sha, retries)
VALUES (?, ?, ?, ?, 1)
ON CONFLICT (owner, name, number, head_sha) DO UPDATE SET retries = retries + 1
RETURNING retries`,
		key.Owner, key.Name, key.Number, headSHA).Scan(&n)
	if err != nil {
		return 0, fmt.Errorf("increment retries: %w", err)
	}
	return n, nil
}
