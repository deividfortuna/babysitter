package prwatch

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"

	"github.com/google/go-github/v91/github"

	"github.com/deividfortuna/babysitter/internal/agent"
	"github.com/deividfortuna/babysitter/internal/checks"
	"github.com/deividfortuna/babysitter/internal/dependabot"
	"github.com/deividfortuna/babysitter/internal/ghclient"
	"github.com/deividfortuna/babysitter/internal/gitrepo"
	"github.com/deividfortuna/babysitter/internal/redact"
	"github.com/deividfortuna/babysitter/internal/snapshot"
	"github.com/deividfortuna/babysitter/internal/store"
	"github.com/deividfortuna/babysitter/internal/textx"
	"github.com/deividfortuna/babysitter/internal/worktree"
)

type StartRequest struct {
	Target            snapshot.Target
	Provider          string
	Model             string
	Effort            string
	SourceDir         string
	IncludeExisting   *bool
	IncludeOwn        *bool
	ApprovalsRequired Approvals
	MergeMethod       *string
	ApprovalMode      *store.ApprovalMode
	AutoApproveRebase *bool
	MergeWhenReady    *bool
	KeepWorktree      *bool
	BranchUpdate      *store.BranchUpdate
	UpdateOnGitHub    *bool
	AutoReason        store.AutoReason
	UpdateType        dependabot.Level
}

func (r StartRequest) approvalMode(provider string) store.ApprovalMode {
	if !hostedProvider(provider) {
		return store.ApprovalAuto
	}
	return *r.ApprovalMode
}

func ignoredAuthor(w store.Watch) string {
	if w.IncludeOwn {
		return ""
	}
	return w.BotLogin
}

type source struct {
	dir       string
	headOwner string
	headName  string
}

type checkout struct {
	source
	method string
	agentChoice
}

type access struct {
	botLogin  string
	userName  string
	userEmail string
}

func (s *Service) checkCheckout(ctx context.Context, req StartRequest) (checkout, error) {
	chosen, err := normalizeAgent(req.Provider, req.Model, req.Effort)
	if err != nil {
		return checkout{}, err
	}
	if _, err := normalizeEffort(chosen.provider, s.launchedModel(chosen), chosen.effort); err != nil {
		return checkout{}, err
	}
	if hostedProvider(chosen.provider) && s.lacksRunner(chosen.provider) {
		return checkout{}, fmt.Errorf("%w: %s", ErrNoAgent, chosen.provider)
	}
	method, ok := normalizeMergeMethod(*req.MergeMethod)
	if !ok {
		return checkout{}, fmt.Errorf("%w: %q", ErrBadMergeMethod, *req.MergeMethod)
	}
	if !req.ApprovalMode.Valid() {
		return checkout{}, fmt.Errorf("%w: %q", ErrBadApprovalMode, *req.ApprovalMode)
	}
	if !req.BranchUpdate.Valid() {
		return checkout{}, fmt.Errorf("%w: %q", ErrBadBranchUpdate, *req.BranchUpdate)
	}
	src, err := givenSource(ctx, req.SourceDir, chosen.provider)
	if err != nil {
		return checkout{}, err
	}
	return checkout{source: src, method: method, agentChoice: chosen}, nil
}

func givenSource(ctx context.Context, dir, provider string) (source, error) {
	if dir != "" {
		return readSource(ctx, dir)
	}
	if !hostedProvider(provider) {
		return source{}, fmt.Errorf("%w: the %s provider works in the checkout of its session", ErrNoCheckout, provider)
	}
	return source{}, nil
}

func (s *Service) orManagedSource(ctx context.Context, src source, headRepo string) (source, error) {
	if src.dir != "" {
		return src, nil
	}
	if s.checkouts == nil {
		return source{}, fmt.Errorf("%w: this daemon makes no checkout of its own", ErrNoCheckout)
	}
	headOwner, headName, err := store.ParseFullName(headRepo)
	if err != nil {
		return source{}, err
	}
	dir, err := s.checkouts.Ensure(ctx, headRepo)
	if err != nil {
		return source{}, err
	}
	return source{dir: dir, headOwner: headOwner, headName: headName}, nil
}

