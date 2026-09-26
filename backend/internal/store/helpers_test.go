package store

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

func migrateTo(ctx context.Context, db *sql.DB, target int) error {
	return migrateChain(ctx, db, target, false)
}

func (s *Store) AddProposalReply(ctx context.Context, proposalID, inReplyTo int64, body string, now time.Time) (ProposalReply, error) {
	return s.AddProposalReplyTo(ctx, proposalID, SeenItem{Kind: KindReviewComment, ID: inReplyTo}, body, now)
}

func (s *Store) DeleteWatch(ctx context.Context, id int64) error {
	if _, err := s.db.ExecContext(ctx, "DELETE FROM watches WHERE id = ?", id); err != nil {
		return fmt.Errorf("delete watch: %w", err)
	}
	return nil
}
