package prwatch

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/google/go-github/v91/github"
	"golang.org/x/sync/errgroup"

	"github.com/deividfortuna/babysitter/internal/agent"
	"github.com/deividfortuna/babysitter/internal/checks"
	"github.com/deividfortuna/babysitter/internal/ghclient"
	"github.com/deividfortuna/babysitter/internal/redact"
	"github.com/deividfortuna/babysitter/internal/snapshot"
	"github.com/deividfortuna/babysitter/internal/store"
	"github.com/deividfortuna/babysitter/internal/textx"
)

func (s *Service) snapshotOptions(dir string) snapshot.Options {
	return snapshot.Options{Dir: dir, Now: s.now, AfterCall: s.guard.After, MaxFlakyRetries: 3}
}

func (s *Service) watchOptions(w store.Watch) snapshot.Options {
	o := s.snapshotOptions(w.SourceDir)
	o.IgnoreAuthor = ignoredAuthor(w)
	o.TokenLogin = w.BotLogin
	return o
}

type agentStatus struct {
	session  SessionInfo
	untold   string
	proposal string
	author   string
}

func (s *Service) agentStatus(ctx context.Context, w store.Watch, pending []store.Activity) (agentStatus, error) {
	session, err := s.Session(ctx, w)
	if err != nil {
		return agentStatus{}, err
	}
	proposal, err := s.proposalBlocker(ctx, w)
	if err != nil {
		return agentStatus{}, err
	}
	return agentStatus{session: session, untold: untold(pending), proposal: proposal, author: s.authorBlocker(w)}, nil
}

func (a agentStatus) done() bool {
	_, busy := agentBusyWord(a.session.State)
	return !busy && a.untold == "" && a.proposal == "" && a.author == ""
}

func (s *Service) poll(ctx context.Context, client *github.Client, w store.Watch) error {
	id, started, paced := w.ID, s.now(), keepPace
	s.schedule.calm(id)
	defer func() { s.schedule.polled(id, started, paced, s.cadence()) }()
	w, err := s.store.GetWatch(ctx, w.ID)
	if err != nil || w.Status != store.WatchActive {
		return err
	}
	now := s.now()
	snap, err := snapshot.Collect(ctx, client, s.store, target(w), s.watchOptions(w))
	if err != nil {
		return s.pollFailed(ctx, w, err, now)
	}
	p, err := s.refresh(ctx, w, snap, now)
	if err != nil {
		return err
	}
	w = p.watch
	if w, err = s.followUpdateType(ctx, w, snap); err != nil {
		return err
	}
	if len(p.inserted) > 0 {
		if err := s.store.SetWatchHeartbeat(ctx, w.ID, now); err != nil {
			return err
		}
	}
	if reason, ok := p.ended(); ok {
		_, err := s.stop(ctx, w.ID, reason, "", StopOptions{})
		return err
	}
	if err := s.githubStep(ctx, client, w, snap.PR.NodeID); err != nil {
		return err
	}
	if err := s.rebaseStale(ctx, w); err != nil {
		s.log.Error("rebase the proposal that waits", "watch", w.ID, "err", err)
	}
	if s.hostsSession(w) {
		s.tellHandback(ctx, w)
	}
	pending, err := s.tell(ctx, client, w)
	if err != nil {
		return err
	}
	state, err := s.agentStatus(ctx, w, pending)
	if err != nil {
		return err
	}
	if err := s.rereview(ctx, client, w, snap, state); err != nil {
		return err
	}
	if err := s.dependabotPolicy(ctx, client, w, snap, state); err != nil {
		s.log.Error("apply the Dependabot policy", "watch", w.ID, "pr", prLabel(w), "err", err)
	}
	if err := s.assess(ctx, w, snap, state, p.newHead()); err != nil {
		return err
	}
	if merged, err := s.mergeWhenReady(ctx, client, w, snap, p.next, state); err != nil || merged {
		return err
	}
	if len(p.inserted) == 0 {
		if err := s.maybeHeartbeat(ctx, w, p.next, now); err != nil {
			return err
		}
	}
	paced = p.pace(snap, state)
	return nil
}

type pass struct {
	watch    store.Watch
	inserted []store.Activity
	next     State
	merged   bool
	closed   bool
}

func (p pass) ended() (store.StopReason, bool) {
	switch {
	case p.merged:
		return store.StopMerged, true
	case p.closed:
		return store.StopClosed, true
	}
	return "", false
}

func (p pass) newHead() bool { return hasKind(p.inserted, store.ActivityCommit) }

func (p pass) pace(snap *snapshot.Snapshot, state agentStatus) pace {
	quiet := len(p.inserted) == 0 && snap.Checks.AllTerminal && !state.session.State.Working()
	if quiet {
		return slowDown
	}
	return speedUp
}