func readSource(ctx context.Context, dir string) (source, error) {
	if info, err := os.Stat(dir); err != nil || !info.IsDir() {
		return source{}, fmt.Errorf("source directory %q is not a directory", dir)
	}
	origin, err := gitrepo.RemoteURL(ctx, dir, "origin")
	if err != nil {
		return source{}, fmt.Errorf("%s is not a git checkout with an origin remote: %w", dir, err)
	}
	headOwner, headName, err := store.ParseFullName(origin)
	if err != nil {
		return source{}, fmt.Errorf("origin of %s: %w", dir, err)
	}
	return source{dir: dir, headOwner: headOwner, headName: headName}, nil
}

func (s *Service) lacksRunner(provider string) bool {
	return len(s.agents) > 0 && s.agents[provider] == nil
}

func (s *Service) launchedModel(chosen agentChoice) string {
	runner := s.agents[chosen.provider]
	if runner == nil {
		return chosen.model
	}
	launched := normalized(agent.PickModel(runner.DefaultModel(), chosen.model))
	if _, known := modelOf(chosen.provider, launched); !known {
		return chosen.model
	}
	return launched
}

func checkHeadRepo(pr snapshot.PR, c checkout) error {
	if !strings.EqualFold(pr.HeadRepo, c.headOwner+"/"+c.headName) {
		return fmt.Errorf("%w: head is %s, origin is %s/%s", ErrWrongRepo, pr.HeadRepo, c.headOwner, c.headName)
	}
	return nil
}

func (s *Service) checkStart(ctx context.Context, client *github.Client, req StartRequest, c checkout, key store.WatchKey, pr snapshot.PR) (access, int, error) {
	headCtx, stopHead := context.WithCancel(ctx)
	defer stopHead()
	head := make(chan error, 1)
	go func() { head <- s.readyHead(headCtx, c, pr.HeadBranch) }()

	var (
		acc       access
		approvals int
		wg        sync.WaitGroup
	)
	errs := make([]error, 4)
	wg.Go(func() { errs[0] = checkPush(ctx, client, c) })
	wg.Go(func() { acc.botLogin, errs[1] = currentLogin(ctx, client) })
	wg.Go(func() { acc.userName, acc.userEmail, errs[2] = gitIdentity(ctx, c.dir) })
	wg.Go(func() { approvals, errs[3] = s.approvalsRequired(ctx, client, req, key, pr.BaseBranch) })
	wg.Wait()
	if err := cmp.Or(errs...); err != nil {
		stopHead()
		<-head
		return access{}, 0, err
	}
	if err := <-head; err != nil {
		return access{}, 0, err
	}
	return acc, approvals, nil
}

func (s *Service) readyHead(ctx context.Context, c checkout, headRef string) error {
	if !hostedProvider(c.provider) {
		return checkHeadBranch(ctx, c.dir, headRef)
	}
	if s.git == nil {
		return nil
	}
	return s.git.Fetch(ctx, c.dir, "origin/"+headRef)
}

func checkPush(ctx context.Context, client *github.Client, c checkout) error {
	repo, err := ghclient.GetRepo(ctx, client, c.headOwner, c.headName)
	if err != nil {
		return err
	}
	if !repo.GetPermissions().GetPush() {
		return fmt.Errorf("%w: %s/%s", ErrNoPushAccess, c.headOwner, c.headName)
	}
	return nil
}

func currentLogin(ctx context.Context, client *github.Client) (string, error) {
	user, err := ghclient.CurrentUser(ctx, client)
	if err != nil {
		return "", err
	}
	return user.GetLogin(), nil
}

func gitIdentity(ctx context.Context, dir string) (name, email string, err error) {
	name, err = gitrepo.ConfigValue(ctx, dir, "user.name")
	if err != nil {
		return "", "", err
	}
	email, err = gitrepo.ConfigValue(ctx, dir, "user.email")
	if err != nil {
		return "", "", err
	}
	if name == "" || email == "" {
		return "", "", ErrNoIdentity
	}
	return name, email, nil
}

