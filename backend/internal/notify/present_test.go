package notify

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/deividfortuna/babysitter/internal/store"
	"github.com/deividfortuna/babysitter/internal/testutil"
)

type unreadableRows struct{ *centerStore }

func (unreadableRows) ListNotifications(context.Context, store.ListNotificationsOptions) ([]store.Notification, error) {
	return nil, errors.New("database is locked")
}

type heldMark struct {
	*centerStore
	entered chan struct{}
	release chan struct{}
}

func (h heldMark) ListNotifications(ctx context.Context, o store.ListNotificationsOptions) ([]store.Notification, error) {
	rows, err := h.centerStore.ListNotifications(ctx, o)
	close(h.entered)
	<-h.release
	return rows, err
}

type heldAdd struct {
	*centerStore
	entered chan struct{}
	release chan struct{}
}

func (h heldAdd) AddNotification(ctx context.Context, n store.Notification) (store.Notification, error) {
	row, err := h.centerStore.AddNotification(ctx, n)
	close(h.entered)
	<-h.release
	return row, err
}

func present(t *testing.T, c *Center) (release func()) {
	t.Helper()
	_, release, err := c.Present(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	return release
}

type claim struct {
	mark    int64
	release func()
	err     error
}

func TestPresentTakesTheClaimAndTheMarkTogether(t *testing.T) {
	st := heldMark{centerStore: newCenterStore(), entered: make(chan struct{}), release: make(chan struct{})}
	desktop := &spyDesktop{}
	c := NewCenter(CenterDeps{Log: testutil.Logger(t), Store: st, Desktop: desktop})

	claims := make(chan claim, 1)
	go func() {
		mark, release, err := c.Present(context.Background())
		claims <- claim{mark: mark, release: release, err: err}
	}()
	<-st.entered

	posted := make(chan store.Notification, 1)
	go func() {
		row, err := c.Post(context.Background(), item())
		if err != nil {
			t.Error(err)
		}
		posted <- row
	}()
	time.Sleep(50 * time.Millisecond)
	close(st.release)

	taken := <-claims
	if taken.err != nil {
		t.Fatal(taken.err)
	}
	defer taken.release()
	row := <-posted
	c.Wait()

	drawnByTheDaemon := len(desktop.sent) > 0
	leftToTheClient := row.ID > taken.mark
	if drawnByTheDaemon == leftToTheClient {
		t.Fatalf("notification %d against the mark %d: the daemon drew it %v and the client draws it %v, want one of the two",
			row.ID, taken.mark, drawnByTheDaemon, leftToTheClient)
	}
}

func TestReleaseGivesThePostsInsideTheHandoverToTheDaemon(t *testing.T) {
	t.Parallel()
	st := heldAdd{centerStore: newCenterStore(), entered: make(chan struct{}), release: make(chan struct{})}
	desktop := &spyDesktop{}
	c := NewCenter(CenterDeps{Log: testutil.Logger(t), Store: st, Desktop: desktop})
	release := present(t, c)

	posted := make(chan struct{})
	go func() {
		defer close(posted)
		if _, err := c.Post(context.Background(), item()); err != nil {
			t.Error(err)
		}
	}()
	<-st.entered

	release()
	close(st.release)
	<-posted
	c.Wait()

	if len(desktop.sent) != 1 {
		t.Fatalf("the daemon drew %d notifications of the release window, want 1", len(desktop.sent))
	}
}

func TestPresentLeavesTheShowingWhenTheMarkIsUnreadable(t *testing.T) {
	t.Parallel()
	st := unreadableRows{centerStore: newCenterStore()}
	c := NewCenter(CenterDeps{Log: testutil.Logger(t), Store: st, Desktop: &spyDesktop{}})

	if _, _, err := c.Present(context.Background()); err == nil {
		t.Fatal("Present took the claim with the mark unreadable")
	}
	if c.Presenting() {
		t.Fatal("the daemon gave the showing away with the mark unreadable")
	}
}
