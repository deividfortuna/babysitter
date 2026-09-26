package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/mattn/go-sqlite3"

	"github.com/deividfortuna/babysitter/internal/checks"
	"github.com/deividfortuna/babysitter/internal/events"
)

var (
	ErrWatchExists   = errors.New("pull request is already watched")
	ErrWatchNotFound = errors.New("watch not found")
)

type WatchStatus string

const (
	WatchActive  WatchStatus = "active"
	WatchStopped WatchStatus = "stopped"
)

var WatchStatuses = []WatchStatus{WatchActive, WatchStopped}

func (s WatchStatus) Valid() bool { return slices.Contains(WatchStatuses, s) }

type StopReason string

const (
	StopUser       StopReason = "user"
	StopMerged     StopReason = "merged"
	StopClosed     StopReason = "closed"
	StopLostAccess StopReason = "lost_access"
	StopError      StopReason = "error"
)

var StopReasons = []StopReason{StopUser, StopMerged, StopClosed, StopLostAccess, StopError}

func (r StopReason) Valid() bool { return slices.Contains(StopReasons, r) }

func (r StopReason) Word() string {
	switch r {
	case StopUser:
		return "by request"
	case StopMerged:
		return "merged"
	case StopClosed:
		return "closed"
	case StopLostAccess:
		return "lost access"
	default:
		return string(r)
	}
}

type Watch struct {
	ID                int64
	Owner             string
	Name              string
	Number            int
	URL               string
	Title             string
	Author            string
	BotLogin          string
	HeadRef           string
	BaseRef           string
	SourceDir         string
	WorktreeDir       string
	WorkBranch        string
	GitUserName       string
	GitUserEmail      string
	Provider          string
	Model             string
	Status            WatchStatus
	StopReason        StopReason
	IncludeExisting   bool
	IncludeOwn        bool
	StartedAt         time.Time
	StoppedAt         *time.Time
	LastPollAt        *time.Time
	LastHeartbeatAt   *time.Time
	LastError         string
	ConsecutiveErrors int
	HeadSHA           string
	PRState           PRState
	MergeableState    MergeableState
	CheckStates       map[string]checks.State
	GreenSHA          string
	Summary           json.RawMessage
	AgentSession      string
	ApprovalsRequired int
	MergeMethod       string
	ReadySince        *time.Time
	ReadyBlockers     []string
	ApprovalMode      ApprovalMode
	AutoApproveRebase bool
	TakenOverAt       *time.Time
	TakenOverPID      int
	HandbackStart     string
}

func (w Watch) Asks() bool { return w.ApprovalMode == ApprovalManual }

func (w Watch) Key() WatchKey {
	return WatchKey{Owner: w.Owner, Name: w.Name, Number: w.Number}
}

func (w Watch) Repo() string {
	return w.Owner + "/" + w.Name
}

type WatchState struct {
	Title          string
	BaseRef        string
	HeadSHA        string
	PRState        PRState
	MergeableState MergeableState
	CheckStates    map[string]checks.State
	GreenSHA       string
	PolledAt       time.Time
}

type ListWatchesOptions struct {
	Status WatchStatus
}

const watchColumns = `id, owner, name, number, url, title, author, bot_login, head_ref, base_ref,
	source_dir, worktree_dir, work_branch, git_user_name, git_user_email, provider, model, status, stop_reason,
	include_existing, started_at, stopped_at, last_poll_at, last_heartbeat_at, last_error,
	consecutive_errors, head_sha, pr_state, mergeable_state, check_states, green_sha, summary, include_own, agent_session,
	approvals_required, merge_method, ready_since, ready_blockers, approval_mode, auto_approve_rebase,
	taken_over_at, taken_over_pid, handback_start`

