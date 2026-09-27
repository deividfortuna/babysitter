package httpd

import (
	"net/http"
	"time"

	"github.com/deividfortuna/babysitter/internal/autostart"
	"github.com/deividfortuna/babysitter/internal/dependabot"
	"github.com/deividfortuna/babysitter/internal/store"
)

type WatchOverrides struct {
	Provider          string        `json:"provider" enum:",claude,copilot" description:"The AI provider of the watches auto start begins; empty takes the provider of watch start"`
	Model             string        `json:"model" description:"The model of the agent; empty takes the model of the provider"`
	ApprovalMode      string        `json:"approvalMode" enum:",auto,manual" description:"Who releases the work of a turn of the agent; empty takes the setting of the daemon"`
	MergeMethod       string        `json:"mergeMethod" enum:",squash,merge,rebase" description:"The merge method of the watches; empty takes the setting of the daemon"`
	ApprovalsRequired Optional[int] `json:"approvalsRequired,omitzero" minimum:"0" nullable:"true" description:"How many approvals the pull request needs; absent takes the setting of the daemon, 0 asks for none, and null asks for the rule of the base branch"`
	IncludeExisting   *bool         `json:"includeExisting,omitempty" description:"Report the review items the pull request has already; absent takes the setting of the daemon"`
}

type RepoConfig struct {
	RepoID              int64          `json:"repoId"`
	Repo                string         `json:"repo"`
	CheckoutDir         string         `json:"checkoutDir" description:"The checkout auto start makes each worktree from; the toggles stay off without it"`
	AutoStartMine       bool           `json:"autoStartMine" description:"Start a watch on each new pull request that the author opened or that is assigned to the author"`
	AutoStartMineSince  *time.Time     `json:"autoStartMineSince,omitempty" description:"When the toggle went on; only pull requests created from then on start"`
	IncludeDrafts       bool           `json:"includeDrafts" description:"Auto start also takes a draft of the author"`
	AutoWatchDependabot bool           `json:"autoWatchDependabot" description:"Start a watch on each new pull request of Dependabot"`
	AutoWatchSince      *time.Time     `json:"autoWatchDependabotSince,omitempty" description:"When the toggle went on; only pull requests created from then on start"`
	Overrides           WatchOverrides `json:"overrides"`
	DependabotScope     string         `json:"dependabotScope" enum:"patch,minor,major" description:"The highest Dependabot update that merges on its own"`
	DependabotApproval  string         `json:"dependabotApproval" enum:"never,ask,green" description:"never: the daemon submits no review; ask: a notification asks you to approve a green update in scope; green: the daemon approves a green update in scope in your name"`
	DependabotLimit     int            `json:"dependabotLimit" minimum:"1" description:"How many Dependabot watches run at the same time; the rest wait in the queue"`
}

type UpdateRepoConfigRequest struct {
	CheckoutDir         *string         `json:"checkoutDir,omitempty" description:"A git checkout whose origin is the repository; empty clears it"`
	AutoStartMine       *bool           `json:"autoStartMine,omitempty"`
	IncludeDrafts       *bool           `json:"includeDrafts,omitempty"`
	AutoWatchDependabot *bool           `json:"autoWatchDependabot,omitempty"`
	Overrides           *WatchOverrides `json:"overrides,omitempty" description:"Replaces every override at once"`
	DependabotScope     *string         `json:"dependabotScope,omitempty" enum:"patch,minor,major"`
	DependabotApproval  *string         `json:"dependabotApproval,omitempty" enum:"never,ask,green"`
	DependabotLimit     *int            `json:"dependabotLimit,omitempty" minimum:"1"`
}

type QueuedPullRequest struct {
	Number     int              `json:"number"`
	Title      string           `json:"title"`
	URL        string           `json:"url"`
	UpdateType dependabot.Level `json:"updateType" enum:"patch,minor,major"`
	CreatedAt  time.Time        `json:"createdAt"`
	Position   int              `json:"position" description:"1 starts next"`
}

type RepoQueue struct {
	Repo         string              `json:"repo"`
	PullRequests []QueuedPullRequest `json:"pullRequests" description:"The Dependabot pull requests that wait for a place, oldest first"`
}

