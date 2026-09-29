package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/deividfortuna/babysitter/internal/events"
)

type ActivityKind string

const (
	ActivityComment          ActivityKind = "comment"
	ActivityReviewComment    ActivityKind = "review_comment"
	ActivityReview           ActivityKind = "review"
	ActivityCheckFailed      ActivityKind = "check_failed"
	ActivityCheckRecovered   ActivityKind = "check_recovered"
	ActivityChecksGreen      ActivityKind = "checks_green"
	ActivityCommit           ActivityKind = "commit"
	ActivityBehind           ActivityKind = "behind"
	ActivityConflict         ActivityKind = "conflict"
	ActivityMerged           ActivityKind = "merged"
	ActivityClosed           ActivityKind = "closed"
	ActivityHeartbeat        ActivityKind = "heartbeat"
	ActivityWatchStarted     ActivityKind = "watch_started"
	ActivityWatchStopped     ActivityKind = "watch_stopped"
	ActivitySessionStarted   ActivityKind = "session_started"
	ActivitySessionExited    ActivityKind = "session_exited"
	ActivityNudged           ActivityKind = "nudged"
	ActivityAgentFailed      ActivityKind = "agent_failed"
	ActivityMergeReady       ActivityKind = "merge_ready"
	ActivityMergeFailed      ActivityKind = "merge_failed"
	ActivityReplied          ActivityKind = "replied"
	ActivityReviewRequested  ActivityKind = "review_requested"
	ActivityProposal         ActivityKind = "proposal"
	ActivityTakenOver        ActivityKind = "taken_over"
	ActivityHandedBack       ActivityKind = "handed_back"
	ActivityAutoStarted      ActivityKind = "auto_started"
	ActivityApproved         ActivityKind = "approved"
	ActivityApprovalAsked    ActivityKind = "approval_asked"
	ActivityBranchUpdated    ActivityKind = "branch_updated"
	ActivityBranchNotUpdated ActivityKind = "branch_update_failed"
)

var ActivityKinds = []ActivityKind{
	ActivityComment, ActivityReviewComment, ActivityReview,
	ActivityCheckFailed, ActivityCheckRecovered, ActivityChecksGreen,
	ActivityCommit, ActivityBehind, ActivityConflict, ActivityMerged, ActivityClosed,
	ActivityHeartbeat, ActivityWatchStarted, ActivityWatchStopped,
	ActivitySessionStarted, ActivitySessionExited, ActivityNudged, ActivityAgentFailed,
	ActivityMergeReady, ActivityMergeFailed, ActivityReplied, ActivityReviewRequested, ActivityProposal,
	ActivityTakenOver, ActivityHandedBack, ActivityAutoStarted, ActivityApproved, ActivityApprovalAsked,
	ActivityBranchUpdated, ActivityBranchNotUpdated,
}

func (k ActivityKind) Valid() bool { return slices.Contains(ActivityKinds, k) }

var ActionableKinds = []ActivityKind{
	ActivityComment, ActivityReviewComment, ActivityReview, ActivityCheckFailed, ActivityBehind, ActivityConflict,
}

type Activity struct {
	ID         int64
	WatchID    int64
	Kind       ActivityKind
	Ref        string
	At         time.Time
	Actor      string
	Summary    string
	URL        string
	Payload    json.RawMessage
	Reported   bool
	ReportedAt *time.Time
	NudgedAt   *time.Time
}

const activityColumns = "id, watch_id, kind, ref, at, actor, summary, url, payload, reported, reported_at, nudged_at"

