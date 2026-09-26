package store

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/deividfortuna/babysitter/internal/events"
)

func TestAddNotificationReturnsTheStoredRow(t *testing.T) {
	t.Parallel()
	s, _ := openTemp(t)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Second)

	n, err := s.AddNotification(ctx, Notification{
		Kind:      NotificationReview,
		Repo:      "octo/hello",
		Number:    42,
		Title:     "PR #42",
		Body:      "alice left a comment",
		URL:       "https://github.com/octo/hello/pull/42",
		CreatedAt: now,
	})
	if err != nil {
		t.Fatalf("AddNotification() error = %v", err)
	}
	if n.ID == 0 {
		t.Error("AddNotification() left the id at zero")
	}
	if n.ReadAt != nil {
		t.Errorf("AddNotification() marked the row read at %v", n.ReadAt)
	}

	rows, err := s.ListNotifications(ctx, ListNotificationsOptions{})
	if err != nil {
		t.Fatalf("ListNotifications() error = %v", err)
	}
	if len(rows) != 1 || rows[0].ID != n.ID {
		t.Fatalf("ListNotifications() = %+v, want the row AddNotification returned", rows)
	}
	if rows[0].Body != "alice left a comment" || !rows[0].CreatedAt.Equal(now) {
		t.Errorf("ListNotifications() row = %+v, want the values that went in", rows[0])
	}
}

func TestAddNotificationKeepsTheSilentFlag(t *testing.T) {
	t.Parallel()
	s, _ := openTemp(t)
	ctx := context.Background()

	silent := add(t, s, Notification{Kind: NotificationAgent, Title: "PR #42", Body: "quietly", Silent: true})
	loud := add(t, s, Notification{Kind: NotificationAgent, Title: "PR #42", Body: "out loud"})

	if !silent.Silent {
		t.Error("AddNotification() dropped the silent flag of the row it returned")
	}
	if loud.Silent {
		t.Error("AddNotification() made a row silent that asked for a sound")
	}
	rows, err := s.ListNotifications(ctx, ListNotificationsOptions{})
	if err != nil {
		t.Fatalf("ListNotifications() error = %v", err)
	}
	if len(rows) != 2 || rows[0].Silent || !rows[1].Silent {
		t.Errorf("ListNotifications() = %+v, want the silent flag of each row", rows)
	}
}

func TestAddNotificationRejectsAnUnknownKind(t *testing.T) {
	t.Parallel()
	s, _ := openTemp(t)

	_, err := s.AddNotification(context.Background(), Notification{Kind: "gossip", Title: "hi", Body: "there"})
	if err == nil {
		t.Fatal("AddNotification() took an unknown kind")
	}
}

func TestJoinKindsNamesEveryKindInOrder(t *testing.T) {
	t.Parallel()

	if got := JoinKinds(); got != "agent, review, checks, watch, merge" {
		t.Errorf("JoinKinds() = %q, want every kind in the order of the schema", got)
	}
}

func TestListNotificationsNewestFirstAndUnreadOnly(t *testing.T) {
	t.Parallel()
	s, _ := openTemp(t)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Second)

	first := add(t, s, Notification{Kind: NotificationWatch, Title: "first", Body: "one", CreatedAt: now})
	second := add(t, s, Notification{Kind: NotificationWatch, Title: "second", Body: "two", CreatedAt: now.Add(time.Second)})

	rows, err := s.ListNotifications(ctx, ListNotificationsOptions{})
	if err != nil {
		t.Fatalf("ListNotifications() error = %v", err)
	}
	if len(rows) != 2 || rows[0].ID != second.ID || rows[1].ID != first.ID {
		t.Fatalf("ListNotifications() = %+v, want the newest row first", rows)
	}

	if err := s.MarkNotificationsRead(ctx, []int64{first.ID}, now); err != nil {
		t.Fatalf("MarkNotificationsRead() error = %v", err)
	}
	unread, err := s.ListNotifications(ctx, ListNotificationsOptions{UnreadOnly: true})
	if err != nil {
		t.Fatalf("ListNotifications() error = %v", err)
	}
	if len(unread) != 1 || unread[0].ID != second.ID {
		t.Fatalf("ListNotifications(unread) = %+v, want only the row that stayed unread", unread)
	}
}

func TestMarkNotificationsReadWithNoIDsMarksThemAll(t *testing.T) {
	t.Parallel()
	s, _ := openTemp(t)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Second)

	add(t, s, Notification{Kind: NotificationChecks, Title: "one", Body: "one", CreatedAt: now})
	add(t, s, Notification{Kind: NotificationChecks, Title: "two", Body: "two", CreatedAt: now})

	if err := s.MarkNotificationsRead(ctx, nil, now); err != nil {
		t.Fatalf("MarkNotificationsRead() error = %v", err)
	}
	count, err := s.UnreadNotifications(ctx)
	if err != nil {
		t.Fatalf("UnreadNotifications() error = %v", err)
	}
	if count != 0 {
		t.Errorf("UnreadNotifications() = %d, want 0", count)
	}
}

