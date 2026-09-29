package notify

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/deividfortuna/babysitter/internal/store"
	"github.com/deividfortuna/babysitter/internal/testutil"
)

type centerStore struct {
	mu          sync.Mutex
	rows        []store.Notification
	settings    store.Settings
	settingsErr error
	addErr      error
}

func newCenterStore() *centerStore {
	return &centerStore{settings: store.DefaultSettings()}
}

func (c *centerStore) AddNotification(_ context.Context, n store.Notification) (store.Notification, error) {
	if c.addErr != nil {
		return store.Notification{}, c.addErr
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	n.ID = int64(len(c.rows) + 1)
	c.rows = append(c.rows, n)
	return n, nil
}

func (c *centerStore) ListNotifications(_ context.Context, o store.ListNotificationsOptions) ([]store.Notification, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	newest := make([]store.Notification, 0, len(c.rows))
	for i := len(c.rows) - 1; i >= 0 && len(newest) != o.Limit; i-- {
		newest = append(newest, c.rows[i])
	}
	return newest, nil
}

func (c *centerStore) Settings(context.Context) (store.Settings, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.settingsErr != nil {
		return store.Settings{}, c.settingsErr
	}
	return c.settings, nil
}

type spyDesktop struct {
	sent []Notification
	err  error
}

func (s *spyDesktop) Send(_ context.Context, n Notification) (Result, error) {
	s.sent = append(s.sent, n)
	if s.err != nil {
		return Result{}, s.err
	}
	return Result{Backend: "spy", Silent: n.Silent}, nil
}

func item() Item {
	return Item{
		Kind:    store.NotificationReview,
		Repo:    "octo/hello",
		Number:  42,
		Title:   "PR #42",
		Message: "alice left a comment",
		URL:     "https://github.com/octo/hello/pull/42",
	}
}

func TestPostRecordsTheRowAndShowsIt(t *testing.T) {
	t.Parallel()
	st, desk := newCenterStore(), &spyDesktop{}
	c := NewCenter(CenterDeps{Log: testutil.Logger(t), Store: st, Desktop: desk})

	row, err := c.Post(context.Background(), item())
	if err != nil {
		t.Fatalf("Post() error = %v", err)
	}
	if row.ID == 0 || row.Kind != store.NotificationReview || row.Number != 42 {
		t.Fatalf("Post() row = %+v, want the stored review notification", row)
	}
	if row.Body != "alice left a comment" {
		t.Errorf("Post() body = %q, want the message of the notification", row.Body)
	}
	c.Wait()
	if len(desk.sent) != 1 {
		t.Fatalf("the desktop got %d notifications, want 1", len(desk.sent))
	}
}

func TestPostStaysQuietWhileAClientPresents(t *testing.T) {
	t.Parallel()
	st, desk := newCenterStore(), &spyDesktop{}
	c := NewCenter(CenterDeps{Log: testutil.Logger(t), Store: st, Desktop: desk})

	release := present(t, c)
	if _, err := c.Post(context.Background(), item()); err != nil {
		t.Fatalf("Post() error = %v", err)
	}
	c.Wait()
	if len(desk.sent) != 0 {
		t.Fatalf("the desktop got %d notifications while the app presents, want 0", len(desk.sent))
	}
	if len(st.rows) != 1 {
		t.Fatalf("the store holds %d rows, want the notification recorded anyway", len(st.rows))
	}

	release()
	if _, err := c.Post(context.Background(), item()); err != nil {
		t.Fatalf("Post() error = %v", err)
	}
	c.Wait()
	if len(desk.sent) != 1 {
		t.Fatalf("the desktop got %d notifications after the app left, want 1", len(desk.sent))
	}
}

func TestPostKeepsTheDesktopQuietWhileTwoClientsPresent(t *testing.T) {
	t.Parallel()
	st, desk := newCenterStore(), &spyDesktop{}
	c := NewCenter(CenterDeps{Log: testutil.Logger(t), Store: st, Desktop: desk})

	first, second := present(t, c), present(t, c)
	first()
	if _, err := c.Post(context.Background(), item()); err != nil {
		t.Fatalf("Post() error = %v", err)
	}
	c.Wait()
	if len(desk.sent) != 0 {
		t.Fatalf("the desktop got %d notifications while one client still presents, want 0", len(desk.sent))
	}
	second()
	second()
	if _, err := c.Post(context.Background(), item()); err != nil {
		t.Fatalf("Post() error = %v", err)
	}
	c.Wait()
	if len(desk.sent) != 1 {
		t.Fatalf("the desktop got %d notifications after both left, want 1", len(desk.sent))
	}
}

func TestPostRecordsTheRowWithTheNotificationsOff(t *testing.T) {
	t.Parallel()
	st, desk := newCenterStore(), &spyDesktop{}
	st.settings.NotificationsEnabled = false
	c := NewCenter(CenterDeps{Log: testutil.Logger(t), Store: st, Desktop: desk})

	if _, err := c.Post(context.Background(), item()); err != nil {
		t.Fatalf("Post() error = %v", err)
	}
	c.Wait()
	if len(desk.sent) != 0 {
		t.Fatalf("the desktop got %d notifications with the setting off, want 0", len(desk.sent))
	}
	if len(st.rows) != 1 {
		t.Fatalf("the store holds %d rows, want the notification in the history anyway", len(st.rows))
	}
}

func TestPostSilencesTheNotificationOfASilentKind(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name   string
		silent []store.NotificationKind
		want   bool
	}{
		{"the kind is silent", []store.NotificationKind{store.NotificationReview}, true},
		{"another kind is silent", []store.NotificationKind{store.NotificationChecks}, false},
		{"no kind is silent", nil, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			st, desk := newCenterStore(), &spyDesktop{}
			st.settings.SilentNotificationKinds = c.silent
			center := NewCenter(CenterDeps{Log: testutil.Logger(t), Store: st, Desktop: desk})

			if _, err := center.Post(context.Background(), item()); err != nil {
				t.Fatalf("Post() error = %v", err)
			}
			center.Wait()
			if len(desk.sent) != 1 || desk.sent[0].Silent != c.want {
				t.Fatalf("the desktop got %+v, want one notification with silent %t", desk.sent, c.want)
			}
			if st.rows[0].Silent {
				t.Fatalf("the store holds %+v, want the row without the silent flag of the settings", st.rows[0])
			}
		})
	}
}