func (s *Service) refresh(ctx context.Context, w store.Watch, snap *snapshot.Snapshot, now time.Time) (pass, error) {
	inserted, next, err := s.recordDiff(ctx, w, snap, now)
	if err != nil {
		return pass{}, err
	}
	if err := s.keepJobs(ctx, w, snap); err != nil {
		return pass{}, err
	}
	w.HeadSHA, w.CheckStates, w.MergeableState = next.HeadSHA, next.Checks, next.MergeableState
	w.Title, w.BaseRef = snap.PR.Title, snap.PR.BaseBranch
	if len(inserted) > 0 {
		s.report(ctx, w, inserted)
	}
	return pass{watch: w, inserted: inserted, next: next, merged: snap.PR.Merged, closed: snap.PR.Closed}, nil
}

func (s *Service) pollFailed(ctx context.Context, w store.Watch, pollErr error, now time.Time) error {
	if ctx.Err() != nil || errors.Is(pollErr, ghclient.ErrPaused) {
		return pollErr
	}
	clean := redact.Err(pollErr)
	n, serr := s.store.SetWatchError(ctx, w.ID, clean, now)
	if serr != nil {
		return errors.Join(clean, serr)
	}
	if lostAccess(pollErr) && n >= lostAccessAfter {
		_, stopErr := s.stop(ctx, w.ID, store.StopLostAccess, clean.Error(), StopOptions{KeepWorktree: new(true)})
		return errors.Join(clean, stopErr)
	}
	return clean
}

func (s *Service) recordDiff(ctx context.Context, w store.Watch, snap *snapshot.Snapshot, now time.Time) ([]store.Activity, State, error) {
	items, next := Diff(stateOf(w), snap, now)
	var inserted []store.Activity
	var surfacedAgain []int64
	for _, a := range items {
		a.WatchID = w.ID
		row, isNew, err := s.store.InsertActivity(ctx, clean(a))
		if err != nil {
			return nil, State{}, err
		}
		switch {
		case isNew:
			inserted = append(inserted, row)
		case isReviewItem(row.Kind):
			surfacedAgain = append(surfacedAgain, row.ID)
		}
	}
	if err := s.store.UnmarkActivityNudged(ctx, surfacedAgain); err != nil {
		return nil, State{}, err
	}
	if err := snap.Commit(ctx, s.store); err != nil {
		return nil, State{}, err
	}
	if err := s.store.UpdateWatchState(ctx, w.ID, store.WatchState{
		Title: snap.PR.Title, BaseRef: snap.PR.BaseBranch,
		HeadSHA: next.HeadSHA, PRState: next.PRState, MergeableState: next.MergeableState,
		CheckStates: next.Checks, GreenSHA: next.GreenSHA, PolledAt: now,
	}); err != nil {
		return nil, State{}, err
	}
	return inserted, next, nil
}

func isReviewItem(kind store.ActivityKind) bool {
	return kind == store.ActivityComment || kind == store.ActivityReviewComment || kind == store.ActivityReview
}

func hasKind(items []store.Activity, kind store.ActivityKind) bool {
	for _, a := range items {
		if a.Kind == kind {
			return true
		}
	}
	return false
}

func (s *Service) tell(ctx context.Context, client *github.Client, w store.Watch) ([]store.Activity, error) {
	if !s.attended(w) {
		return nil, nil
	}
	todo, err := s.actionable(ctx, w)
	if err != nil || len(todo) == 0 {
		return nil, err
	}
	if !s.runs(w) {
		return todo, nil
	}
	if s.holdsForProposal(ctx, w) {
		s.log.Info("a proposal waits on you, the message waits for the decision", "watch", w.ID)
		return todo, nil
	}
	if s.withAuthor(w) {
		s.log.Info("the session is with you, the message waits for the hand-back", "watch", w.ID)
		return todo, nil
	}
	if s.waitsOnAuthor(w) {
		s.log.Info("the agent waits on you, the message waits for the next poll", "watch", w.ID)
		return todo, nil
	}
	if s.waitsForGitHub(ctx, w) {
		s.log.Info("GitHub updates the branch, the message waits for the new head", "watch", w.ID)
		return todo, nil
	}
	m, err := s.compose(ctx, client, w, todo)
	if err != nil {
		return todo, err
	}
	s.syncWork(ctx, w)
	_, err = s.deliver(ctx, w, m.text, m.summary, deliverRoutine, m.rows)
	switch {
	case errors.Is(err, ErrAgentBusy):
		s.log.Info("the agent waits on you, the message waits for the next poll", "watch", w.ID)
		return todo, nil
	case err != nil:
		s.agentFailed(ctx, w, "send a message to the agent", err)
		return todo, nil
	}
	return nil, nil
}

func (s *Service) untoldOf(ctx context.Context, w store.Watch) ([]store.Activity, error) {
	if !s.attended(w) {
		return nil, nil
	}
	return s.actionable(ctx, w)
}

func (s *Service) actionable(ctx context.Context, w store.Watch) ([]store.Activity, error) {
	todo, err := s.store.UnnudgedActionable(ctx, w.ID)
	if err != nil {
		return nil, err
	}
	current, stale := currentItems(todo, w.HeadSHA)
	if err := s.store.MarkActivityNudged(ctx, ids(stale), s.now()); err != nil {
		return nil, err
	}
	return s.holdForGitHub(ctx, w, current)
}

