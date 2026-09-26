package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/mattn/go-sqlite3"

	"github.com/deividfortuna/babysitter/internal/events"
)

type Repo struct {
	ID           int64
	Owner        string
	Name         string
	AddedAt      time.Time
	LastSyncedAt *time.Time
	LastError    string
}

func (r Repo) FullName() string {
	return r.Owner + "/" + r.Name
}

const repoColumns = "id, owner, name, added_at, last_synced_at, last_error"

func (s *Store) AddRepo(ctx context.Context, owner, name string) (Repo, error) {
	now := time.Now()
	res, err := s.db.ExecContext(ctx,
		"INSERT INTO repos (owner, name, added_at) VALUES (?, ?, ?)",
		owner, name, timeToDB(now))
	if err != nil {
		var sqliteErr sqlite3.Error
		if errors.As(err, &sqliteErr) && sqliteErr.ExtendedCode == sqlite3.ErrConstraintUnique {
			return Repo{}, ErrRepoExists
		}
		return Repo{}, fmt.Errorf("add repository: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return Repo{}, fmt.Errorf("add repository: %w", err)
	}
	s.publish(events.RepoAdded, owner+"/"+name, 0)
	return Repo{ID: id, Owner: owner, Name: name, AddedAt: now.UTC().Truncate(time.Second)}, nil
}

func (s *Store) RemoveRepo(ctx context.Context, owner, name string) error {
	res, err := s.db.ExecContext(ctx, "DELETE FROM repos WHERE owner = ? AND name = ?", owner, name)
	if err != nil {
		return fmt.Errorf("remove repository: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("remove repository: %w", err)
	}
	if n == 0 {
		return ErrRepoNotFound
	}
	s.publish(events.RepoRemoved, owner+"/"+name, 0)
	return nil
}

func (s *Store) ListRepos(ctx context.Context) ([]Repo, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT "+repoColumns+" FROM repos ORDER BY owner, name")
	if err != nil {
		return nil, fmt.Errorf("list repositories: %w", err)
	}
	defer rows.Close()

	var repos []Repo
	for rows.Next() {
		r, err := scanRepo(rows)
		if err != nil {
			return nil, fmt.Errorf("list repositories: %w", err)
		}
		repos = append(repos, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list repositories: %w", err)
	}
	return repos, nil
}

func (s *Store) GetRepo(ctx context.Context, owner, name string) (Repo, error) {
	row := s.db.QueryRowContext(ctx, "SELECT "+repoColumns+" FROM repos WHERE owner = ? AND name = ?", owner, name)
	r, err := scanRepo(row)
	if errors.Is(err, sql.ErrNoRows) {
		return Repo{}, ErrRepoNotFound
	}
	if err != nil {
		return Repo{}, fmt.Errorf("get repository: %w", err)
	}
	return r, nil
}

func (s *Store) SetRepoSync(ctx context.Context, id int64, at time.Time, syncErr error) error {
	msg := ""
	if syncErr != nil {
		msg = syncErr.Error()
	}
	_, err := s.db.ExecContext(ctx,
		"UPDATE repos SET last_synced_at = ?, last_error = ? WHERE id = ?",
		timeToDB(at), msg, id)
	if err != nil {
		return fmt.Errorf("record sync: %w", err)
	}
	if s.pub != nil {
		s.publish(events.RepoSynced, s.repoFullName(ctx, id), 0)
	}
	return nil
}

type scanner interface {
	Scan(dest ...any) error
}

func scanRepo(sc scanner) (Repo, error) {
	var (
		r        Repo
		addedAt  string
		syncedAt sql.NullString
	)
	if err := sc.Scan(&r.ID, &r.Owner, &r.Name, &addedAt, &syncedAt, &r.LastError); err != nil {
		return Repo{}, err
	}
	var err error
	if r.AddedAt, err = timeFromDB(addedAt); err != nil {
		return Repo{}, err
	}
	if r.LastSyncedAt, err = timePtrFromDB(syncedAt); err != nil {
		return Repo{}, err
	}
	return r, nil
}

func (s *Store) RemoveRepoByID(ctx context.Context, id int64) error {
	var owner, name string
	err := s.db.QueryRowContext(ctx, "DELETE FROM repos WHERE id = ? RETURNING owner, name", id).Scan(&owner, &name)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrRepoNotFound
	}
	if err != nil {
		return fmt.Errorf("remove repository: %w", err)
	}
	s.publish(events.RepoRemoved, owner+"/"+name, 0)
	return nil
}