func (s *Store) InsertActivity(ctx context.Context, a Activity) (Activity, bool, error) {
	if len(a.Payload) == 0 {
		a.Payload = json.RawMessage("{}")
	}
	var id int64
	err := s.db.QueryRowContext(ctx, `
INSERT INTO watch_activity (watch_id, kind, ref, at, actor, summary, url, payload, reported, reported_at, nudged_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT (watch_id, kind, ref) DO NOTHING
RETURNING id`,
		a.WatchID, a.Kind, a.Ref, timeToDB(a.At), a.Actor, a.Summary, a.URL, string(a.Payload), a.Reported, timePtrToDB(a.ReportedAt), timePtrToDB(a.NudgedAt)).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		row := s.db.QueryRowContext(ctx,
			"SELECT "+activityColumns+" FROM watch_activity WHERE watch_id = ? AND kind = ? AND ref = ?",
			a.WatchID, a.Kind, a.Ref)
		existing, err := scanActivity(row)
		if err != nil {
			return Activity{}, false, fmt.Errorf("insert activity: %w", err)
		}
		return existing, false, nil
	}
	if err != nil {
		return Activity{}, false, fmt.Errorf("insert activity: %w", err)
	}
	a.ID = id
	return a, true, nil
}

func (s *Store) HasActivity(ctx context.Context, watchID int64, kind ActivityKind, ref string) (bool, error) {
	var has bool
	err := s.db.QueryRowContext(ctx,
		"SELECT EXISTS (SELECT 1 FROM watch_activity WHERE watch_id = ? AND kind = ? AND ref = ?)",
		watchID, kind, ref).Scan(&has)
	if err != nil {
		return false, fmt.Errorf("has activity: %w", err)
	}
	return has, nil
}

func (s *Store) ActivityByRef(ctx context.Context, watchID int64, kind ActivityKind, ref string) (Activity, bool, error) {
	row := s.db.QueryRowContext(ctx,
		"SELECT "+activityColumns+" FROM watch_activity WHERE watch_id = ? AND kind = ? AND ref = ?", watchID, kind, ref)
	a, err := scanActivity(row)
	if errors.Is(err, sql.ErrNoRows) {
		return Activity{}, false, nil
	}
	if err != nil {
		return Activity{}, false, fmt.Errorf("activity by ref: %w", err)
	}
	return a, true, nil
}

func (s *Store) UnreportedActivity(ctx context.Context, watchID int64) ([]Activity, error) {
	return s.queryActivity(ctx, "SELECT "+activityColumns+" FROM watch_activity WHERE watch_id = ? AND reported = 0 ORDER BY id", watchID)
}

func (s *Store) MarkActivityReported(ctx context.Context, ids []int64, now time.Time) error {
	return s.markActivity(ctx, "reported = 1, reported_at = ?", ids, now, "mark activity reported")
}

func (s *Store) ListActivity(ctx context.Context, watchID, sinceID int64, limit int) ([]Activity, error) {
	if limit <= 0 {
		limit = 200
	}
	return s.queryActivity(ctx,
		"SELECT "+activityColumns+" FROM watch_activity WHERE watch_id = ? AND id > ? ORDER BY id LIMIT ?",
		watchID, sinceID, limit)
}

func (s *Store) CountActivity(ctx context.Context, watchID int64) (map[ActivityKind]int, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT kind, COUNT(*) FROM watch_activity WHERE watch_id = ? GROUP BY kind", watchID)
	if err != nil {
		return nil, fmt.Errorf("count activity: %w", err)
	}
	defer rows.Close()
	out := map[ActivityKind]int{}
	for rows.Next() {
		var kind ActivityKind
		var n int
		if err := rows.Scan(&kind, &n); err != nil {
			return nil, fmt.Errorf("count activity: %w", err)
		}
		out[kind] = n
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("count activity: %w", err)
	}
	return out, nil
}

func (s *Store) UnnudgedActionable(ctx context.Context, watchID int64) ([]Activity, error) {
	q := "SELECT " + activityColumns + ` FROM watch_activity
WHERE watch_id = ? AND nudged_at IS NULL AND kind IN (` + placeholders(len(ActionableKinds)) + `)
ORDER BY id`
	args := []any{watchID}
	for _, k := range ActionableKinds {
		args = append(args, k)
	}
	return s.queryActivity(ctx, q, args...)
}

func (s *Store) UnnudgedOfKind(ctx context.Context, watchID int64, kind ActivityKind) ([]Activity, error) {
	q := "SELECT " + activityColumns + ` FROM watch_activity
WHERE watch_id = ? AND nudged_at IS NULL AND kind = ?
ORDER BY id`
	return s.queryActivity(ctx, q, watchID, kind)
}