func (s *Store) CreateWatch(ctx context.Context, w Watch) (Watch, error) {
	if w.CheckStates == nil {
		w.CheckStates = map[string]checks.State{}
	}
	checkStates, err := json.Marshal(w.CheckStates)
	if err != nil {
		return Watch{}, fmt.Errorf("create watch: %w", err)
	}
	if len(w.Summary) == 0 {
		w.Summary = json.RawMessage("{}")
	}
	if w.Provider == "" {
		w.Provider = "claude"
	}
	if w.ApprovalMode == "" {
		w.ApprovalMode = ApprovalAuto
	}
	w.Status = WatchActive
	res, err := s.db.ExecContext(ctx, `
INSERT INTO watches (owner, name, number, url, title, author, bot_login, head_ref, base_ref,
	source_dir, worktree_dir, work_branch, git_user_name, git_user_email, provider, model, status, stop_reason,
	include_existing, started_at, head_sha, pr_state, mergeable_state, check_states, green_sha, summary, include_own, agent_session,
	approvals_required, merge_method, approval_mode, auto_approve_rebase)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		w.Owner, w.Name, w.Number, w.URL, w.Title, w.Author, w.BotLogin, w.HeadRef, w.BaseRef,
		w.SourceDir, w.WorktreeDir, w.WorkBranch, w.GitUserName, w.GitUserEmail, w.Provider, w.Model, w.Status, w.StopReason,
		w.IncludeExisting, timeToDB(w.StartedAt), w.HeadSHA, w.PRState, w.MergeableState, string(checkStates), w.GreenSHA, string(w.Summary), w.IncludeOwn, w.AgentSession,
		w.ApprovalsRequired, w.MergeMethod, w.ApprovalMode, w.AutoApproveRebase)
	if err != nil {
		var sqliteErr sqlite3.Error
		if errors.As(err, &sqliteErr) && sqliteErr.ExtendedCode == sqlite3.ErrConstraintUnique {
			return Watch{}, ErrWatchExists
		}
		return Watch{}, fmt.Errorf("create watch: %w", err)
	}
	w.ID, err = res.LastInsertId()
	if err != nil {
		return Watch{}, fmt.Errorf("create watch: %w", err)
	}
	s.publish(events.WatchStarted, w.Repo(), w.Number)
	return s.GetWatch(ctx, w.ID)
}

func (s *Store) GetWatch(ctx context.Context, id int64) (Watch, error) {
	row := s.db.QueryRowContext(ctx, "SELECT "+watchColumns+" FROM watches WHERE id = ?", id)
	w, err := scanWatch(row)
	if errors.Is(err, sql.ErrNoRows) {
		return Watch{}, ErrWatchNotFound
	}
	if err != nil {
		return Watch{}, fmt.Errorf("get watch: %w", err)
	}
	return w, nil
}

func (s *Store) FindActiveWatch(ctx context.Context, key WatchKey) (Watch, error) {
	row := s.db.QueryRowContext(ctx,
		"SELECT "+watchColumns+" FROM watches WHERE owner = ? AND name = ? AND number = ? AND status = ?",
		key.Owner, key.Name, key.Number, WatchActive)
	w, err := scanWatch(row)
	if errors.Is(err, sql.ErrNoRows) {
		return Watch{}, ErrWatchNotFound
	}
	if err != nil {
		return Watch{}, fmt.Errorf("find watch: %w", err)
	}
	return w, nil
}

func (s *Store) ListWatches(ctx context.Context, o ListWatchesOptions) ([]Watch, error) {
	q := "SELECT " + watchColumns + " FROM watches"
	var args []any
	if o.Status != "" {
		q += " WHERE status = ?"
		args = append(args, o.Status)
	}
	q += " ORDER BY id"
	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("list watches: %w", err)
	}
	defer rows.Close()
	out := make([]Watch, 0)
	for rows.Next() {
		w, err := scanWatch(rows)
		if err != nil {
			return nil, fmt.Errorf("list watches: %w", err)
		}
		out = append(out, w)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list watches: %w", err)
	}
	return out, nil
}

func (s *Store) UpdateWatchState(ctx context.Context, id int64, st WatchState) error {
	if st.CheckStates == nil {
		st.CheckStates = map[string]checks.State{}
	}
	checkStates, err := json.Marshal(st.CheckStates)
	if err != nil {
		return fmt.Errorf("update watch: %w", err)
	}
	_, err = s.db.ExecContext(ctx, `
UPDATE watches SET title = COALESCE(NULLIF(?, ''), title), base_ref = COALESCE(NULLIF(?, ''), base_ref),
	head_sha = ?, pr_state = ?, mergeable_state = ?, check_states = ?, green_sha = ?,
	last_poll_at = ?, last_error = '', consecutive_errors = 0
WHERE id = ?`,
		st.Title, st.BaseRef, st.HeadSHA, st.PRState, st.MergeableState, string(checkStates), st.GreenSHA, timeToDB(st.PolledAt), id)
	if err != nil {
		return fmt.Errorf("update watch: %w", err)
	}
	return nil
}

func (s *Store) SetWatchError(ctx context.Context, id int64, pollErr error, now time.Time) (int, error) {
	var n int
	err := s.db.QueryRowContext(ctx, `
UPDATE watches SET last_error = ?, consecutive_errors = consecutive_errors + 1, last_poll_at = ?
WHERE id = ? RETURNING consecutive_errors`, pollErr.Error(), timeToDB(now), id).Scan(&n)
	if err != nil {
		return 0, fmt.Errorf("set watch error: %w", err)
	}
	return n, nil
}

func (s *Store) SetWatchHeartbeat(ctx context.Context, id int64, now time.Time) error {
	if _, err := s.db.ExecContext(ctx, "UPDATE watches SET last_heartbeat_at = ? WHERE id = ?", timeToDB(now), id); err != nil {
		return fmt.Errorf("set watch heartbeat: %w", err)
	}
	return nil
}

func (s *Store) StopWatch(ctx context.Context, id int64, reason StopReason, summary json.RawMessage, now time.Time) (Watch, error) {
	if len(summary) == 0 {
		summary = json.RawMessage("{}")
	}
	res, err := s.db.ExecContext(ctx, `
UPDATE watches SET status = ?, stop_reason = ?, stopped_at = ?, summary = ?
WHERE id = ? AND status = ?`, WatchStopped, reason, timeToDB(now), string(summary), id, WatchActive)
	if err != nil {
		return Watch{}, fmt.Errorf("stop watch: %w", err)
	}
	w, err := s.GetWatch(ctx, id)
	if err != nil {
		return Watch{}, err
	}
	if n, _ := res.RowsAffected(); n > 0 {
		s.publish(events.WatchStopped, w.Repo(), w.Number)
	}
	return w, nil
}

func (s *Store) SetWatchSummary(ctx context.Context, id int64, summary json.RawMessage) (Watch, error) {
	if len(summary) == 0 {
		summary = json.RawMessage("{}")
	}
	if _, err := s.db.ExecContext(ctx, "UPDATE watches SET summary = ? WHERE id = ?", string(summary), id); err != nil {
		return Watch{}, fmt.Errorf("set watch summary: %w", err)
	}
	return s.GetWatch(ctx, id)
}

func scanWatch(row scanner) (Watch, error) {
	var (
		w                                                               Watch
		stoppedAt, lastPollAt, lastHeartbeatAt, readySince, takenOverAt sql.NullString
		startedAt, checkStates, summary, blockers                       string
		includeExisting, includeOwn                                     int
	)
	err := row.Scan(&w.ID, &w.Owner, &w.Name, &w.Number, &w.URL, &w.Title, &w.Author, &w.BotLogin, &w.HeadRef, &w.BaseRef,
		&w.SourceDir, &w.WorktreeDir, &w.WorkBranch, &w.GitUserName, &w.GitUserEmail, &w.Provider, &w.Model, &w.Status, &w.StopReason,
		&includeExisting, &startedAt, &stoppedAt, &lastPollAt, &lastHeartbeatAt, &w.LastError,
		&w.ConsecutiveErrors, &w.HeadSHA, &w.PRState, &w.MergeableState, &checkStates, &w.GreenSHA, &summary, &includeOwn, &w.AgentSession,
		&w.ApprovalsRequired, &w.MergeMethod, &readySince, &blockers, &w.ApprovalMode, &w.AutoApproveRebase,
		&takenOverAt, &w.TakenOverPID, &w.HandbackStart)
	if err != nil {
		return Watch{}, err
	}
	w.IncludeExisting = includeExisting != 0
	w.IncludeOwn = includeOwn != 0
	if w.StartedAt, err = timeFromDB(startedAt); err != nil {
		return Watch{}, err
	}
	if w.StoppedAt, err = timePtrFromDB(stoppedAt); err != nil {
		return Watch{}, err
	}
	if w.LastPollAt, err = timePtrFromDB(lastPollAt); err != nil {
		return Watch{}, err
	}
	if w.LastHeartbeatAt, err = timePtrFromDB(lastHeartbeatAt); err != nil {
		return Watch{}, err
	}
	if w.ReadySince, err = timePtrFromDB(readySince); err != nil {
		return Watch{}, err
	}
	if w.TakenOverAt, err = timePtrFromDB(takenOverAt); err != nil {
		return Watch{}, err
	}
	w.ReadyBlockers = []string{}
	if err := json.Unmarshal([]byte(blockers), &w.ReadyBlockers); err != nil {
		return Watch{}, err
	}
	w.CheckStates = map[string]checks.State{}
	if err := json.Unmarshal([]byte(checkStates), &w.CheckStates); err != nil {
		return Watch{}, err
	}
	w.Summary = json.RawMessage(summary)
	return w, nil
}

func (s *Store) SetWatchAgentSession(ctx context.Context, id int64, session string) error {
	if _, err := s.db.ExecContext(ctx, "UPDATE watches SET agent_session = ? WHERE id = ?", session, id); err != nil {
		return fmt.Errorf("set watch agent session: %w", err)
	}
	return nil
}

func (s *Store) PublishSession(k WatchKey) {
	s.publish(events.WatchSession, k.Repo(), k.Number)
}

func (s *Store) watchEvent(ctx context.Context, watchID int64) (repo string, number int, ok bool) {
	if s.pub == nil {
		return "", 0, false
	}
	err := s.db.QueryRowContext(ctx, "SELECT owner || '/' || name, number FROM watches WHERE id = ?", watchID).Scan(&repo, &number)
	return repo, number, err == nil
}

func (s *Store) SetWatchApproval(ctx context.Context, id int64, mode ApprovalMode, autoRebase bool) (Watch, error) {
	if !mode.Valid() {
		return Watch{}, fmt.Errorf("unknown approval mode %q: use auto or manual", mode)
	}
	return s.updateWatch(ctx, id, "set watch approval",
		"UPDATE watches SET approval_mode = ?, auto_approve_rebase = ? WHERE id = ? AND (approval_mode != ? OR auto_approve_rebase != ?)",
		mode, autoRebase, id, mode, autoRebase)
}

func (s *Store) SetWatchMergeRules(ctx context.Context, id int64, approvals int, method string) (Watch, error) {
	return s.updateWatch(ctx, id, "set watch merge rules",
		"UPDATE watches SET approvals_required = ?, merge_method = ? WHERE id = ? AND (approvals_required != ? OR merge_method != ?)",
		approvals, method, id, approvals, method)
}

func (s *Store) SetWatchTakeover(ctx context.Context, id int64, at *time.Time, pid int) (Watch, error) {
	since := timePtrToDB(at)
	return s.updateWatch(ctx, id, "set watch takeover",
		"UPDATE watches SET taken_over_at = ?, taken_over_pid = ? WHERE id = ? AND (taken_over_at IS NOT ? OR taken_over_pid != ?)",
		since, pid, id, since, pid)
}

func (s *Store) SetWatchHandbackStart(ctx context.Context, id int64, sha string) error {
	if _, err := s.db.ExecContext(ctx, "UPDATE watches SET handback_start = ? WHERE id = ?", sha, id); err != nil {
		return fmt.Errorf("set watch hand-back start: %w", err)
	}
	return nil
}

func (s *Store) updateWatch(ctx context.Context, id int64, what, query string, args ...any) (Watch, error) {
	res, err := s.db.ExecContext(ctx, query, args...)
	if err != nil {
		return Watch{}, fmt.Errorf("%s: %w", what, err)
	}
	if n, _ := res.RowsAffected(); n > 0 {
		if repo, number, ok := s.watchEvent(ctx, id); ok {
			s.publish(events.WatchChanged, repo, number)
		}
	}
	return s.GetWatch(ctx, id)
}

func (s *Store) SetWatchReadiness(ctx context.Context, id int64, readySince *time.Time, blockers []string) error {
	if blockers == nil {
		blockers = []string{}
	}
	b, err := json.Marshal(blockers)
	if err != nil {
		return fmt.Errorf("set watch readiness: %w", err)
	}
	var since any
	if readySince != nil {
		since = timeToDB(*readySince)
	}
	res, err := s.db.ExecContext(ctx,
		"UPDATE watches SET ready_since = ?, ready_blockers = ? WHERE id = ? AND (ready_since IS NOT ? OR ready_blockers IS NOT ?)",
		since, string(b), id, since, string(b))
	if err != nil {
		return fmt.Errorf("set watch readiness: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return nil
	}
	if repo, number, ok := s.watchEvent(ctx, id); ok {
		s.publish(events.WatchReady, repo, number)
	}
	return nil
}
