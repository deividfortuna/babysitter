package ghclient

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/go-github/v91/github"
)

func lowBudget(now time.Time) *github.Response {
	resp := &github.Response{}
	resp.Rate.Limit = 5000
	resp.Rate.Remaining = 3
	resp.Rate.Reset = github.Timestamp{Time: now.Add(time.Hour)}
	return resp
}

func TestAPausingGuardRecordsTheWaitInsteadOfTakingIt(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 9, 22, 10, 0, 0, 0, time.UTC)
	clock := func() time.Time { return now }
	g := (&RateGuard{Floor: 10}).Pausing(clock)

	took := time.Now()
	err := g.After(context.Background(), lowBudget(now), nil)

	if !errors.Is(err, ErrPaused) {
		t.Fatalf("After() error = %v, want %v", err, ErrPaused)
	}
	if waited := time.Since(took); waited > time.Second {
		t.Fatalf("the guard waited %s in place, and the caller holds a lock", waited)
	}
	if !g.Paused() {
		t.Fatal("the guard says the calls may go on, and the budget is spent")
	}
	now = now.Add(time.Hour + 2*time.Second)
	if g.Paused() {
		t.Fatal("the guard still pauses past the reset")
	}
}

func TestAPausingGuardKeepsTheFloorAndTheLogOfTheOneItComesFrom(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 9, 22, 10, 0, 0, 0, time.UTC)
	from := &RateGuard{Floor: 10}
	g := from.Pausing(func() time.Time { return now })

	if g.Floor != from.Floor {
		t.Fatalf("floor = %d, want %d", g.Floor, from.Floor)
	}
	resp := &github.Response{}
	resp.Rate.Limit = 5000
	resp.Rate.Remaining = 4000
	if err := g.After(context.Background(), resp, nil); err != nil {
		t.Fatalf("After() error = %v, want none", err)
	}
	if g.Paused() {
		t.Fatal("the guard paused on a budget above its floor")
	}
}

func TestAGuardThatIsNotThereNeverPauses(t *testing.T) {
	t.Parallel()
	var g *RateGuard
	if g.Pausing(time.Now) != nil {
		t.Fatal("Pausing() of no guard answered with one")
	}
	if g.Paused() {
		t.Fatal("Paused() of no guard = true")
	}
}
