package store

import (
	"cmp"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/mattn/go-sqlite3"

	"github.com/deividfortuna/babysitter/internal/events"
)

var (
	ErrProposalNotFound = errors.New("proposal not found")
	ErrProposalActive   = errors.New("the watch has a proposal open already")
	ErrReplyNotFound    = errors.New("the proposal has no such reply")
)

type ProposalStatus string

const (
	ProposalOpen       ProposalStatus = "open"
	ProposalPending    ProposalStatus = "pending"
	ProposalRejected   ProposalStatus = "rejected"
	ProposalReleased   ProposalStatus = "released"
	ProposalFailed     ProposalStatus = "failed"
	ProposalDeclined   ProposalStatus = "declined"
	ProposalSuperseded ProposalStatus = "superseded"
)

var ProposalStatuses = []ProposalStatus{
	ProposalOpen, ProposalPending, ProposalReleased, ProposalFailed, ProposalRejected, ProposalDeclined, ProposalSuperseded,
}

func (st ProposalStatus) Valid() bool { return slices.Contains(ProposalStatuses, st) }

func (st ProposalStatus) Waiting() bool {
	return st == ProposalOpen || st == ProposalPending || st == ProposalFailed
}

func (st ProposalStatus) Rejectable() bool {
	return st == ProposalPending || st == ProposalFailed
}

type Proposal struct {
	ID           int64
	WatchID      int64
	Number       int
	Status       ProposalStatus
	HeadSHA      string
	BaseSHA      string
	StartSHA     string
	WorkSHA      string
	HasPush      bool
	OpenedAt     time.Time
	EndedAt      *time.Time
	ReleasedAt   *time.Time
	Error        string
	ApprovedAt   *time.Time
	DecidedAt    *time.Time
	PushRejected bool
	RebasedFrom  string
	Reason       string
}

func (p Proposal) Pushes() bool { return p.HasPush && !p.PushRejected }

type ProposalReply struct {
	ID          int64
	ProposalID  int64
	InReplyTo   int64
	InReplyKind ReviewItemKind
	Body        string
	RecordedAt  time.Time
	PostedKind  ReviewItemKind
	PostedID    int64
	PostedURL   string
	PostedAt    *time.Time
	Error       string
	DroppedAt   *time.Time
	Edited      string
	Dropped     bool
}

func (r ProposalReply) Target() SeenItem { return SeenItem{Kind: r.InReplyKind, ID: r.InReplyTo} }

func (r ProposalReply) Posted() bool { return r.PostedAt != nil }

func (r ProposalReply) Text() string { return cmp.Or(r.Edited, r.Body) }

func (r ProposalReply) Waiting() bool { return !r.Posted() && r.DroppedAt == nil && !r.Dropped }

const proposalColumns = "id, watch_id, number, status, head_sha, base_sha, work_sha, has_push, opened_at, ended_at, released_at, error, " +
	"approved_at, decided_at, push_rejected, rebased_from, reason, start_sha"

const replyColumns = "id, proposal_id, in_reply_to, in_reply_kind, body, recorded_at, posted_kind, posted_id, posted_url, posted_at, error, dropped_at, edited_body, dropped"

func (s *Store) OpenProposal(ctx context.Context, watchID int64, headSHA, baseSHA, startSHA string, now time.Time) (Proposal, error) {
	var id int64
	err := s.db.QueryRowContext(ctx, `
INSERT INTO proposals (watch_id, number, status, head_sha, base_sha, start_sha, opened_at)
VALUES (?, (SELECT COALESCE(MAX(number), 0) + 1 FROM proposals WHERE watch_id = ?), ?, ?, ?, ?, ?)
RETURNING id`, watchID, watchID, ProposalOpen, headSHA, baseSHA, startSHA, timeToDB(now)).Scan(&id)
	if err != nil {
		var sqliteErr sqlite3.Error
		if errors.As(err, &sqliteErr) && sqliteErr.ExtendedCode == sqlite3.ErrConstraintUnique {
			return Proposal{}, ErrProposalActive
		}
		return Proposal{}, fmt.Errorf("open proposal: %w", err)
	}
	s.publishProposal(ctx, watchID)
	return s.proposalByID(ctx, id)
}