func (s *Service) Start(ctx context.Context, req StartRequest) (store.Watch, error) {
	req, err := s.withDefaults(ctx, req)
	if err != nil {
		return store.Watch{}, err
	}
	co, err := s.checkCheckout(ctx, req)
	if err != nil {
		return store.Watch{}, err
	}
	client, err := s.newClient(ctx)
	if err != nil {
		return store.Watch{}, err
	}
	now := s.now()
	snap, err := snapshot.Collect(ctx, client, s.store, req.Target, s.snapshotOptions(req.SourceDir))
	if err != nil {
		return store.Watch{}, err
	}
	if snap.PR.Merged || snap.PR.Closed {
		return store.Watch{}, fmt.Errorf("%w: %s#%d is %s", ErrNotOpen, snap.PR.Repo, snap.PR.Number, snap.PR.State)
	}
	owner, name, err := store.ParseFullName(snap.PR.Repo)
	if err != nil {
		return store.Watch{}, err
	}
	key := store.WatchKey{Owner: owner, Name: name, Number: snap.PR.Number}
	if existing, err := s.store.FindActiveWatch(ctx, key); err == nil {
		return existing, store.ErrWatchExists
	} else if !errors.Is(err, store.ErrWatchNotFound) {
		return store.Watch{}, err
	}
	if co.source, err = s.orManagedSource(ctx, co.source, snap.PR.HeadRepo); err != nil {
		return store.Watch{}, err
	}
	if err := checkHeadRepo(snap.PR, co); err != nil {
		return store.Watch{}, err
	}
	acc, approvals, err := s.checkStart(ctx, client, req, co, key, snap.PR)
	if err != nil {
		return store.Watch{}, err
	}

	headRef := snap.PR.HeadBranch
	var dir, branch string
	if hostedProvider(co.provider) {
		dir, branch, err = s.makeWorktree(ctx, co.dir, key, headRef)
		if err != nil {
			return store.Watch{}, err
		}
	}

	baseline, base := Diff(State{}, snap, now)
	w, err := s.store.CreateWatch(ctx, store.Watch{
		Owner: owner, Name: name, Number: snap.PR.Number, URL: snap.PR.URL, Title: snap.PR.Title,
		Author: snap.PR.Author, AuthorAvatarURL: snap.PR.AuthorAvatarURL,
		BotLogin: acc.botLogin, HeadRef: headRef, BaseRef: snap.PR.BaseBranch,
		SourceDir: co.dir, WorktreeDir: dir, WorkBranch: branch,
		GitUserName: acc.userName, GitUserEmail: acc.userEmail,
		Provider: co.provider, Model: co.model, Effort: co.effort,
		IncludeExisting: *req.IncludeExisting, IncludeOwn: *req.IncludeOwn, StartedAt: now,
		ApprovalsRequired: approvals, MergeMethod: co.method,
		ApprovalMode: req.approvalMode(co.provider), AutoApproveRebase: *req.AutoApproveRebase,
		MergeWhenReady: req.MergeWhenReady != nil && *req.MergeWhenReady, KeepWorktree: *req.KeepWorktree, AutoReason: req.AutoReason, UpdateType: cmp.Or(req.UpdateType, snap.PR.UpdateType),
		BranchUpdate: *req.BranchUpdate, UpdateOnGitHub: *req.UpdateOnGitHub,
		HeadSHA: base.HeadSHA, PRState: base.PRState, MergeableState: base.MergeableState, CheckStates: base.Checks, GreenSHA: base.GreenSHA,
	})
	if err != nil {
		s.dropWorktree(ctx, co.dir, dir, branch)
		return store.Watch{}, err
	}
	if err := s.finishStart(ctx, client, w, snap, baseline, *req.IncludeExisting); err != nil {
		s.log.Error("finish the start of the watch", "watch", w.ID, "pr", prLabel(w), "err", err)
	}
	if fresh, err := s.store.GetWatch(ctx, w.ID); err == nil {
		w = fresh
	}
	s.Kick(w.ID)
	return w, nil
}

