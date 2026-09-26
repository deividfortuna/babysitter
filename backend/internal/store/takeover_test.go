package store

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/deividfortuna/babysitter/internal/events"
)

func TestAWatchIsTakenOverAndHandedBack(t *testing.T) {
	t.Parallel()
	s, _ := openTemp(t)
	ctx := context.Background()
	w, err := s.CreateWatch(ctx, Watch{Owner: "octo", Name: "hello", Number: 3, StartedAt: time.Now()})
	if err != nil {
		t.Fatal(err)
	}
	if w.TakenOverAt != nil || w.TakenOverPID != 0 || w.HandbackStart != "" {
		t.Fatalf("created watch = %+v", w)
	}
	pub := &recordingPublisher{}
	s.SetPublisher(pub)

	at := time.Date(2026, 9, 26, 12, 0, 0, 0, time.UTC)
	w, err = s.SetWatchTakeover(ctx, w.ID, &at, 4242)
	if err != nil || w.TakenOverAt == nil || !w.TakenOverAt.Equal(at) || w.TakenOverPID != 4242 {
		t.Fatalf("SetWatchTakeover() = %+v, %v", w, err)
	}
	if e := pub.last(); e.Type != events.WatchChanged || e.Number != 3 {
		t.Fatalf("event = %+v", e)
	}

	w, err = s.SetWatchTakeover(ctx, w.ID, nil, 0)
	if err != nil || w.TakenOverAt != nil || w.TakenOverPID != 0 {
		t.Fatalf("SetWatchTakeover() to clear = %+v, %v", w, err)
	}
	if _, err := s.SetWatchTakeover(ctx, w.ID+100, &at, 1); !errors.Is(err, ErrWatchNotFound) {
		t.Fatalf("SetWatchTakeover() of no watch = %v, want ErrWatchNotFound", err)
	}
}

func TestAWatchKeepsTheStartOfTheTurnAfterAHandBack(t *testing.T) {
	t.Parallel()
	s, _ := openTemp(t)
	ctx := context.Background()
	w, err := s.CreateWatch(ctx, Watch{Owner: "octo", Name: "hello", Number: 3, StartedAt: time.Now()})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.SetWatchHandbackStart(ctx, w.ID, "abc"); err != nil {
		t.Fatal(err)
	}
	if w, err = s.GetWatch(ctx, w.ID); err != nil || w.HandbackStart != "abc" {
		t.Fatalf("HandbackStart = %q, %v", w.HandbackStart, err)
	}
	if err := s.SetWatchHandbackStart(ctx, w.ID, ""); err != nil {
		t.Fatal(err)
	}
	if w, err = s.GetWatch(ctx, w.ID); err != nil || w.HandbackStart != "" {
		t.Fatalf("HandbackStart after clear = %q, %v", w.HandbackStart, err)
	}
}

func TestTheTakeoverKindsAreNotActionable(t *testing.T) {
	t.Parallel()
	for _, k := range []ActivityKind{ActivityTakenOver, ActivityHandedBack} {
		if !k.Valid() {
			t.Fatalf("%s is not a valid kind", k)
		}
		if slices.Contains(ActionableKinds, k) {
			t.Fatalf("%s is actionable", k)
		}
	}
}