func (s *Store) SetActivityJob(ctx context.Context, id int64, payload json.RawMessage, url string) error {
	_, err := s.db.ExecContext(ctx,
		"UPDATE watch_activity SET payload = ?, url = COALESCE(NULLIF(?, ''), url) WHERE id = ?",
		string(payload), url, id)
	if err != nil {
		return fmt.Errorf("set activity job: %w", err)
	}
	return nil
}

func (s *Store) LastNudged(ctx context.Context, watchID int64) (Activity, bool, error) {
	q := "SELECT " + activityColumns + ` FROM watch_activity
WHERE watch_id = ? AND kind = ?
ORDER BY id DESC LIMIT 1`
	rows, err := s.queryActivity(ctx, q, watchID, ActivityNudged)
	if err != nil || len(rows) == 0 {
		return Activity{}, false, err
	}
	return rows[0], true, nil
}

func (s *Store) MarkActivityNudged(ctx context.Context, ids []int64, now time.Time) error {
	return s.markActivity(ctx, "nudged_at = ?", ids, now, "mark activity nudged")
}

func (s *Store) UnmarkActivityNudged(ctx context.Context, ids []int64) error {
	if len(ids) == 0 {
		return nil
	}
	args := make([]any, 0, len(ids))
	for _, id := range ids {
		args = append(args, id)
	}
	q := "UPDATE watch_activity SET nudged_at = NULL WHERE id IN (" + placeholders(len(ids)) + ")"
	if _, err := s.db.ExecContext(ctx, q, args...); err != nil {
		return fmt.Errorf("unmark activity nudged: %w", err)
	}
	return nil
}

func (s *Store) DeleteActivity(ctx context.Context, id int64) error {
	if _, err := s.db.ExecContext(ctx, "DELETE FROM watch_activity WHERE id = ?", id); err != nil {
		return fmt.Errorf("delete activity: %w", err)
	}
	return nil
}

func (s *Store) markActivity(ctx context.Context, set string, ids []int64, now time.Time, what string) error {
	if len(ids) == 0 {
		return nil
	}
	args := make([]any, 0, len(ids)+1)
	args = append(args, timeToDB(now))
	for _, id := range ids {
		args = append(args, id)
	}
	q := "UPDATE watch_activity SET " + set + " WHERE id IN (" + placeholders(len(ids)) + ")"
	if _, err := s.db.ExecContext(ctx, q, args...); err != nil {
		return fmt.Errorf("%s: %w", what, err)
	}
	return nil
}

func (s *Store) queryActivity(ctx context.Context, q string, args ...any) ([]Activity, error) {
	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("list activity: %w", err)
	}
	defer rows.Close()
	out := make([]Activity, 0)
	for rows.Next() {
		a, err := scanActivity(rows)
		if err != nil {
			return nil, fmt.Errorf("list activity: %w", err)
		}
		out = append(out, a)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list activity: %w", err)
	}
	return out, nil
}

func scanActivity(row scanner) (Activity, error) {
	var (
		a           Activity
		at, payload string
		reported    int
		reportedAt  sql.NullString
		nudgedAt    sql.NullString
	)
	if err := row.Scan(&a.ID, &a.WatchID, &a.Kind, &a.Ref, &at, &a.Actor, &a.Summary, &a.URL, &payload, &reported, &reportedAt, &nudgedAt); err != nil {
		return Activity{}, err
	}
	var err error
	if a.At, err = timeFromDB(at); err != nil {
		return Activity{}, err
	}
	if a.ReportedAt, err = timePtrFromDB(reportedAt); err != nil {
		return Activity{}, err
	}
	if a.NudgedAt, err = timePtrFromDB(nudgedAt); err != nil {
		return Activity{}, err
	}
	a.Reported = reported != 0
	a.Payload = json.RawMessage(payload)
	return a, nil
}

func (s *Store) PublishActivity(k WatchKey) {
	s.publish(events.WatchActivity, k.Repo(), k.Number)
}

func placeholders(n int) string {
	if n <= 0 {
		return ""
	}
	return strings.TrimSuffix(strings.Repeat("?, ", n), ", ")
}