func (s *Store) ActiveProposal(ctx context.Context, watchID int64) (Proposal, bool, error) {
	return s.LatestProposal(ctx, watchID, ProposalOpen)
}

func (s *Store) LatestProposal(ctx context.Context, watchID int64, status ProposalStatus) (Proposal, bool, error) {
	row := s.db.QueryRowContext(ctx,
		"SELECT "+proposalColumns+" FROM proposals WHERE watch_id = ? AND status = ? ORDER BY number DESC LIMIT 1", watchID, status)
	p, err := scanProposal(row)
	if errors.Is(err, sql.ErrNoRows) {
		return Proposal{}, false, nil
	}
	if err != nil {
		return Proposal{}, false, fmt.Errorf("latest proposal: %w", err)
	}
	return p, true, nil
}

func (s *Store) GetProposal(ctx context.Context, watchID int64, number int) (Proposal, error) {
	row := s.db.QueryRowContext(ctx, "SELECT "+proposalColumns+" FROM proposals WHERE watch_id = ? AND number = ?", watchID, number)
	p, err := scanProposal(row)
	if errors.Is(err, sql.ErrNoRows) {
		return Proposal{}, fmt.Errorf("%w: %d", ErrProposalNotFound, number)
	}
	if err != nil {
		return Proposal{}, fmt.Errorf("get proposal: %w", err)
	}
	return p, nil
}

func (s *Store) ListProposals(ctx context.Context, watchID int64) ([]Proposal, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT "+proposalColumns+" FROM proposals WHERE watch_id = ? ORDER BY number DESC", watchID)
	if err != nil {
		return nil, fmt.Errorf("list proposals: %w", err)
	}
	defer rows.Close()
	out := make([]Proposal, 0)
	for rows.Next() {
		p, err := scanProposal(rows)
		if err != nil {
			return nil, fmt.Errorf("list proposals: %w", err)
		}
		out = append(out, p)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list proposals: %w", err)
	}
	return out, nil
}

func (s *Store) EndProposal(ctx context.Context, id int64, workSHA string, hasPush bool, now time.Time) error {
	return s.updateProposal(ctx, id, "end proposal",
		"UPDATE proposals SET work_sha = ?, has_push = ?, ended_at = ? WHERE id = ?", workSHA, hasPush, timeToDB(now), id)
}

func (s *Store) SetProposalOutcome(ctx context.Context, id int64, status ProposalStatus, errText string, now time.Time) error {
	var released any
	if status == ProposalReleased {
		released = timeToDB(now)
	}
	return s.updateProposal(ctx, id, "set proposal outcome",
		"UPDATE proposals SET status = ?, error = ?, released_at = COALESCE(?, released_at) WHERE id = ?", status, errText, released, id)
}

func (s *Store) DeclineWaitingProposals(ctx context.Context, watchID int64) ([]Proposal, error) {
	waiting, err := s.declineWaiting(ctx, watchID)
	if err != nil {
		return nil, fmt.Errorf("decline waiting proposals: %w", err)
	}
	if len(waiting) > 0 {
		s.publishProposal(ctx, watchID)
	}
	return waiting, nil
}

func (s *Store) declineWaiting(ctx context.Context, watchID int64) ([]Proposal, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	waitingStatuses := []any{watchID, ProposalOpen, ProposalPending, ProposalFailed}
	waiting, err := waitingProposals(ctx, tx, waitingStatuses)
	if err != nil {
		return nil, err
	}
	if _, err := tx.ExecContext(ctx,
		"UPDATE proposals SET status = ? WHERE watch_id = ? AND status IN (?, ?, ?)", append([]any{ProposalDeclined}, waitingStatuses...)...); err != nil {
		return nil, err
	}
	return waiting, tx.Commit()
}