type message struct {
	text    string
	summary string
	rows    []store.Activity
}

func (m message) empty() bool { return len(m.rows) == 0 }

func (s *Service) compose(ctx context.Context, client *github.Client, w store.Watch, current []store.Activity) (message, error) {
	if len(current) == 0 {
		return message{}, nil
	}
	items := toItems(current)
	if err := s.attachLogs(ctx, client, w, items); err != nil {
		return message{}, err
	}
	text, err := agent.NudgeMessage(agent.Nudge{PR: pullRequestOf(w), Items: items})
	if err != nil {
		return message{}, err
	}
	return message{text: text, summary: "told the agent about " + agent.Summarize(items), rows: current}, nil
}

const logReads = 4

func (s *Service) attachLogs(ctx context.Context, client *github.Client, w store.Watch, items []agent.Item) error {
	jobs := logJobs(items)
	logs := make([]jobLog, len(jobs))
	g, gctx := errgroup.WithContext(ctx)
	g.SetLimit(logReads)
	for i, job := range jobs {
		g.Go(func() error {
			if err := gctx.Err(); err != nil {
				return err
			}
			var err error
			logs[i], err = s.readJobLog(gctx, client, w, job)
			return err
		})
	}
	if err := g.Wait(); err != nil {
		return err
	}
	byJob := make(map[int64]jobLog, len(jobs))
	for i, job := range jobs {
		byJob[job] = logs[i]
	}
	told := map[int64]bool{}
	for i := range items {
		if !wantsLog(items[i]) {
			continue
		}
		job := items[i].JobID
		l := byJob[job]
		items[i].NoLog = l.unserved
		if !told[job] {
			items[i].Log, items[i].LogStep = l.text, l.step
		}
		told[job] = true
	}
	return nil
}

type jobLog struct {
	text, step string
	unserved   bool
}

func logJobs(items []agent.Item) []int64 {
	var jobs []int64
	for _, it := range items {
		if wantsLog(it) && !slices.Contains(jobs, it.JobID) {
			jobs = append(jobs, it.JobID)
		}
	}
	return jobs
}

func (s *Service) readJobLog(ctx context.Context, client *github.Client, w store.Watch, job int64) (jobLog, error) {
	var out jobLog
	raw, resp, err := ghclient.JobLogs(ctx, client, w.Owner, w.Name, job)
	if err := s.guard.After(ctx, resp, err); err != nil {
		if errors.Is(err, ghclient.ErrPaused) {
			return jobLog{}, err
		}
		out.unserved = ghclient.IsGone(err)
		s.log.Warn("read the log of a failed job", "watch", w.ID, "job", job, "err", redact.Text(err.Error()))
	}
	if raw == "" {
		return out, nil
	}
	ex := checks.TrimLog(raw, checks.LogOptions{})
	out.text, out.step = redact.Text(ex.Text), redact.Text(ex.Step)
	return out, nil
}

func wantsLog(it agent.Item) bool {
	return it.Kind == store.ActivityCheckFailed && it.JobID != 0
}

func currentItems(items []store.Activity, headSHA string) (current, stale []store.Activity) {
	for _, a := range items {
		if staleCheck(a, headSHA) {
			stale = append(stale, a)
			continue
		}
		current = append(current, a)
	}
	return current, stale
}

func staleCheck(a store.Activity, headSHA string) bool {
	if !boundToHead(a.Kind) || headSHA == "" {
		return false
	}
	var p struct {
		SHA string `json:"sha"`
	}
	if err := json.Unmarshal(a.Payload, &p); err != nil {
		return false
	}
	return p.SHA != "" && p.SHA != headSHA
}

func boundToHead(kind store.ActivityKind) bool {
	return slices.Contains([]store.ActivityKind{store.ActivityCheckFailed, store.ActivityBehind, store.ActivityConflict}, kind)
}

func ids(as []store.Activity) []int64 {
	out := make([]int64, 0, len(as))
	for _, a := range as {
		out = append(out, a.ID)
	}
	return out
}

func (s *Service) maybeHeartbeat(ctx context.Context, w store.Watch, st State, now time.Time) error {
	if s.heartbeat <= 0 {
		return nil
	}
	last := w.StartedAt
	if w.LastHeartbeatAt != nil {
		last = *w.LastHeartbeatAt
	}
	if now.Sub(last) < s.heartbeat {
		return nil
	}
	row, isNew, err := s.store.InsertActivity(ctx, store.Activity{
		WatchID: w.ID, Kind: store.ActivityHeartbeat, Ref: now.UTC().Truncate(s.heartbeat).Format(time.RFC3339), At: now,
		Summary: fmt.Sprintf("still watching, head %s, checks %s, %s", textx.ShortSHA(st.HeadSHA), checks.Summarize(st.Checks, st.HeadSHA, st.GreenSHA), mergeWord(st.MergeableState)),
		URL:     w.URL,
	})
	if err != nil {
		return err
	}
	if isNew {
		s.report(ctx, w, []store.Activity{row})
	}
	return s.store.SetWatchHeartbeat(ctx, w.ID, now)
}
