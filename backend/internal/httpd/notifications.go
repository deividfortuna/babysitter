package httpd

import (
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/deividfortuna/babysitter/internal/notify"
	"github.com/deividfortuna/babysitter/internal/store"
)

const maxNotificationsLimit = 1000

func (a *api) handleListNotifications(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	status := q.Get("status")
	if status != "" && status != "all" && status != "unread" {
		writeError(w, http.StatusBadRequest, "invalid_status", "status must be all or unread")
		return
	}
	opts := store.ListNotificationsOptions{UnreadOnly: status == "unread"}
	if raw := q.Get("limit"); raw != "" {
		limit, err := strconv.Atoi(raw)
		if err != nil || limit < 1 || limit > maxNotificationsLimit {
			writeError(w, http.StatusBadRequest, "invalid_limit", fmt.Sprintf("limit must be a whole number between 1 and %d", maxNotificationsLimit))
			return
		}
		opts.Limit = limit
	}
	rows, err := a.store.ListNotifications(r.Context(), opts)
	if storeErrors.write(w, err) {
		return
	}
	unread, err := a.store.UnreadNotifications(r.Context())
	if storeErrors.write(w, err) {
		return
	}
	out := NotificationList{Notifications: make([]Notification, 0, len(rows)), UnreadCount: unread}
	for _, n := range rows {
		out.Notifications = append(out.Notifications, notificationFromStore(n))
	}
	writeJSON(w, http.StatusOK, out)
}

func (a *api) handleReadNotifications(w http.ResponseWriter, r *http.Request) {
	var req ReadNotificationsRequest
	if err := readJSON(w, r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", "body must be JSON with an optional ids field")
		return
	}
	if storeErrors.write(w, a.store.MarkNotificationsRead(r.Context(), req.IDs, time.Now().UTC())) {
		return
	}
	unread, err := a.store.UnreadNotifications(r.Context())
	if storeErrors.write(w, err) {
		return
	}
	writeJSON(w, http.StatusOK, NotificationsRead{UnreadCount: unread})
}

func (a *api) handleAddNotification(w http.ResponseWriter, r *http.Request) {
	if a.notifications == nil {
		writeError(w, http.StatusServiceUnavailable, "notifications_unavailable", "this daemon records no notifications")
		return
	}
	var req NewNotificationRequest
	if err := readJSON(w, r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", "body must be JSON with a title and a body")
		return
	}
	kind := store.NotificationKind(req.Kind)
	if req.Kind == "" {
		kind = store.NotificationAgent
	}
	row, err := a.notifications.Post(r.Context(), notify.Item{
		Kind:     kind,
		WatchID:  req.WatchID,
		Repo:     req.Repo,
		Number:   req.Number,
		Title:    req.Title,
		Subtitle: req.Subtitle,
		Message:  req.Body,
		URL:      req.URL,
		Silent:   req.Silent,
	})
	if storeErrors.write(w, err) {
		return
	}
	writeJSON(w, http.StatusCreated, notificationFromStore(row))
}
