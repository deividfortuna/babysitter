package httpd

import (
	"net/http"
	"time"

	"github.com/deividfortuna/babysitter/internal/prwatch"
	"github.com/deividfortuna/babysitter/internal/store"
)

func (a *api) handleGetSettings(w http.ResponseWriter, r *http.Request) {
	s, err := a.store.Settings(r.Context())
	if storeErrors.write(w, err) {
		return
	}
	writeJSON(w, http.StatusOK, settingsOut(s))
}

func (a *api) handlePutSettings(w http.ResponseWriter, r *http.Request) {
	current, err := a.store.Settings(r.Context())
	if storeErrors.write(w, err) {
		return
	}
	req := settingsOut(current)
	if err := readJSON(w, r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", "body must be the settings to change as JSON")
		return
	}
	model, effort, err := prwatch.NormalizeHostedAgent(req.Provider, req.Model, req.Effort)
	if err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", err.Error())
		return
	}
	saved, err := a.store.SaveSettings(r.Context(), store.Settings{
		PollInterval:      time.Duration(req.PollIntervalSeconds) * time.Second,
		WatchInterval:     time.Duration(req.WatchIntervalSeconds) * time.Second,
		WatchMaxInterval:  time.Duration(req.WatchMaxIntervalSeconds) * time.Second,
		CheckMaxInterval:  time.Duration(req.CheckMaxIntervalSeconds) * time.Second,
		ApprovalsRequired: req.ApprovalsRequired,
		MergeMethod:       req.MergeMethod,
		IncludeExisting:   req.IncludeExisting,
		IncludeOwn:        req.IncludeOwn,
		KeepWorktree:      req.KeepWorktree,

		NotificationsEnabled:   req.NotificationsEnabled,
		NotificationSound:      req.NotificationSound,
		MutedNotificationKinds: mutedKindsIn(req.MutedNotificationKinds),
		ApprovalMode:           store.ApprovalMode(req.ApprovalMode),
		AutoApproveRebase:      req.AutoApproveRebase,
		Provider:               req.Provider,
		Model:                  model,
		Effort:                 effort,
	})
	if storeErrors.write(w, err) {
		return
	}
	if a.applySettings != nil {
		a.applySettings(saved)
	}
	writeJSON(w, http.StatusOK, settingsOut(saved))
}

func settingsOut(s store.Settings) Settings {
	return Settings{
		PollIntervalSeconds:     int(s.PollInterval.Seconds()),
		WatchIntervalSeconds:    int(s.WatchInterval.Seconds()),
		WatchMaxIntervalSeconds: int(s.WatchMaxInterval.Seconds()),
		CheckMaxIntervalSeconds: int(s.CheckMaxInterval.Seconds()),
		ApprovalsRequired:       s.ApprovalsRequired,
		MergeMethod:             s.MergeMethod,
		IncludeExisting:         s.IncludeExisting,
		IncludeOwn:              s.IncludeOwn,
		KeepWorktree:            s.KeepWorktree,
		NotificationsEnabled:    s.NotificationsEnabled,
		NotificationSound:       s.NotificationSound,
		MutedNotificationKinds:  mutedKindsOut(s.MutedNotificationKinds),
		ApprovalMode:            string(s.ApprovalMode),
		AutoApproveRebase:       s.AutoApproveRebase,
		Provider:                s.Provider,
		Model:                   s.Model,
		Effort:                  s.Effort,
	}
}

func mutedKindsIn(kinds []string) []store.NotificationKind {
	out := make([]store.NotificationKind, 0, len(kinds))
	for _, kind := range kinds {
		out = append(out, store.NotificationKind(kind))
	}
	return out
}

func mutedKindsOut(kinds []store.NotificationKind) []string {
	out := make([]string, 0, len(kinds))
	for _, kind := range kinds {
		out = append(out, string(kind))
	}
	return out
}
