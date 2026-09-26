package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/deividfortuna/babysitter/internal/ghclient"
)

const DefaultResponseCap = 4096

const DefaultResponseBytesCap = 64 << 20

func (s *Store) CachedResponse(ctx context.Context, key string) (ghclient.CachedResponse, bool, error) {
	var (
		r      ghclient.CachedResponse
		header string
	)
	err := s.db.QueryRowContext(ctx,
		`SELECT etag, last_modified, status, header, body FROM github_responses WHERE key = ?`, key).
		Scan(&r.ETag, &r.LastModified, &r.Status, &header, &r.Body)
	if errors.Is(err, sql.ErrNoRows) {
		return ghclient.CachedResponse{}, false, nil
	}
	if err != nil {
		return ghclient.CachedResponse{}, false, fmt.Errorf("read cached response: %w", err)
	}
	if err := json.Unmarshal([]byte(header), &r.Header); err != nil {
		return ghclient.CachedResponse{}, false, fmt.Errorf("read cached response header: %w", err)
	}
	return r, true, nil
}

func (s *Store) PutCachedResponse(ctx context.Context, key string, r ghclient.CachedResponse) error {
	header, err := json.Marshal(r.Header)
	if err != nil {
		return fmt.Errorf("write cached response header: %w", err)
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("write cached response: %w", err)
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `
INSERT OR REPLACE INTO github_responses (key, etag, last_modified, status, header, body, size)
VALUES (?, ?, ?, ?, ?, ?, ?)`,
		key, r.ETag, r.LastModified, r.Status, string(header), r.Body, len(r.Body)); err != nil {
		return fmt.Errorf("write cached response: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `
DELETE FROM github_responses WHERE rowid IN (
    SELECT rowid FROM (
        SELECT rowid, ROW_NUMBER() OVER newest AS n, SUM(size) OVER newest AS bytes
        FROM github_responses
        WINDOW newest AS (ORDER BY rowid DESC)
    )
    WHERE n > 1 AND (n > ? OR bytes > ?)
)`, s.responseCap, s.responseBytesCap); err != nil {
		return fmt.Errorf("trim cached responses: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("write cached response: %w", err)
	}
	return nil
}

func (s *Store) DeleteCachedResponse(ctx context.Context, key string) error {
	if _, err := s.db.ExecContext(ctx, `DELETE FROM github_responses WHERE key = ?`, key); err != nil {
		return fmt.Errorf("delete cached response: %w", err)
	}
	return nil
}