func TestPostCarriesTheSilentFlagOnTheRow(t *testing.T) {
	t.Parallel()
	st := newCenterStore()
	c := NewCenter(CenterDeps{Log: testutil.Logger(t), Store: st})
	quiet := item()
	quiet.Silent = true

	row, err := c.Post(context.Background(), quiet)
	if err != nil {
		t.Fatalf("Post() error = %v", err)
	}
	if !row.Silent {
		t.Errorf("Post() row = %+v, want the silent flag of the notification", row)
	}
	if len(st.rows) != 1 || !st.rows[0].Silent {
		t.Errorf("the store holds %+v, want the silent flag on the row", st.rows)
	}
}

func TestPostReportsTheRowEvenWhenTheDesktopFails(t *testing.T) {
	t.Parallel()
	st, desk := newCenterStore(), &spyDesktop{err: errors.New("no tool")}
	c := NewCenter(CenterDeps{Log: testutil.Logger(t), Store: st, Desktop: desk})

	row, err := c.Post(context.Background(), item())
	if err != nil {
		t.Fatalf("Post() error = %v, want the failure of the desktop swallowed", err)
	}
	if row.ID == 0 {
		t.Error("Post() returned no row")
	}
}

func TestPostFailsWhenTheStoreFails(t *testing.T) {
	t.Parallel()
	st, desk := newCenterStore(), &spyDesktop{}
	st.addErr = errors.New("database is locked")
	c := NewCenter(CenterDeps{Log: testutil.Logger(t), Store: st, Desktop: desk})

	if _, err := c.Post(context.Background(), item()); err == nil {
		t.Fatal("Post() hid the failure of the store")
	}
	c.Wait()
	if len(desk.sent) != 0 {
		t.Errorf("the desktop got %d notifications for a row that was never stored, want 0", len(desk.sent))
	}
}