func TestUnreadNotificationsCountsOnlyTheUnreadRows(t *testing.T) {
	t.Parallel()
	s, _ := openTemp(t)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Second)

	read := add(t, s, Notification{Kind: NotificationAgent, Title: "one", Body: "one", CreatedAt: now})
	add(t, s, Notification{Kind: NotificationAgent, Title: "two", Body: "two", CreatedAt: now})
	if err := s.MarkNotificationsRead(ctx, []int64{read.ID}, now); err != nil {
		t.Fatalf("MarkNotificationsRead() error = %v", err)
	}

	count, err := s.UnreadNotifications(ctx)
	if err != nil {
		t.Fatalf("UnreadNotifications() error = %v", err)
	}
	if count != 1 {
		t.Errorf("UnreadNotifications() = %d, want 1", count)
	}
}

func TestListNotificationsTakesTheLimit(t *testing.T) {
	t.Parallel()
	s, _ := openTemp(t)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Second)
	for i := range 5 {
		add(t, s, Notification{Kind: NotificationMerge, Title: "row", Body: "body", CreatedAt: now.Add(time.Duration(i) * time.Second)})
	}

	rows, err := s.ListNotifications(ctx, ListNotificationsOptions{Limit: 2})
	if err != nil {
		t.Fatalf("ListNotifications() error = %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("ListNotifications(limit 2) returned %d rows", len(rows))
	}
}

func TestNotificationsFollowTheWatchThatIsRemoved(t *testing.T) {
	t.Parallel()
	s, _ := openTemp(t)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Second)

	w, err := s.CreateWatch(ctx, Watch{Owner: "octo", Name: "hello", Number: 7, StartedAt: now})
	if err != nil {
		t.Fatal(err)
	}
	add(t, s, Notification{WatchID: w.ID, Kind: NotificationWatch, Title: "watch", Body: "started", CreatedAt: now})

	if err := s.DeleteWatch(ctx, w.ID); err != nil {
		t.Fatalf("DeleteWatch() error = %v", err)
	}
	rows, err := s.ListNotifications(ctx, ListNotificationsOptions{})
	if err != nil {
		t.Fatalf("ListNotifications() error = %v", err)
	}
	if len(rows) != 0 {
		t.Errorf("ListNotifications() = %+v, want none after the watch went", rows)
	}
}

func add(t *testing.T, s *Store, n Notification) Notification {
	t.Helper()
	row, err := s.AddNotification(context.Background(), n)
	if err != nil {
		t.Fatalf("AddNotification() error = %v", err)
	}
	return row
}

func TestAddNotificationKeepsTheNewestRowsAndDropsTheRest(t *testing.T) {
	t.Parallel()
	s, _ := openTemp(t)
	ctx := context.Background()
	s.notificationCap = 3

	for i := range 5 {
		if _, err := s.AddNotification(ctx, Notification{
			Kind: NotificationAgent, Title: "PR #42", Body: fmt.Sprintf("message %d", i),
		}); err != nil {
			t.Fatalf("AddNotification() error = %v", err)
		}
	}

	rows, err := s.ListNotifications(ctx, ListNotificationsOptions{})
	if err != nil {
		t.Fatalf("ListNotifications() error = %v", err)
	}
	if len(rows) != 3 {
		t.Fatalf("the history holds %d rows, want the cap of %d", len(rows), s.notificationCap)
	}
	if rows[0].Body != "message 4" || rows[2].Body != "message 2" {
		t.Errorf("the history holds %+v, want the newest three", rows)
	}
	unread, err := s.UnreadNotifications(ctx)
	if err != nil {
		t.Fatalf("UnreadNotifications() error = %v", err)
	}
	if unread != 3 {
		t.Errorf("unread = %d, want only the rows the history kept", unread)
	}
}

func TestAddNotificationKeepsTheRowWhenTheTrimFails(t *testing.T) {
	t.Parallel()
	s, _ := openTemp(t)
	ctx := context.Background()
	s.notificationCap = 1
	if _, err := s.db.ExecContext(ctx, `
CREATE TRIGGER notifications_keep_every_row BEFORE DELETE ON notifications
BEGIN SELECT RAISE(ABORT, 'the trim cannot delete'); END`); err != nil {
		t.Fatalf("create trigger: %v", err)
	}
	add(t, s, Notification{Kind: NotificationAgent, Title: "PR #42", Body: "message 0"})
	pub := &recordingPublisher{}
	s.SetPublisher(pub)

	row, err := s.AddNotification(ctx, Notification{Kind: NotificationAgent, Title: "PR #42", Body: "message 1"})
	if err != nil {
		t.Fatalf("AddNotification() error = %v, want the row the trim could not make room for", err)
	}
	if row.ID == 0 {
		t.Error("AddNotification() left the id at zero")
	}
	if got := pub.last(); got.Type != events.NotificationAdded {
		t.Errorf("published %+v, want %s for a row that was recorded", got, events.NotificationAdded)
	}
	rows, err := s.ListNotifications(ctx, ListNotificationsOptions{})
	if err != nil {
		t.Fatalf("ListNotifications() error = %v", err)
	}
	if len(rows) != 2 {
		t.Errorf("the history holds %d rows, want both: the trim deleted nothing", len(rows))
	}
}

func TestAddNotificationOfAWatchThatDoesNotExistSaysSo(t *testing.T) {
	t.Parallel()
	s, _ := openTemp(t)

	_, err := s.AddNotification(context.Background(), Notification{
		WatchID: 999, Kind: NotificationAgent, Title: "PR #42", Body: "alice needs an answer",
	})

	if !errors.Is(err, ErrWatchNotFound) {
		t.Fatalf("AddNotification() error = %v, want ErrWatchNotFound", err)
	}
}