func repoConfigOut(repo store.Repo, c store.RepoConfig) RepoConfig {
	o := c.Overrides
	return RepoConfig{
		RepoID: repo.ID, Repo: repo.FullName(), CheckoutDir: c.CheckoutDir,
		AutoStartMine: c.OwnOn(), AutoStartMineSince: c.OwnSince, IncludeDrafts: c.IncludeDrafts,
		AutoWatchDependabot: c.DependabotOn(), AutoWatchSince: c.DependabotSince,
		Overrides: WatchOverrides{
			Provider: o.Provider, Model: o.Model, ApprovalMode: string(o.ApprovalMode), MergeMethod: o.MergeMethod,
			ApprovalsRequired: Optional[int]{Set: o.ApprovalsSet, Value: o.Approvals}, IncludeExisting: o.IncludeExisting,
		},
		DependabotScope: string(c.DependabotScope), DependabotApproval: string(c.DependabotApproval), DependabotLimit: c.DependabotLimit,
	}
}

func (req UpdateRepoConfigRequest) change() autostart.Change {
	c := autostart.Change{
		CheckoutDir: req.CheckoutDir, AutoStartMine: req.AutoStartMine, IncludeDrafts: req.IncludeDrafts,
		AutoWatchDependabot: req.AutoWatchDependabot, DependabotLimit: req.DependabotLimit,
	}
	if o := req.Overrides; o != nil {
		c.Overrides = &store.WatchOverrides{
			Provider: o.Provider, Model: o.Model, ApprovalMode: store.ApprovalMode(o.ApprovalMode), MergeMethod: o.MergeMethod,
			ApprovalsSet: o.ApprovalsRequired.Set, Approvals: o.ApprovalsRequired.Value, IncludeExisting: o.IncludeExisting,
		}
	}
	if req.DependabotScope != nil {
		scope := dependabot.Level(*req.DependabotScope)
		c.DependabotScope = &scope
	}
	if req.DependabotApproval != nil {
		approval := store.DependabotApproval(*req.DependabotApproval)
		c.DependabotApproval = &approval
	}
	return c
}

var repoConfigErrors = newErrorMap("store_failed",
	notFound("repository_not_found", store.ErrRepoNotFound),
	badRequest("invalid_checkout", autostart.ErrBadCheckout),
	badRequest("invalid_config", store.ErrInvalidRepoConfig),
)

func (a *api) repoOf(w http.ResponseWriter, r *http.Request) (store.Repo, bool) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return store.Repo{}, false
	}
	repo, err := a.store.GetRepoByID(r.Context(), id)
	if repoConfigErrors.write(w, err) {
		return store.Repo{}, false
	}
	return repo, true
}

func (a *api) handleGetRepoConfig(w http.ResponseWriter, r *http.Request) {
	repo, ok := a.repoOf(w, r)
	if !ok {
		return
	}
	c, err := a.store.GetRepoConfig(r.Context(), repo.ID)
	if repoConfigErrors.write(w, err) {
		return
	}
	writeJSON(w, http.StatusOK, repoConfigOut(repo, c))
}

func (a *api) handleUpdateRepoConfig(w http.ResponseWriter, r *http.Request) {
	repo, ok := a.repoOf(w, r)
	if !ok {
		return
	}
	var req UpdateRepoConfigRequest
	if err := readJSON(w, r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", "body must be a JSON object with the fields of the repository configuration")
		return
	}
	c, err := autostart.Configure(r.Context(), a.store, repo, req.change(), time.Now())
	if repoConfigErrors.write(w, err) {
		return
	}
	a.syncer.Kick()
	writeJSON(w, http.StatusOK, repoConfigOut(repo, c))
}

func (a *api) handleRepoQueue(w http.ResponseWriter, r *http.Request) {
	repo, ok := a.repoOf(w, r)
	if !ok {
		return
	}
	prs, err := autostart.Queue(r.Context(), a.store, repo)
	if repoConfigErrors.write(w, err) {
		return
	}
	writeJSON(w, http.StatusOK, repoQueueOut(repo, prs))
}

func repoQueueOut(repo store.Repo, prs []store.PullRequest) RepoQueue {
	out := RepoQueue{Repo: repo.FullName(), PullRequests: make([]QueuedPullRequest, 0, len(prs))}
	for i, pr := range prs {
		out.PullRequests = append(out.PullRequests, QueuedPullRequest{
			Number: pr.Number, Title: pr.Title, URL: pr.HTMLURL, UpdateType: pr.UpdateType, CreatedAt: pr.CreatedAt, Position: i + 1,
		})
	}
	return out
}
