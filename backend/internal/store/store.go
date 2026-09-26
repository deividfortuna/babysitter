package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	_ "github.com/mattn/go-sqlite3"

	"github.com/deividfortuna/babysitter/internal/events"
)

var (
	ErrRepoExists   = errors.New("repository is already watched")
	ErrRepoNotFound = errors.New("repository is not watched")
)

type Publisher interface {
	Publish(t events.Type, repo string, number int) events.Event
}

type Store struct {
	db               *sql.DB
	pub              Publisher
	log              *slog.Logger
	notificationCap  int
	responseCap      int
	responseBytesCap int
}

func (s *Store) SetPublisher(p Publisher) {
	s.pub = p
}

func (s *Store) SetLogger(log *slog.Logger) {
	if log == nil {
		log = slog.New(slog.DiscardHandler)
	}
	s.log = log
}

func (s *Store) publish(t events.Type, repo string, number int) {
	if s.pub != nil {
		s.pub.Publish(t, repo, number)
	}
}

func (s *Store) repoFullName(ctx context.Context, id int64) string {
	var name string
	if err := s.db.QueryRowContext(ctx, "SELECT owner || '/' || name FROM repos WHERE id = ?", id).Scan(&name); err != nil {
		return ""
	}
	return name
}

func DefaultPath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("user config dir: %w", err)
	}
	return filepath.Join(dir, "babysitter", "babysitter.db"), nil
}

func Open(path string) (*Store, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, fmt.Errorf("create database dir: %w", err)
	}
	dsn := fmt.Sprintf("file:%s?_journal_mode=WAL&_busy_timeout=5000&_foreign_keys=on", path)
	db, err := sql.Open("sqlite3", dsn)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}
	db.SetMaxOpenConns(1)
	if err := migrate(context.Background(), db); err != nil {
		db.Close()
		return nil, err
	}
	return &Store{
		db:               db,
		log:              slog.New(slog.DiscardHandler),
		notificationCap:  DefaultNotificationCap,
		responseCap:      DefaultResponseCap,
		responseBytesCap: DefaultResponseBytesCap,
	}, nil
}

func (s *Store) Close() error {
	return s.db.Close()
}

var scpRemote = regexp.MustCompile(`^(?:[^@/]+@)?([^:/]+):(.*)$`)

func isGitHubHost(host string) bool {
	host = strings.ToLower(host)
	return host == "github.com" || strings.HasSuffix(host, ".github.com")
}

func ParseFullName(s string) (owner, name string, err error) {
	s = strings.TrimSpace(s)
	host, path := "", s
	switch {
	case strings.Contains(s, "://"):
		u, perr := url.Parse(s)
		if perr != nil {
			return "", "", errors.New("invalid repository URL, want owner/name")
		}
		u.User = nil
		s, host, path = u.String(), u.Hostname(), u.Path
	case scpRemote.MatchString(s):
		m := scpRemote.FindStringSubmatch(s)
		host, path = m[1], m[2]
	case strings.HasPrefix(s, "github.com/"):
		host, path = "github.com", strings.TrimPrefix(s, "github.com/")
	}
	if host != "" && !isGitHubHost(host) {
		return "", "", fmt.Errorf("invalid repository %q, want a github.com repository", s)
	}
	path = strings.TrimSuffix(strings.Trim(path, "/"), ".git")
	owner, name, ok := strings.Cut(path, "/")
	if !ok || owner == "" || name == "" || strings.Contains(name, "/") {
		return "", "", fmt.Errorf("invalid repository %q, want owner/name", s)
	}
	return owner, name, nil
}

func timeToDB(t time.Time) string {
	return t.UTC().Format(time.RFC3339)
}

func timePtrToDB(t *time.Time) any {
	if t == nil {
		return nil
	}
	return timeToDB(*t)
}

func timeFromDB(s string) (time.Time, error) {
	if s == "" {
		return time.Time{}, nil
	}
	return time.Parse(time.RFC3339, s)
}

func timePtrFromDB(s sql.NullString) (*time.Time, error) {
	if !s.Valid || s.String == "" {
		return nil, nil
	}
	t, err := time.Parse(time.RFC3339, s.String)
	if err != nil {
		return nil, err
	}
	return &t, nil
}
