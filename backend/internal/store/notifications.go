package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/mattn/go-sqlite3"

	"github.com/deividfortuna/babysitter/internal/events"
)

func isForeignKeyFailure(err error) bool {
	var sqliteErr sqlite3.Error
	return errors.As(err, &sqliteErr) && sqliteErr.ExtendedCode == sqlite3.ErrConstraintForeignKey
}

type NotificationKind string

const (
	NotificationAgent  NotificationKind = "agent"
	NotificationReview NotificationKind = "review"
	NotificationChecks NotificationKind = "checks"
	NotificationWatch  NotificationKind = "watch"
	NotificationMerge  NotificationKind = "merge"
	NotificationAuto   NotificationKind = "auto"
)

var NotificationKinds = []NotificationKind{
	NotificationAgent, NotificationReview, NotificationChecks, NotificationWatch, NotificationMerge, NotificationAuto,
}

func (k NotificationKind) Valid() bool { return slices.Contains(NotificationKinds, k) }

type NotificationAction string

const (
	ActionNone         NotificationAction = ""
	ActionApproveMerge NotificationAction = "approve_merge"
)

var NotificationActions = []NotificationAction{ActionNone, ActionApproveMerge}

func (a NotificationAction) Valid() bool { return slices.Contains(NotificationActions, a) }

var ErrInvalidNotification = errors.New("invalid notification")

type Notification struct {
	ID        int64
	WatchID   int64
	Kind      NotificationKind
	Repo      string
	Number    int
	Title     string
	Body      string
	URL       string
	CreatedAt time.Time
	ReadAt    *time.Time
	Silent    bool
	Action    NotificationAction
}

func (n Notification) Validate() error {
	if !n.Kind.Valid() {
		return fmt.Errorf("%w: unknown kind %q: use %s", ErrInvalidNotification, n.Kind, JoinKinds())
	}
	if !n.Action.Valid() {
		return fmt.Errorf("%w: unknown action %q", ErrInvalidNotification, n.Action)
	}
	if strings.TrimSpace(n.Title) == "" {
		return fmt.Errorf("%w: the title is empty", ErrInvalidNotification)
	}
	if strings.TrimSpace(n.Body) == "" {
		return fmt.Errorf("%w: the body is empty", ErrInvalidNotification)
	}
	return nil
}

func KindNames() []string {
	out := make([]string, 0, len(NotificationKinds))
	for _, k := range NotificationKinds {
		out = append(out, string(k))
	}
	return out
}

func JoinKinds() string {
	return strings.Join(KindNames(), ", ")
}

type ListNotificationsOptions struct {
	UnreadOnly bool
	Limit      int
}

const notificationColumns = "id, watch_id, kind, repo, number, title, body, url, silent, created_at, read_at, action"

const DefaultNotificationCap = 5000

func (s *Store) AddNotification(ctx context.Context, n Notification) (Notification, error) {
	if err := n.Validate(); err != nil {
		return Notification{}, err
	}
	if n.CreatedAt.IsZero() {
		n.CreatedAt = time.Now().UTC()
	}
	var watchID any
	if n.WatchID != 0 {
		watchID = n.WatchID
	}
	err := s.db.QueryRowContext(ctx, `
INSERT INTO notifications (watch_id, kind, repo, number, title, body, url, silent, created_at, action)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
RETURNING id`,
		watchID, n.Kind, n.Repo, n.Number, n.Title, n.Body, n.URL, n.Silent, timeToDB(n.CreatedAt), n.Action).Scan(&n.ID)
	if isForeignKeyFailure(err) {
		return Notification{}, fmt.Errorf("add notification: %w: %d", ErrWatchNotFound, n.WatchID)
	}
	if err != nil {
		return Notification{}, fmt.Errorf("add notification: %w", err)
	}
	if err := s.trimNotifications(ctx); err != nil {
		s.log.Warn("the notification history was not trimmed", "err", err)
	}
	s.publish(events.NotificationAdded, n.Repo, n.Number)
	return n, nil
}

func (s *Store) trimNotifications(ctx context.Context) error {
	if s.notificationCap <= 0 {
		return nil
	}
	_, err := s.db.ExecContext(ctx, `
DELETE FROM notifications
WHERE id < (SELECT MIN(id) FROM (SELECT id FROM notifications ORDER BY id DESC LIMIT ?))`,
		s.notificationCap)
	if err != nil {
		return fmt.Errorf("trim notifications: %w", err)
	}
	return nil
}

func (s *Store) ListNotifications(ctx context.Context, o ListNotificationsOptions) ([]Notification, error) {
	if o.Limit <= 0 {
		o.Limit = 200
	}
	q := "SELECT " + notificationColumns + " FROM notifications"
	if o.UnreadOnly {
		q += " WHERE read_at IS NULL"
	}
	q += " ORDER BY id DESC LIMIT ?"
	rows, err := s.db.QueryContext(ctx, q, o.Limit)
	if err != nil {
		return nil, fmt.Errorf("list notifications: %w", err)
	}
	defer rows.Close()
	out := make([]Notification, 0)
	for rows.Next() {
		n, err := scanNotification(rows)
		if err != nil {
			return nil, fmt.Errorf("list notifications: %w", err)
		}
		out = append(out, n)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list notifications: %w", err)
	}
	return out, nil
}

func (s *Store) UnreadNotifications(ctx context.Context) (int, error) {
	var count int
	if err := s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM notifications WHERE read_at IS NULL").Scan(&count); err != nil {
		return 0, fmt.Errorf("count unread notifications: %w", err)
	}
	return count, nil
}

func (s *Store) MarkNotificationsRead(ctx context.Context, ids []int64, now time.Time) error {
	q := "UPDATE notifications SET read_at = ? WHERE read_at IS NULL"
	args := []any{timeToDB(now)}
	if len(ids) > 0 {
		q += " AND id IN (" + placeholders(len(ids)) + ")"
		for _, id := range ids {
			args = append(args, id)
		}
	}
	if _, err := s.db.ExecContext(ctx, q, args...); err != nil {
		return fmt.Errorf("mark notifications read: %w", err)
	}
	s.publish(events.NotificationsRead, "", 0)
	return nil
}

func scanNotification(row scanner) (Notification, error) {
	var (
		n         Notification
		watchID   sql.NullInt64
		createdAt string
		readAt    sql.NullString
	)
	if err := row.Scan(&n.ID, &watchID, &n.Kind, &n.Repo, &n.Number, &n.Title, &n.Body, &n.URL, &n.Silent, &createdAt, &readAt, &n.Action); err != nil {
		return Notification{}, err
	}
	var err error
	if n.CreatedAt, err = timeFromDB(createdAt); err != nil {
		return Notification{}, err
	}
	if n.ReadAt, err = timePtrFromDB(readAt); err != nil {
		return Notification{}, err
	}
	n.WatchID = watchID.Int64
	return n, nil
}
