package prwatch

import (
	"testing"

	"github.com/deividfortuna/babysitter/internal/store"
)

func TestNotificationCarriesTheKindAndTheWatch(t *testing.T) {
	t.Parallel()
	s := &Service{}
	w := store.Watch{ID: 7, Owner: "octo", Name: "hello", Number: 42, URL: "https://github.com/octo/hello/pull/42"}

	cases := []struct {
		kind store.ActivityKind
		want store.NotificationKind
	}{
		{store.ActivityReviewComment, store.NotificationReview},
		{store.ActivityComment, store.NotificationReview},
		{store.ActivityReview, store.NotificationReview},
		{store.ActivityCheckFailed, store.NotificationChecks},
		{store.ActivityChecksGreen, store.NotificationChecks},
		{store.ActivityMergeReady, store.NotificationMerge},
		{store.ActivityMergeFailed, store.NotificationMerge},
		{store.ActivityWatchStopped, store.NotificationWatch},
		{store.ActivitySessionExited, store.NotificationWatch},
		{store.ActivityAgentFailed, store.NotificationWatch},
	}
	for _, c := range cases {
		item, ok := s.notification(w, store.Activity{Kind: c.kind, Summary: "something happened"})
		if !ok {
			t.Errorf("activity %q sends no notification", c.kind)
			continue
		}
		if item.Kind != c.want {
			t.Errorf("activity %q has notification kind %q, want %q", c.kind, item.Kind, c.want)
		}
		if item.WatchID != w.ID || item.Repo != "octo/hello" || item.Number != 42 {
			t.Errorf("activity %q notification = %+v, want the watch it belongs to", c.kind, item)
		}
	}
}

func TestNotificationSkipsTheQuietActivity(t *testing.T) {
	t.Parallel()
	s := &Service{}
	w := store.Watch{ID: 7, Owner: "octo", Name: "hello", Number: 42}

	for _, kind := range []store.ActivityKind{store.ActivityHeartbeat, store.ActivityNudged, store.ActivityCommit, store.ActivityReplied} {
		if _, ok := s.notification(w, store.Activity{Kind: kind, Summary: "quiet"}); ok {
			t.Errorf("activity %q sends a notification, want none", kind)
		}
	}
}