func checkHeadBranch(ctx context.Context, sourceDir, headRef string) error {
	current, err := gitrepo.CurrentBranch(ctx, sourceDir)
	if errors.Is(err, gitrepo.ErrDetachedHead) {
		return fmt.Errorf("%w: %s is on a detached head, the pull request head is %s", ErrWrongBranch, sourceDir, headRef)
	}
	if err != nil {
		return err
	}
	if current != headRef {
		return fmt.Errorf("%w: %s is on %s, the pull request head is %s", ErrWrongBranch, sourceDir, current, headRef)
	}
	return nil
}

var worktreeName = regexp.MustCompile(`^[a-z0-9._-]+-[0-9]+$`)

func worktreeDirName(key store.WatchKey) (string, error) {
	name := fmt.Sprintf("%s-%s-%d", strings.ToLower(key.Owner), strings.ToLower(key.Name), key.Number)
	if !worktreeName.MatchString(name) {
		return "", fmt.Errorf("%s has characters that are not safe in a directory name", key.Repo())
	}
	return name, nil
}

func (s *Service) makeWorktree(ctx context.Context, sourceDir string, key store.WatchKey, headRef string) (dir, branch string, err error) {
	name, err := worktreeDirName(key)
	if err != nil {
		return "", "", err
	}
	branch = "babysitter/" + headRef
	dir = filepath.Join(s.dataDir, "worktrees", name)
	if s.git == nil {
		return dir, branch, nil
	}
	if err := s.git.Remove(ctx, sourceDir, dir, branch); err != nil {
		s.log.Warn("remove old worktree", "dir", dir, "err", err)
	}
	if err := s.git.Create(ctx, sourceDir, dir, branch, "origin/"+headRef); err != nil {
		return "", "", err
	}
	return dir, branch, nil
}

func (s *Service) approvalsRequired(ctx context.Context, client *github.Client, req StartRequest, key store.WatchKey, base string) (int, error) {
	if n := req.ApprovalsRequired.Count; n != nil {
		return *n, nil
	}
	return s.branchApprovals(ctx, client, key, base)
}

func (s *Service) branchApprovals(ctx context.Context, client *github.Client, key store.WatchKey, base string) (int, error) {
	n, resp, err := ghclient.RequiredApprovals(ctx, client, key.Owner, key.Name, base)
	err = s.guard.After(ctx, resp, err)
	if errors.Is(err, ghclient.ErrPaused) || ctx.Err() != nil {
		return 0, err
	}
	if err != nil {
		s.log.Warn("read the approvals the base branch requires, asking for one", "pr", key.Owner+"/"+key.Name+"#"+fmt.Sprint(key.Number), "base", base, "err", redact.Text(err.Error()))
		return 1, nil
	}
	return n, nil
}

func (s *Service) dropWorktree(ctx context.Context, sourceDir, dir, branch string) {
	if s.git == nil || dir == "" {
		return
	}
	if err := s.git.Remove(context.WithoutCancel(ctx), sourceDir, dir, branch); err != nil {
		s.log.Warn("remove the worktree of a watch that was not created", "dir", dir, "err", err)
	}
}