func TestPostRejectsAnItemTheStoreWouldRefuse(t *testing.T) {
	t.Parallel()
	c := NewCenter(CenterDeps{Log: testutil.Logger(t), Store: newCenterStore(), Desktop: &spyDesktop{}})

	bad := item()
	bad.Message = "   "
	if _, err := c.Post(context.Background(), bad); err == nil {
		t.Fatal("Post() took a notification with no message")
	}
}

func TestPostRecordsTheRowOfAMutedKindAndShowsNothing(t *testing.T) {
	t.Parallel()
	st, desk := newCenterStore(), &spyDesktop{}
	st.settings.MutedNotificationKinds = []store.NotificationKind{store.NotificationReview}
	c := NewCenter(CenterDeps{Log: testutil.Logger(t), Store: st, Desktop: desk})

	if _, err := c.Post(context.Background(), item()); err != nil {
		t.Fatalf("Post() error = %v", err)
	}
	c.Wait()
	if len(desk.sent) != 0 {
		t.Fatalf("the desktop got %d notifications of a muted kind, want 0", len(desk.sent))
	}
	if len(st.rows) != 1 {
		t.Fatalf("the store holds %d rows, want the notification in the history anyway", len(st.rows))
	}
}

func TestPostShowsAKindNobodyMuted(t *testing.T) {
	t.Parallel()
	st, desk := newCenterStore(), &spyDesktop{}
	st.settings.MutedNotificationKinds = []store.NotificationKind{store.NotificationChecks}
	c := NewCenter(CenterDeps{Log: testutil.Logger(t), Store: st, Desktop: desk})

	if _, err := c.Post(context.Background(), item()); err != nil {
		t.Fatalf("Post() error = %v", err)
	}
	c.Wait()
	if len(desk.sent) != 1 {
		t.Fatalf("the desktop got %d notifications, want the review that nobody muted", len(desk.sent))
	}
}

func TestPostLeavesTheRepositoryOutOfTheBodyOfAWatchRow(t *testing.T) {
	t.Parallel()
	st, desk := newCenterStore(), &spyDesktop{}
	c := NewCenter(CenterDeps{Log: testutil.Logger(t), Store: st, Desktop: desk})

	withRepo := item()
	withRepo.Subtitle = withRepo.Repo
	row, err := c.Post(context.Background(), withRepo)
	if err != nil {
		t.Fatalf("Post() error = %v", err)
	}
	if row.Body != "alice left a comment" {
		t.Errorf("Post() body = %q, want the message alone: the row holds the repository already", row.Body)
	}
	c.Wait()
	if len(desk.sent) != 1 || desk.sent[0].Subtitle != "octo/hello" {
		t.Errorf("the desktop got %+v, want the repository as the second line of the banner", desk.sent)
	}
}

func TestPostKeepsTheSubtitleInTheBodyOfARowOfNoRepository(t *testing.T) {
	t.Parallel()
	st, desk := newCenterStore(), &spyDesktop{}
	c := NewCenter(CenterDeps{Log: testutil.Logger(t), Store: st, Desktop: desk})

	loose := Item{
		Kind:  store.NotificationAgent,
		Title: "PR #42", Subtitle: "octo/hello", Message: "Reply to alice",
	}
	row, err := c.Post(context.Background(), loose)
	if err != nil {
		t.Fatalf("Post() error = %v", err)
	}
	if row.Body != "octo/hello: Reply to alice" {
		t.Errorf("Post() body = %q, want the whole sentence", row.Body)
	}
	c.Wait()
	if len(desk.sent) != 1 || desk.sent[0].Subtitle != "octo/hello" {
		t.Errorf("the desktop got %+v, want the subtitle on the banner", desk.sent)
	}
}

type slowDesktop struct {
	release chan struct{}
	ctxErr  chan error
}

func (d *slowDesktop) Send(ctx context.Context, _ Notification) (Result, error) {
	<-d.release
	d.ctxErr <- ctx.Err()
	return Result{Backend: "slow"}, nil
}

