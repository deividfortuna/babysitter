package prwatch

import (
	"cmp"
	"context"
	"fmt"
	"time"

	"github.com/deividfortuna/babysitter/internal/notify"
	"github.com/deividfortuna/babysitter/internal/redact"
	"github.com/deividfortuna/babysitter/internal/store"
)

var notificationKinds = map[store.ActivityKind]store.NotificationKind{
	store.ActivityComment:       store.NotificationReview,
	store.ActivityReviewComment: store.NotificationReview,
	store.ActivityReview:        store.NotificationReview,
	store.ActivityCheckFailed:   store.NotificationChecks,
	store.ActivityChecksGreen:   store.NotificationChecks,
	store.ActivityWatchStarted:  store.NotificationWatch,
	store.ActivityWatchStopped:  store.NotificationWatch,
	store.ActivitySessionExited: store.NotificationWatch,
	store.ActivityAgentFailed:   store.NotificationWatch,
	store.ActivityMergeReady:    store.NotificationMerge,
	store.ActivityMergeFailed:   store.NotificationMerge,
	store.ActivityProposal:      store.NotificationAgent,
}

func (s *Service) notification(w store.Watch, a store.Activity) (notify.Item, bool) {
	kind, ok := notificationKinds[a.Kind]
	if !ok {
		return notify.Item{}, false
	}
	if kind == store.NotificationReview && s.attended(w) {
		return notify.Item{}, false
	}
	item := notify.Item{
		Kind:     kind,
		WatchID:  w.ID,
		Repo:     w.Repo(),
		Number:   w.Number,
		Title:    fmt.Sprintf("PR #%d", w.Number),
		Subtitle: w.Repo(),
		URL:      cmp.Or(a.URL, w.URL),
		Message:  a.Summary,
	}
	if a.Kind == store.ActivityWatchStarted {
		item.Message = fmt.Sprintf("Watching %s, checks every %s", w.HeadRef, s.Interval().Round(time.Second))
	}
	item.Message = redact.Text(item.Message)
	return item, true
}

func (s *Service) notify(ctx context.Context, item notify.Item) {
	if s.notifications == nil {
		return
	}
	if _, err := s.notifications.Post(ctx, item); err != nil {
		s.log.Warn("notification failed", "title", item.Title, "err", err)
	}
}
