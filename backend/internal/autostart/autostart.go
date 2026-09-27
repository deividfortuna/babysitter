package autostart

import (
	"cmp"
	"context"
	"errors"
	"log/slog"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/deividfortuna/babysitter/internal/agent"
	"github.com/deividfortuna/babysitter/internal/dependabot"
	"github.com/deividfortuna/babysitter/internal/prwatch"
	"github.com/deividfortuna/babysitter/internal/redact"
	"github.com/deividfortuna/babysitter/internal/snapshot"
	"github.com/deividfortuna/babysitter/internal/store"
)

const loginTTL = 30 * time.Minute

type QueueStore interface {
	GetRepoConfig(ctx context.Context, repoID int64) (store.RepoConfig, error)
	OpenPRs(ctx context.Context, repoID int64) ([]store.PullRequest, error)
	HadWatch(ctx context.Context, key store.WatchKey) (bool, error)
	AutoStartClaimed(ctx context.Context, repoID int64, number int) (bool, error)
}

type Store interface {
	QueueStore
	ListRepos(ctx context.Context) ([]store.Repo, error)
	ListWatches(ctx context.Context, o store.ListWatchesOptions) ([]store.Watch, error)
	ClaimAutoStart(ctx context.Context, repoID int64, number int, now time.Time) error
	ReleaseAutoStart(ctx context.Context, repoID int64, number int) error
}

type StartFunc func(ctx context.Context, req prwatch.StartRequest) (store.Watch, error)

type LoginFunc func(ctx context.Context) (string, error)

type Deps struct {
	Store Store
	Start StartFunc
	Login LoginFunc
	Log   *slog.Logger
	Now   func() time.Time
}

type Starter struct {
	store Store
	start StartFunc
	login LoginFunc
	log   *slog.Logger
	now   func() time.Time

	mu      sync.Mutex
	viewer  string
	viewAt  time.Time
	skipped map[forkKey]bool
}

type forkKey struct {
	repoID int64
	number int
}

func New(d Deps) *Starter {
	s := &Starter{
		store:   d.Store,
		start:   d.Start,
		login:   d.Login,
		log:     cmp.Or(d.Log, slog.New(slog.DiscardHandler)),
		now:     d.Now,
		skipped: map[forkKey]bool{},
	}
	if s.now == nil {
		s.now = time.Now
	}
	return s
}

func (s *Starter) Run(ctx context.Context) {
	repos, err := s.store.ListRepos(ctx)
	if err != nil {
		s.log.Error("auto start: list the repositories", "err", err)
		return
	}
	for _, repo := range repos {
		if ctx.Err() != nil {
			return
		}
		if err := s.runRepo(ctx, repo); err != nil {
			s.log.Error("auto start failed", "repo", repo.FullName(), "err", redact.Err(err))
		}
	}
}

func (s *Starter) runRepo(ctx context.Context, repo store.Repo) error {
	cfg, err := s.store.GetRepoConfig(ctx, repo.ID)
	if err != nil || !cfg.AutoStarts() {
		return err
	}
	login, err := s.viewerLogin(ctx, cfg)
	if err != nil {
		return err
	}
	prs, err := s.store.OpenPRs(ctx, repo.ID)
	if err != nil {
		return err
	}
	open, err := s.untaken(ctx, repo, eligible(cfg, login, prs))
	if err != nil {
		return err
	}
	var updates []store.PullRequest
	for _, c := range open {
		if c.reason == store.AutoDependabot {
			updates = append(updates, c.pr)
			continue
		}
		s.startOne(ctx, repo, cfg, c)
	}
	running, err := runningUpdates(ctx, s.store, repo)
	if err != nil {
		return err
	}
	for _, pr := range firstN(updates, cfg.DependabotLimit-running) {
		s.startOne(ctx, repo, cfg, candidate{pr: pr, reason: store.AutoDependabot})
	}
	return nil
}

