package store

import (
	"context"
	"testing"
	"time"

	"github.com/deividfortuna/babysitter/internal/events"
)

func TestPublishNamesThePullRequestOfTheWatch(t *testing.T) {
	t.Parallel()
	s, _ := openTemp(t)
	pub := &recordingPublisher{}
	ctx := context.Background()
	w, err := s.CreateWatch(ctx, newWatch(time.Date(2026, 9, 7, 12, 0, 0, 0, time.UTC)))
	if err != nil {
		t.Fatal(err)
	}
	s.SetPublisher(pub)

	s.PublishActivity(w.Key())
	if got := pub.last(); got.Type != events.WatchActivity || got.Repo != w.Repo() || got.Number != w.Number {
		t.Fatalf("PublishActivity sent %+v, want the pull request of watch %d", got, w.ID)
	}
	s.PublishSession(w.Key())
	if got := pub.last(); got.Type != events.WatchSession || got.Repo != w.Repo() || got.Number != w.Number {
		t.Fatalf("PublishSession sent %+v, want the pull request of watch %d", got, w.ID)
	}
}

func TestPublishReadsNothingFromTheDatabase(t *testing.T) {
	t.Parallel()
	s, _ := openTemp(t)
	pub := &recordingPublisher{}
	ctx := context.Background()
	w, err := s.CreateWatch(ctx, newWatch(time.Date(2026, 9, 7, 12, 0, 0, 0, time.UTC)))
	if err != nil {
		t.Fatal(err)
	}
	s.SetPublisher(pub)
	s.Close()

	s.PublishActivity(w.Key())
	s.PublishSession(w.Key())

	if len(pub.events) != 2 {
		t.Fatalf("events = %+v, want the activity event and the session event", pub.events)
	}
	for _, got := range pub.events {
		if got.Repo != w.Repo() || got.Number != w.Number {
			t.Fatalf("event %+v does not name the pull request of watch %d", got, w.ID)
		}
	}
}

func TestPublishWithoutAPublisherIsQuiet(t *testing.T) {
	t.Parallel()
	s, _ := openTemp(t)
	ctx := context.Background()
	w, err := s.CreateWatch(ctx, newWatch(time.Date(2026, 9, 7, 12, 0, 0, 0, time.UTC)))
	if err != nil {
		t.Fatal(err)
	}
	s.PublishActivity(w.Key())
	s.PublishSession(w.Key())
}