func TestPostAnswersBeforeTheDesktopDrawsTheBanner(t *testing.T) {
	t.Parallel()
	desk := &slowDesktop{release: make(chan struct{}), ctxErr: make(chan error, 1)}
	release := sync.OnceFunc(func() { close(desk.release) })
	defer release()
	c := NewCenter(CenterDeps{Log: testutil.Logger(t), Store: newCenterStore(), Desktop: desk})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	posted := make(chan error, 1)
	go func() {
		_, err := c.Post(ctx, item())
		posted <- err
	}()
	select {
	case err := <-posted:
		if err != nil {
			t.Fatalf("Post() error = %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Post() waited for the desktop, so the caller waits for the operating system")
	}

	cancel()
	release()
	if err := <-desk.ctxErr; err != nil {
		t.Fatalf("the desktop got a context that ended with the request: %v", err)
	}
	c.Wait()
}

func TestPostRejectsAWhitespaceMessageBehindASubtitle(t *testing.T) {
	t.Parallel()
	st := newCenterStore()
	c := NewCenter(CenterDeps{Log: testutil.Logger(t), Store: st, Desktop: &spyDesktop{}})

	loose := Item{
		Kind:  store.NotificationAgent,
		Title: "PR #42", Subtitle: "octo/hello", Message: "   ",
	}
	if _, err := c.Post(context.Background(), loose); !errors.Is(err, store.ErrInvalidNotification) {
		t.Fatalf("Post() error = %v, want one that wraps ErrInvalidNotification", err)
	}
	if len(st.rows) != 0 {
		t.Fatalf("the store holds %d rows, want none for a notification with no message", len(st.rows))
	}
}

func TestPostShowsNothingWhenTheSettingsAreUnreadable(t *testing.T) {
	t.Parallel()
	st, desk := newCenterStore(), &spyDesktop{}
	st.settingsErr = errors.New("database is locked")
	c := NewCenter(CenterDeps{Log: testutil.Logger(t), Store: st, Desktop: desk})

	if _, err := c.Post(context.Background(), item()); err != nil {
		t.Fatalf("Post() error = %v, want the row recorded anyway", err)
	}
	c.Wait()
	if len(desk.sent) != 0 {
		t.Fatalf("the desktop got %d notifications with the settings unreadable, want 0", len(desk.sent))
	}
	if len(st.rows) != 1 {
		t.Fatalf("the store holds %d rows, want the notification in the history", len(st.rows))
	}
}

type blockedDesktop struct {
	ctxErr chan error
	values chan any
	key    any
}

func (d *blockedDesktop) Send(ctx context.Context, _ Notification) (Result, error) {
	d.values <- ctx.Value(d.key)
	<-ctx.Done()
	d.ctxErr <- ctx.Err()
	return Result{}, ctx.Err()
}

type requestKey struct{}

func TestShowGivesTheDesktopABoundedContextOfItsOwn(t *testing.T) {
	t.Parallel()
	desk := &blockedDesktop{ctxErr: make(chan error, 1), values: make(chan any, 1), key: requestKey{}}
	c := NewCenter(CenterDeps{Log: testutil.Logger(t), Store: newCenterStore(), Desktop: desk, ShowTimeout: 50 * time.Millisecond})
	ctx := context.WithValue(context.Background(), requestKey{}, "chi route context")

	if _, err := c.Post(ctx, item()); err != nil {
		t.Fatalf("Post() error = %v", err)
	}
	if got := <-desk.values; got != nil {
		t.Fatalf("the desktop got the value %v of the request, want none of the request kept alive", got)
	}

	done := make(chan struct{})
	go func() {
		c.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Wait() hangs on a desktop that never answers")
	}
	if err := <-desk.ctxErr; !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("the desktop context ended with %v, want the deadline", err)
	}
}

func TestWaitUntilGivesUpWithTheContext(t *testing.T) {
	t.Parallel()
	desk := &slowDesktop{release: make(chan struct{}), ctxErr: make(chan error, 1)}
	c := NewCenter(CenterDeps{Log: testutil.Logger(t), Store: newCenterStore(), Desktop: desk})
	if _, err := c.Post(context.Background(), item()); err != nil {
		t.Fatalf("Post() error = %v", err)
	}

	short, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if c.WaitUntil(short) {
		t.Fatal("WaitUntil() = true while the desktop still holds the banner")
	}

	close(desk.release)
	<-desk.ctxErr
	if !c.WaitUntil(context.Background()) {
		t.Fatal("WaitUntil() = false after the desktop answered")
	}
}
