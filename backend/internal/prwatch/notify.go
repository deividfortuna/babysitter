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
	store.ActivityAutoStarted:   store.NotificationAuto,
	store.ActivityApprovalAsked: store.NotificationAuto,
}

func quiet(w store.Watch, a store.Activity) bool {
	return a.Kind == store.ActivityWatchStarted && w.AutoReason != store.AutoNone
}

func kindOf(a store.Activity) (store.NotificationKind, bool) {
	if a.Kind == store.ActivityMerged && a.Ref == autoMergedRef {
		return store.NotificationMerge, true
	}
	kind, ok := notificationKinds[a.Kind]
	return kind, ok
}

func (s *Service) notification(w store.Watch, a store.Activity) (notify.Item, bool) {
	kind, ok := kindOf(a)
	if !ok || quiet(w, a) {
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
	switch a.Kind {
	case store.ActivityWatchStarted:
		item.Message = fmt.Sprintf("Watching %s, checks every %s", w.HeadRef, s.Interval().Round(time.Second))
	case store.ActivityAutoStarted:
		item.Message = fmt.Sprintf("A watch started on its own: %s. It uses the settings of %s.", w.AutoReason.Word(), w.Repo())
	case store.ActivityApprovalAsked:
		item.Action = store.ActionApproveMerge
	default:
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