func waitingProposals(ctx context.Context, tx *sql.Tx, waitingStatuses []any) ([]Proposal, error) {
	rows, err := tx.QueryContext(ctx,
		"SELECT "+proposalColumns+" FROM proposals WHERE watch_id = ? AND status IN (?, ?, ?) ORDER BY number", waitingStatuses...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var waiting []Proposal
	for rows.Next() {
		p, err := scanProposal(rows)
		if err != nil {
			return nil, err
		}
		waiting = append(waiting, p)
	}
	return waiting, rows.Err()
}

func (s *Store) RestoreProposals(ctx context.Context, ps []Proposal) error {
	if len(ps) == 0 {
		return nil
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("restore proposals: %w", err)
	}
	defer tx.Rollback()
	for _, p := range ps {
		if _, err := tx.ExecContext(ctx, "UPDATE proposals SET status = ? WHERE id = ? AND status = ?", p.Status, p.ID, ProposalDeclined); err != nil {
			return fmt.Errorf("restore proposal %d: %w", p.Number, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("restore proposals: %w", err)
	}
	s.publishProposal(ctx, ps[0].WatchID)
	return nil
}

func (s *Store) MoveProposal(ctx context.Context, id int64, headSHA, baseSHA, workSHA string) error {
	return s.updateProposal(ctx, id, "move proposal",
		"UPDATE proposals SET head_sha = ?, base_sha = ?, work_sha = ? WHERE id = ?", headSHA, baseSHA, workSHA, id)
}

func (s *Store) DeleteProposal(ctx context.Context, id int64) error {
	watchID, err := s.proposalWatch(ctx, id)
	if err != nil {
		return err
	}
	if _, err := s.db.ExecContext(ctx, "DELETE FROM proposals WHERE id = ?", id); err != nil {
		return fmt.Errorf("delete proposal: %w", err)
	}
	s.publishProposal(ctx, watchID)
	return nil
}

func (s *Store) SupersedeProposal(ctx context.Context, oldID, newID int64) error {
	watchID, err := s.proposalWatch(ctx, oldID)
	if err != nil {
		return err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("supersede proposal: %w", err)
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, "UPDATE proposal_replies SET proposal_id = ? WHERE proposal_id = ? AND posted_at IS NULL AND dropped_at IS NULL AND dropped = 0", newID, oldID); err != nil {
		return fmt.Errorf("supersede proposal: %w", err)
	}
	if _, err := tx.ExecContext(ctx, "UPDATE proposals SET start_sha = (SELECT start_sha FROM proposals WHERE id = ?) WHERE id = ?", oldID, newID); err != nil {
		return fmt.Errorf("supersede proposal: %w", err)
	}
	if _, err := tx.ExecContext(ctx, "UPDATE proposals SET status = ? WHERE id = ?", ProposalSuperseded, oldID); err != nil {
		return fmt.Errorf("supersede proposal: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("supersede proposal: %w", err)
	}
	s.publishProposal(ctx, watchID)
	return nil
}

func (s *Store) AddProposalReplyTo(ctx context.Context, proposalID int64, to SeenItem, body string, now time.Time) (ProposalReply, error) {
	inReplyTo := to.ID
	if inReplyTo == 0 {
		to.Kind = ""
	}
	watchID, err := s.proposalWatch(ctx, proposalID)
	if err != nil {
		return ProposalReply{}, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return ProposalReply{}, fmt.Errorf("add proposal reply: %w", err)
	}
	defer tx.Rollback()
	if inReplyTo != 0 {
		if _, err := tx.ExecContext(ctx,
			"DELETE FROM proposal_replies WHERE proposal_id = ? AND in_reply_to = ? AND in_reply_kind = ? AND posted_at IS NULL",
			proposalID, inReplyTo, to.Kind); err != nil {
			return ProposalReply{}, fmt.Errorf("add proposal reply: %w", err)
		}
	}
	var id int64
	err = tx.QueryRowContext(ctx,
		"INSERT INTO proposal_replies (proposal_id, in_reply_to, in_reply_kind, body, recorded_at) VALUES (?, ?, ?, ?, ?) RETURNING id",
		proposalID, inReplyTo, to.Kind, body, timeToDB(now)).Scan(&id)
	if err != nil {
		return ProposalReply{}, fmt.Errorf("add proposal reply: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return ProposalReply{}, fmt.Errorf("add proposal reply: %w", err)
	}
	s.publishProposal(ctx, watchID)
	return ProposalReply{ID: id, ProposalID: proposalID, InReplyTo: inReplyTo, InReplyKind: to.Kind, Body: body, RecordedAt: now}, nil
}

func (s *Store) ProposalReplies(ctx context.Context, proposalID int64) ([]ProposalReply, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT "+replyColumns+" FROM proposal_replies WHERE proposal_id = ? ORDER BY id", proposalID)
	if err != nil {
		return nil, fmt.Errorf("proposal replies: %w", err)
	}
	defer rows.Close()
	out := make([]ProposalReply, 0)
	for rows.Next() {
		r, err := scanReply(rows)
		if err != nil {
			return nil, fmt.Errorf("proposal replies: %w", err)
		}
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("proposal replies: %w", err)
	}
	return out, nil
}

func (s *Store) MarkReplyPosted(ctx context.Context, id int64, kind ReviewItemKind, postedID int64, url string, now time.Time) error {
	if _, err := s.db.ExecContext(ctx,
		"UPDATE proposal_replies SET posted_kind = ?, posted_id = ?, posted_url = ?, posted_at = ?, error = '' WHERE id = ?",
		kind, postedID, url, timeToDB(now), id); err != nil {
		return fmt.Errorf("mark reply posted: %w", err)
	}
	return nil
}

func (s *Store) SetReplyError(ctx context.Context, id int64, errText string) error {
	if _, err := s.db.ExecContext(ctx, "UPDATE proposal_replies SET error = ? WHERE id = ?", errText, id); err != nil {
		return fmt.Errorf("set reply error: %w", err)
	}
	return nil
}

func (s *Store) DropReply(ctx context.Context, id int64, errText string, now time.Time) error {
	if _, err := s.db.ExecContext(ctx, "UPDATE proposal_replies SET error = ?, dropped_at = ? WHERE id = ?", errText, timeToDB(now), id); err != nil {
		return fmt.Errorf("drop reply: %w", err)
	}
	return nil
}

func (s *Store) PendingProposals(ctx context.Context) (map[int64]int, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT watch_id, number FROM proposals WHERE status = ?", ProposalPending)
	if err != nil {
		return nil, fmt.Errorf("pending proposals: %w", err)
	}
	defer rows.Close()
	out := map[int64]int{}
	for rows.Next() {
		var watchID int64
		var number int
		if err := rows.Scan(&watchID, &number); err != nil {
			return nil, fmt.Errorf("pending proposals: %w", err)
		}
		out[watchID] = number
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("pending proposals: %w", err)
	}
	return out, nil
}

func (s *Store) EditReply(ctx context.Context, proposalID, replyID int64, body string) error {
	return s.updateReply(ctx, proposalID, replyID, "edit reply", "edited_body = ?", body)
}

func (s *Store) WithdrawReply(ctx context.Context, proposalID, replyID int64) error {
	return s.updateReply(ctx, proposalID, replyID, "drop reply", "dropped = ?", true)
}

func (s *Store) RewriteReply(ctx context.Context, proposalID, replyID int64, body, edited string) error {
	return s.updateReply(ctx, proposalID, replyID, "rewrite reply", "body = ?, edited_body = ?", body, edited)
}

func (s *Store) updateReply(ctx context.Context, proposalID, replyID int64, what, set string, values ...any) error {
	watchID, err := s.proposalWatch(ctx, proposalID)
	if err != nil {
		return err
	}
	res, err := s.db.ExecContext(ctx, "UPDATE proposal_replies SET "+set+" WHERE id = ? AND proposal_id = ?", append(values, replyID, proposalID)...)
	if err != nil {
		return fmt.Errorf("%s: %w", what, err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return fmt.Errorf("%w: %d", ErrReplyNotFound, replyID)
	}
	s.publishProposal(ctx, watchID)
	return nil
}

func (s *Store) ApproveProposal(ctx context.Context, id int64, pushRejected bool, now time.Time) error {
	return s.updateProposal(ctx, id, "approve proposal",
		"UPDATE proposals SET approved_at = ?, decided_at = ?, push_rejected = ? WHERE id = ?", timeToDB(now), timeToDB(now), pushRejected, id)
}

func (s *Store) RejectProposal(ctx context.Context, id int64, reason string, now time.Time) error {
	return s.updateProposal(ctx, id, "reject proposal",
		"UPDATE proposals SET status = ?, reason = ?, decided_at = ? WHERE id = ?", ProposalRejected, reason, timeToDB(now), id)
}

func (s *Store) MarkProposalRebased(ctx context.Context, id int64, head, work string, now time.Time) error {
	return s.updateProposal(ctx, id, "mark proposal rebased", `
UPDATE proposals SET rebased_from = work_sha, head_sha = ?, base_sha = ?, work_sha = ?, status = ?,
	approved_at = NULL, decided_at = NULL, ended_at = ?, error = ''
WHERE id = ?`, head, head, work, ProposalPending, timeToDB(now), id)
}

func (s *Store) updateProposal(ctx context.Context, id int64, what, q string, args ...any) error {
	watchID, err := s.proposalWatch(ctx, id)
	if err != nil {
		return err
	}
	if _, err := s.db.ExecContext(ctx, q, args...); err != nil {
		return fmt.Errorf("%s: %w", what, err)
	}
	s.publishProposal(ctx, watchID)
	return nil
}

func (s *Store) proposalWatch(ctx context.Context, id int64) (int64, error) {
	var watchID int64
	err := s.db.QueryRowContext(ctx, "SELECT watch_id FROM proposals WHERE id = ?", id).Scan(&watchID)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, ErrProposalNotFound
	}
	if err != nil {
		return 0, fmt.Errorf("proposal watch: %w", err)
	}
	return watchID, nil
}

func (s *Store) proposalByID(ctx context.Context, id int64) (Proposal, error) {
	p, err := scanProposal(s.db.QueryRowContext(ctx, "SELECT "+proposalColumns+" FROM proposals WHERE id = ?", id))
	if err != nil {
		return Proposal{}, fmt.Errorf("get proposal: %w", err)
	}
	return p, nil
}

func (s *Store) publishProposal(ctx context.Context, watchID int64) {
	if repo, number, ok := s.watchEvent(ctx, watchID); ok {
		s.publish(events.WatchProposal, repo, number)
	}
}

func scanProposal(row scanner) (Proposal, error) {
	var (
		p                                    Proposal
		openedAt                             string
		endedAt, released, approved, decided sql.NullString
	)
	err := row.Scan(&p.ID, &p.WatchID, &p.Number, &p.Status, &p.HeadSHA, &p.BaseSHA, &p.WorkSHA, &p.HasPush, &openedAt, &endedAt, &released, &p.Error,
		&approved, &decided, &p.PushRejected, &p.RebasedFrom, &p.Reason, &p.StartSHA)
	if err != nil {
		return Proposal{}, err
	}
	if p.OpenedAt, err = timeFromDB(openedAt); err != nil {
		return Proposal{}, err
	}
	for _, t := range []struct {
		dst **time.Time
		src sql.NullString
	}{{&p.EndedAt, endedAt}, {&p.ReleasedAt, released}, {&p.ApprovedAt, approved}, {&p.DecidedAt, decided}} {
		if *t.dst, err = timePtrFromDB(t.src); err != nil {
			return Proposal{}, err
		}
	}
	return p, nil
}

func scanReply(row scanner) (ProposalReply, error) {
	var (
		r          ProposalReply
		recordedAt string
		postedAt   sql.NullString
		droppedAt  sql.NullString
	)
	err := row.Scan(&r.ID, &r.ProposalID, &r.InReplyTo, &r.InReplyKind, &r.Body, &recordedAt, &r.PostedKind, &r.PostedID, &r.PostedURL, &postedAt, &r.Error, &droppedAt, &r.Edited, &r.Dropped)
	if err != nil {
		return ProposalReply{}, err
	}
	if r.RecordedAt, err = timeFromDB(recordedAt); err != nil {
		return ProposalReply{}, err
	}
	if r.PostedAt, err = timePtrFromDB(postedAt); err != nil {
		return ProposalReply{}, err
	}
	if r.DroppedAt, err = timePtrFromDB(droppedAt); err != nil {
		return ProposalReply{}, err
	}
	return r, nil
}