func (s *Service) finishStart(ctx context.Context, client *github.Client, w store.Watch, snap *snapshot.Snapshot, baseline []store.Activity, includeExisting bool) error {
	if includeExisting {
		if err := s.store.ForgetReviewItems(ctx, w.Key()); err != nil {
			return err
		}
	} else {
		if err := snap.Commit(ctx, s.store); err != nil {
			return err
		}
	}
	if _, err := s.record(ctx, w, store.Activity{
		Kind: store.ActivityWatchStarted, Ref: "start", At: w.StartedAt,
		Summary: fmt.Sprintf("watching %s#%d (%s) from %s, checks %s", w.Repo(), w.Number, w.HeadRef, w.SourceDir, s.cadence()),
		Payload: mustJSON(map[string]any{"head_sha": w.HeadSHA, "source_dir": w.SourceDir, "worktree_dir": w.WorktreeDir}),
	}); err != nil {
		return err
	}
	if err := s.recordAutoStart(ctx, w); err != nil {
		return err
	}
	for _, a := range baseline {
		if !workAtStart(a.Kind) {
			continue
		}
		if _, err := s.record(ctx, w, a); err != nil {
			return err
		}
	}
	unlock := s.locks.Lock(w.ID)
	defer unlock()
	stepErr := s.githubStep(ctx, client, w, snap.PR)
	if stepErr != nil {
		s.log.Error("update the branch on GitHub", "watch", w.ID, "pr", prLabel(w), "err", stepErr)
	}
	if !s.runs(w) {
		return nil
	}
	if _, err := s.ensureSession(ctx, w); err != nil {
		s.agentFailed(ctx, w, "start the agent session", err)
		return nil
	}
	if stepErr != nil {
		return nil
	}
	_, err := s.tell(ctx, client, w)
	return err
}

func (s *Service) recordAutoStart(ctx context.Context, w store.Watch) error {
	if w.AutoReason == store.AutoNone {
		return nil
	}
	_, err := s.record(ctx, w, store.Activity{
		Kind: store.ActivityAutoStarted, Ref: "auto", At: w.StartedAt,
		Summary: "started on its own: " + w.AutoReason.Word(),
		Payload: mustJSON(map[string]any{"reason": w.AutoReason, "update_type": w.UpdateType}),
	})
	return err
}

func workAtStart(kind store.ActivityKind) bool {
	switch kind {
	case store.ActivityBehind, store.ActivityConflict, store.ActivityCheckFailed:
		return true
	default:
		return false
	}
}

type StopOptions struct {
	KeepWorktree *bool
}

func (s *Service) Stop(ctx context.Context, id int64, o StopOptions) (store.Watch, error) {
	unlock := s.locks.Lock(id)
	defer unlock()
	return s.stop(ctx, id, store.StopUser, "", o)
}

type Summary struct {
	PRState         store.PRState              `json:"prState"`
	HeadSHA         string                     `json:"headSha"`
	Checks          string                     `json:"checks"`
	MergeableState  store.MergeableState       `json:"mergeableState"`
	Activity        map[store.ActivityKind]int `json:"activity"`
	Messages        int                        `json:"messages"`
	Reason          store.StopReason           `json:"reason"`
	Detail          string                     `json:"detail,omitempty"`
	WorktreeRemoved bool                       `json:"worktreeRemoved"`
	WorkBranchLeft  string                     `json:"workBranchLeft,omitempty"`
}

func (s *Service) stop(ctx context.Context, id int64, reason store.StopReason, detail string, o StopOptions) (store.Watch, error) {
	w, err := s.store.GetWatch(ctx, id)
	if err != nil {
		return store.Watch{}, err
	}
	if w.Status != store.WatchActive {
		return w, nil
	}
	now := s.now()
	sum, err := s.summary(ctx, w, reason, detail)
	if err != nil {
		return store.Watch{}, err
	}
	w, err = s.store.StopWatch(ctx, id, reason, mustJSON(sum), now)
	if err != nil {
		return store.Watch{}, err
	}
	s.endSession(ctx, w)
	waiting, err := s.store.DeclineWaitingProposals(ctx, w.ID)
	if err != nil {
		s.log.Error("decline the proposals of a stopped watch", "watch", w.ID, "err", err)
	}
	declined := proposalNumbers(waiting)
	s.work.clear(w.ID)
	s.rereviewTries.drop(w.ID)
	s.branchTries.drop(w.ID)
	fate := worktreeNotTried
	keep := s.keepsWorktree(w, o)
	if !keep {
		fate = s.removeWorktree(ctx, w)
	}
	if fate == worktreeBranchLeft {
		sum.WorkBranchLeft = w.WorkBranch
	}
	if fate.gone() {
		sum.WorktreeRemoved = true
		w.Summary = mustJSON(sum)
		if _, serr := s.store.SetWatchSummary(ctx, w.ID, w.Summary); serr != nil {
			s.log.Error("record the worktree removal", "watch", w.ID, "dir", w.WorktreeDir, "err", serr)
		}
	}
	text := fmt.Sprintf("stopped watching (%s): %s at %s, checks %s", reason.Word(), sum.PRState, textx.ShortSHA(sum.HeadSHA), sum.Checks)
	text += "; " + worktreeWord(w, keep, fate)
	if len(declined) > 0 {
		text += "; " + declinedWord(declined)
	}
	if _, err := s.record(ctx, w, store.Activity{
		Kind: store.ActivityWatchStopped, Ref: "stop", At: now, Summary: text, Payload: mustJSON(sum),
	}); err != nil {
		return store.Watch{}, err
	}
	return w, nil
}