func (s *Starter) viewerLogin(ctx context.Context, cfg store.RepoConfig) (string, error) {
	if !cfg.OwnOn() || s.login == nil {
		return "", nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.viewer != "" && s.now().Sub(s.viewAt) < loginTTL {
		return s.viewer, nil
	}
	login, err := s.login(ctx)
	if err != nil {
		return "", err
	}
	s.viewer, s.viewAt = login, s.now()
	return login, nil
}

func (s *Starter) untaken(ctx context.Context, repo store.Repo, cs []candidate) ([]candidate, error) {
	out := make([]candidate, 0, len(cs))
	for _, c := range cs {
		if c.pr.Fork {
			s.skipFork(repo, c.pr)
			continue
		}
		taken, err := isTaken(ctx, s.store, repo, c.pr.Number)
		if err != nil {
			return nil, err
		}
		if !taken {
			out = append(out, c)
		}
	}
	return out, nil
}

func (s *Starter) skipFork(repo store.Repo, pr store.PullRequest) {
	key := forkKey{repo.ID, pr.Number}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.skipped[key] {
		return
	}
	s.skipped[key] = true
	s.log.Info("auto start skips a pull request from a fork: the agent cannot push to it",
		"pr", repo.FullName()+"#"+strconv.Itoa(pr.Number), "title", pr.Title)
}

func (s *Starter) startOne(ctx context.Context, repo store.Repo, cfg store.RepoConfig, c candidate) {
	label := repo.FullName() + "#" + strconv.Itoa(c.pr.Number)
	err := s.store.ClaimAutoStart(ctx, repo.ID, c.pr.Number, s.now())
	if errors.Is(err, store.ErrAlreadyClaimed) {
		s.log.Debug("auto start: another daemon took the pull request", "pr", label)
		return
	}
	if err != nil {
		s.log.Error("auto start: claim the pull request", "pr", label, "err", err)
		return
	}
	w, err := s.start(ctx, request(repo, cfg, c))
	if errors.Is(err, store.ErrWatchExists) {
		s.log.Debug("auto start: the pull request is already watched", "pr", label)
		return
	}
	if err != nil {
		s.log.Warn("auto start: the watch did not start, the next pass tries again", "pr", label, "err", redact.Err(err))
		if rerr := s.store.ReleaseAutoStart(context.WithoutCancel(ctx), repo.ID, c.pr.Number); rerr != nil {
			s.log.Error("auto start: release the pull request", "pr", label, "err", rerr)
		}
		return
	}
	s.log.Info("auto start began a watch", "pr", label, "watch", w.ID, "reason", c.reason)
}

func request(repo store.Repo, cfg store.RepoConfig, c candidate) prwatch.StartRequest {
	o := cfg.Overrides
	req := prwatch.StartRequest{
		Target:            snapshot.Target{Owner: repo.Owner, Name: repo.Name, Number: c.pr.Number},
		Provider:          o.Provider,
		Model:             o.Model,
		SourceDir:         cfg.CheckoutDir,
		IncludeExisting:   o.IncludeExisting,
		ApprovalsRequired: prwatch.Approvals{Set: o.ApprovalsSet, Count: o.Approvals},
		AutoReason:        c.reason,
	}
	if o.MergeMethod != "" {
		req.MergeMethod = &o.MergeMethod
	}
	if o.ApprovalMode != "" {
		req.ApprovalMode = &o.ApprovalMode
	}
	if c.reason == store.AutoDependabot {
		inScope := dependabot.Within(c.pr.UpdateType, cfg.DependabotScope)
		req.UpdateType = c.pr.UpdateType
		req.MergeWhenReady = &inScope
	}
	return req
}

type candidate struct {
	pr     store.PullRequest
	reason store.AutoReason
}

func eligible(cfg store.RepoConfig, login string, prs []store.PullRequest) []candidate {
	var out []candidate
	for _, pr := range prs {
		if reason, ok := reasonFor(cfg, login, pr); ok {
			out = append(out, candidate{pr: pr, reason: reason})
		}
	}
	return out
}

func reasonFor(cfg store.RepoConfig, login string, pr store.PullRequest) (store.AutoReason, bool) {
	if agent.IsDependabot(pr.Author) {
		return store.AutoDependabot, cfg.DependabotOn() && openedSince(pr, cfg.DependabotSince)
	}
	reason := ownReason(login, pr)
	ready := !pr.Draft || cfg.IncludeDrafts
	return reason, reason != store.AutoNone && cfg.OwnOn() && openedSince(pr, cfg.OwnSince) && ready
}

func ownReason(login string, pr store.PullRequest) store.AutoReason {
	switch {
	case login == "":
		return store.AutoNone
	case strings.EqualFold(pr.Author, login):
		return store.AutoMine
	case pr.AssignedTo(login):
		return store.AutoAssigned
	default:
		return store.AutoNone
	}
}

func openedSince(pr store.PullRequest, since *time.Time) bool {
	return since != nil && !pr.CreatedAt.Before(*since)
}

func isTaken(ctx context.Context, st QueueStore, repo store.Repo, number int) (bool, error) {
	claimed, err := st.AutoStartClaimed(ctx, repo.ID, number)
	if err != nil || claimed {
		return claimed, err
	}
	return st.HadWatch(ctx, store.WatchKey{Owner: repo.Owner, Name: repo.Name, Number: number})
}

func runningUpdates(ctx context.Context, st Store, repo store.Repo) (int, error) {
	active, err := st.ListWatches(ctx, store.ListWatchesOptions{Status: store.WatchActive})
	if err != nil {
		return 0, err
	}
	n := 0
	for _, w := range active {
		if strings.EqualFold(w.Repo(), repo.FullName()) && agent.IsDependabot(w.Author) {
			n++
		}
	}
	return n, nil
}

func firstN(prs []store.PullRequest, n int) []store.PullRequest {
	slices.SortStableFunc(prs, oldestFirst)
	return prs[:max(0, min(n, len(prs)))]
}

func oldestFirst(a, b store.PullRequest) int {
	return cmp.Or(a.CreatedAt.Compare(b.CreatedAt), cmp.Compare(a.Number, b.Number))
}
