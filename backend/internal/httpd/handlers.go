package httpd

import (
	"net/http"

	"github.com/deividfortuna/babysitter/internal/store"
)

func (a *api) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, Health{
		Status:    "ok",
		Version:   a.version,
		PID:       a.pid,
		StartedAt: a.startedAt.Format(timeLayout),
		Name:      a.name,
	})
}

func (a *api) handleListRepos(w http.ResponseWriter, r *http.Request) {
	repos, err := a.store.ListRepos(r.Context())
	if storeErrors.write(w, err) {
		return
	}
	out := RepoList{Repos: make([]Repo, 0, len(repos))}
	for _, repo := range repos {
		out.Repos = append(out.Repos, repoFromStore(repo))
	}
	writeJSON(w, http.StatusOK, out)
}

func (a *api) handleAddRepo(w http.ResponseWriter, r *http.Request) {
	var req AddRepoRequest
	if err := readJSON(w, r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", "body must be JSON with a fullName field")
		return
	}
	owner, name, err := store.ParseFullName(req.FullName)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_repository", err.Error())
		return
	}
	repo, err := a.store.AddRepo(r.Context(), owner, name)
	if storeErrors.write(w, err) {
		return
	}
	a.syncer.Kick()
	writeJSON(w, http.StatusCreated, repoFromStore(repo))
}

func (a *api) handleRemoveRepo(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	if storeErrors.write(w, a.store.RemoveRepoByID(r.Context(), id)) {
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *api) handleListPulls(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	opts := store.ListPRsOptions{State: store.PRStateFilter(q.Get("state"))}
	if repo := q.Get("repo"); repo != "" {
		owner, name, err := store.ParseFullName(repo)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid_repository", err.Error())
			return
		}
		opts.Owner, opts.Name = owner, name
	}
	prs, err := a.store.ListPRs(r.Context(), opts)
	if storeErrors.write(w, err) {
		return
	}
	out := PullRequestList{PullRequests: make([]PullRequest, 0, len(prs))}
	for _, pr := range prs {
		out.PullRequests = append(out.PullRequests, pullFromStore(pr))
	}
	writeJSON(w, http.StatusOK, out)
}

func (a *api) handleSync(w http.ResponseWriter, r *http.Request) {
	a.syncer.Sync()
	writeJSON(w, http.StatusAccepted, SyncAccepted{Accepted: true})
}

func (a *api) handleShutdown(w http.ResponseWriter, r *http.Request) {
	if r.Header.Get("Origin") != "" {
		writeError(w, http.StatusForbidden, "origin_forbidden", "browsers cannot stop the daemon")
		return
	}
	w.WriteHeader(http.StatusAccepted)
	a.shutdown()
}