func proposalNumbers(ps []store.Proposal) []int {
	numbers := make([]int, 0, len(ps))
	for _, p := range ps {
		numbers = append(numbers, p.Number)
	}
	return numbers
}

func declinedWord(numbers []int) string {
	words := make([]string, 0, len(numbers))
	for _, n := range numbers {
		words = append(words, strconv.Itoa(n))
	}
	if len(words) == 1 {
		return "proposal " + words[0] + " declined"
	}
	return "proposals " + textx.JoinAnd(words) + " declined"
}

type worktreeFate int

const (
	worktreeNotTried worktreeFate = iota
	worktreeRemoved
	worktreeBranchLeft
	worktreeRemovalFailed
)

func (f worktreeFate) gone() bool {
	return f == worktreeRemoved || f == worktreeBranchLeft
}

func (s *Service) removeWorktree(ctx context.Context, w store.Watch) worktreeFate {
	if s.git == nil || w.WorktreeDir == "" {
		return worktreeNotTried
	}
	if err := s.git.Remove(ctx, w.SourceDir, w.WorktreeDir, w.WorkBranch); err != nil {
		s.log.Warn("remove worktree", "watch", w.ID, "dir", w.WorktreeDir, "err", err)
		if errors.Is(err, worktree.ErrBranchLeft) {
			return worktreeBranchLeft
		}
		return worktreeRemovalFailed
	}
	return worktreeRemoved
}

func worktreeWord(w store.Watch, keep bool, fate worktreeFate) string {
	switch {
	case fate == worktreeRemoved:
		return "worktree removed"
	case fate == worktreeBranchLeft:
		return "worktree removed, its branch " + w.WorkBranch + " stays in " + w.SourceDir
	case w.WorktreeDir == "":
		return "no worktree"
	case fate == worktreeRemovalFailed:
		return "worktree left at " + w.WorktreeDir + ", its removal failed"
	case w.TakenOverAt != nil:
		return "worktree kept: the session was with you"
	case keep:
		return "worktree kept at " + w.WorktreeDir
	default:
		return "worktree left at " + w.WorktreeDir
	}
}

func (s *Service) summary(ctx context.Context, w store.Watch, reason store.StopReason, detail string) (Summary, error) {
	counts, err := s.store.CountActivity(ctx, w.ID)
	if err != nil {
		return Summary{}, err
	}
	return Summary{
		PRState: cmp.Or(w.PRState, "open"), HeadSHA: w.HeadSHA, Checks: checks.Summarize(w.CheckStates, w.HeadSHA, w.GreenSHA), MergeableState: w.MergeableState,
		Activity: counts, Messages: counts[store.ActivityNudged], Reason: reason, Detail: redact.Text(detail),
	}, nil
}

func mergeWord(mergeable store.MergeableState) string {
	switch mergeable {
	case "":
		return "mergeability unknown"
	case store.MergeableClean:
		return "mergeable"
	case store.MergeableDirty:
		return "conflicts with base"
	default:
		return string(mergeable)
	}
}

func target(w store.Watch) snapshot.Target {
	return snapshot.Target{Owner: w.Owner, Name: w.Name, Number: w.Number}
}

func lostAccess(err error) bool {
	return ghclient.IsNotFound(err) || ghclient.IsDenied(err)
}
