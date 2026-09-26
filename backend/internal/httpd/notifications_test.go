package httpd

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/deividfortuna/babysitter/internal/events"
	"github.com/deividfortuna/babysitter/internal/notify"
	"github.com/deividfortuna/babysitter/internal/store"
	"github.com/deividfortuna/babysitter/internal/testutil"
)

func seedNotification(t *testing.T, st *store.Store, title string) store.Notification {
	t.Helper()
	n, err := st.AddNotification(context.Background(), store.Notification{
		Kind:      store.NotificationReview,
		Repo:      "octo/hello",
		Number:    42,
		Title:     title,
		Body:      "alice left a comment",
		URL:       "https://github.com/octo/hello/pull/42",
		CreatedAt: time.Now().UTC(),
	})
	if err != nil {
		t.Fatal(err)
	}
	return n
}

func TestListNotificationsAnswersNewestFirstWithTheUnreadCount(t *testing.T) {
	t.Parallel()
	h, st, _ := newTestAPI(t)
	seedNotification(t, st, "first")
	second := seedNotification(t, st, "second")

	var out NotificationList
	rec := call(t, h, http.MethodGet, "/notifications", "", &out)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /notifications = %d, want 200: %s", rec.Code, rec.Body)
	}
	if len(out.Notifications) != 2 || out.Notifications[0].ID != second.ID {
		t.Fatalf("GET /notifications = %+v, want the newest row first", out.Notifications)
	}
	if out.UnreadCount != 2 {
		t.Errorf("unreadCount = %d, want 2", out.UnreadCount)
	}
	if out.Notifications[0].Repo != "octo/hello" || out.Notifications[0].Number != 42 {
		t.Errorf("row = %+v, want the pull request it is about", out.Notifications[0])
	}
}

func TestListNotificationsTakesTheUnreadFilterAndTheLimit(t *testing.T) {
	t.Parallel()
	h, st, _ := newTestAPI(t)
	read := seedNotification(t, st, "read")
	seedNotification(t, st, "unread")
	if err := st.MarkNotificationsRead(context.Background(), []int64{read.ID}, time.Now()); err != nil {
		t.Fatal(err)
	}

	var out NotificationList
	call(t, h, http.MethodGet, "/notifications?status=unread", "", &out)
	if len(out.Notifications) != 1 || out.Notifications[0].Title != "unread" {
		t.Fatalf("GET /notifications?status=unread = %+v, want the one unread row", out.Notifications)
	}

	var limited NotificationList
	call(t, h, http.MethodGet, "/notifications?limit=1", "", &limited)
	if len(limited.Notifications) != 1 {
		t.Fatalf("GET /notifications?limit=1 returned %d rows", len(limited.Notifications))
	}
	if limited.UnreadCount != 1 {
		t.Errorf("unreadCount = %d, want the whole count of 1 and not the count of the page", limited.UnreadCount)
	}
}

func TestListNotificationsRefusesABadLimit(t *testing.T) {
	t.Parallel()
	h, _, _ := newTestAPI(t)

	rec := call(t, h, http.MethodGet, "/notifications?limit=all", "", nil)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("GET /notifications?limit=all = %d, want 400", rec.Code)
	}
}

func TestListNotificationsRefusesAStatusItDoesNotKnow(t *testing.T) {
	t.Parallel()
	h, _, _ := newTestAPI(t)

	for _, status := range []string{"unraed", "read", "true"} {
		rec := call(t, h, http.MethodGet, "/notifications?status="+status, "", nil)
		if rec.Code != http.StatusBadRequest {
			t.Errorf("GET /notifications?status=%s = %d, want 400", status, rec.Code)
		}
	}
}

func TestListNotificationsTakesTheTwoWordsOfTheEnum(t *testing.T) {
	t.Parallel()
	h, _, _ := newTestAPI(t)

	for _, status := range []string{"", "all", "unread"} {
		rec := call(t, h, http.MethodGet, "/notifications?status="+status, "", nil)
		if rec.Code != http.StatusOK {
			t.Errorf("GET /notifications?status=%s = %d, want 200", status, rec.Code)
		}
	}
}

func TestListNotificationsRefusesALimitOverTheCeiling(t *testing.T) {
	t.Parallel()
	h, _, _ := newTestAPI(t)

	rec := call(t, h, http.MethodGet, "/notifications?limit=100000000", "", nil)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("GET /notifications?limit=100000000 = %d, want 400", rec.Code)
	}
	if rec := call(t, h, http.MethodGet, "/notifications?limit="+strconv.Itoa(maxNotificationsLimit), "", nil); rec.Code != http.StatusOK {
		t.Fatalf("GET /notifications?limit=%d = %d, want 200 at the ceiling", maxNotificationsLimit, rec.Code)
	}
}

func TestReadNotificationsMarksTheGivenRows(t *testing.T) {
	t.Parallel()
	h, st, _ := newTestAPI(t)
	first := seedNotification(t, st, "first")
	seedNotification(t, st, "second")

	var out NotificationsRead
	rec := call(t, h, http.MethodPost, "/notifications/read", `{"ids":[`+strconv.FormatInt(first.ID, 10)+`]}`, &out)
	if rec.Code != http.StatusOK {
		t.Fatalf("POST /notifications/read = %d, want 200: %s", rec.Code, rec.Body)
	}
	if out.UnreadCount != 1 {
		t.Errorf("unreadCount = %d, want 1 left", out.UnreadCount)
	}
}

func TestReadNotificationsWithNoIDsMarksThemAll(t *testing.T) {
	t.Parallel()
	h, st, _ := newTestAPI(t)
	seedNotification(t, st, "first")
	seedNotification(t, st, "second")

	var out NotificationsRead
	rec := call(t, h, http.MethodPost, "/notifications/read", `{}`, &out)
	if rec.Code != http.StatusOK {
		t.Fatalf("POST /notifications/read = %d, want 200: %s", rec.Code, rec.Body)
	}
	if out.UnreadCount != 0 {
		t.Errorf("unreadCount = %d, want 0", out.UnreadCount)
	}
}

func TestPostNotificationRecordsAndShowsIt(t *testing.T) {
	t.Parallel()
	h, st, _ := newTestAPIWith(t, context.Background(), withCenter(nil))

	var out Notification
	rec := call(t, h, http.MethodPost, "/notifications",
		`{"title":"PR #42","body":"alice needs an answer","url":"https://github.com/octo/hello/pull/42"}`, &out)
	if rec.Code != http.StatusCreated {
		t.Fatalf("POST /notifications = %d, want 201: %s", rec.Code, rec.Body)
	}
	if out.ID == 0 || out.Kind != string(store.NotificationAgent) {
		t.Fatalf("POST /notifications = %+v, want a stored agent notification", out)
	}

	rows, err := st.ListNotifications(context.Background(), store.ListNotificationsOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].Body != "alice needs an answer" {
		t.Fatalf("the store holds %+v, want the notification the request sent", rows)
	}
}

func TestPostNotificationCarriesTheSilentFlagToTheReader(t *testing.T) {
	t.Parallel()
	h, _, _ := newTestAPIWith(t, context.Background(), withCenter(nil))

	var created Notification
	rec := call(t, h, http.MethodPost, "/notifications", `{"title":"PR #42","body":"quietly","silent":true}`, &created)
	if rec.Code != http.StatusCreated {
		t.Fatalf("POST /notifications = %d, want 201: %s", rec.Code, rec.Body)
	}
	if !created.Silent {
		t.Errorf("POST /notifications = %+v, want the silent flag of the request", created)
	}

	var list NotificationList
	call(t, h, http.MethodGet, "/notifications", "", &list)
	if len(list.Notifications) != 1 || !list.Notifications[0].Silent {
		t.Errorf("GET /notifications = %+v, want the silent row", list.Notifications)
	}
}

func TestPostNotificationRefusesAnEmptyBody(t *testing.T) {
	t.Parallel()
	h, _, _ := newTestAPIWith(t, context.Background(), withCenter(nil))

	rec := call(t, h, http.MethodPost, "/notifications", `{"title":"PR #42"}`, nil)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("POST /notifications = %d, want 400: %s", rec.Code, rec.Body)
	}
}

func TestPostNotificationRefusesAnUnknownKind(t *testing.T) {
	t.Parallel()
	h, _, _ := newTestAPIWith(t, context.Background(), withCenter(nil))

	rec := call(t, h, http.MethodPost, "/notifications", `{"title":"hi","body":"there","kind":"gossip"}`, nil)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("POST /notifications = %d, want 400: %s", rec.Code, rec.Body)
	}
}

func TestEventStreamWithPresentTakesTheShowingOverFromTheDaemon(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	var center *notify.Center
	h, _, _ := newTestAPIWith(t, ctx, withCenter(&center))

	openStream(t, ctx, h, "?present=1", func() {
		if !testutil.Within(testutil.Timeout, func() bool { return center.Presenting() }) {
			t.Error("the stream did not take the showing over")
		}
	})

	if !testutil.Within(testutil.Timeout, func() bool { return !center.Presenting() }) {
		t.Fatal("the daemon did not take the showing back after the stream closed")
	}
}

func withCenter(out **notify.Center) func(*Deps) {
	return func(d *Deps) {
		center := notify.NewCenter(notify.CenterDeps{Store: d.Store.(notify.Store)})
		d.Notifications = center
		if out != nil {
			*out = center
		}
	}
}

func TestPostNotificationOfAWatchThatDoesNotExistIsNotFound(t *testing.T) {
	t.Parallel()
	h, _, _ := newTestAPIWith(t, context.Background(), withCenter(nil))

	rec := call(t, h, http.MethodPost, "/notifications", `{"title":"PR #42","body":"alice needs an answer","watchId":999}`, nil)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("POST /notifications = %d, want 404: %s", rec.Code, rec.Body)
	}
}

type unreadableNotifications struct {
	*store.Store
}

func (unreadableNotifications) ListNotifications(context.Context, store.ListNotificationsOptions) ([]store.Notification, error) {
	return nil, errors.New("database is locked")
}

func withUnreadableNotifications(out **notify.Center) func(*Deps) {
	return func(d *Deps) {
		d.Store = unreadableNotifications{Store: d.Store.(*store.Store)}
		withCenter(out)(d)
	}
}

func openStream(t *testing.T, ctx context.Context, h http.Handler, query string, whileOpen func()) *flushRecorder {
	t.Helper()
	streamCtx, stop := context.WithCancel(ctx)
	rec := newFlushRecorder()
	done := make(chan struct{})
	go func() {
		defer close(done)
		req := httptest.NewRequest(http.MethodGet, Prefix+"/events"+query, nil).WithContext(streamCtx)
		h.ServeHTTP(rec, req)
	}()
	select {
	case <-rec.flushed:
	case <-done:
	case <-time.After(2 * time.Second):
		t.Error("the stream neither said ready nor gave up")
	}
	if whileOpen != nil {
		whileOpen()
	}
	stop()
	<-done
	return rec
}

func TestEventStreamOpensWhenTheNotificationsAreUnreadable(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	h, _, _ := newTestAPIWith(t, ctx, withUnreadableNotifications(nil))

	rec := openStream(t, ctx, h, "?present=1", nil)

	if rec.Code != http.StatusOK {
		t.Fatalf("GET /events = %d, want 200: %s", rec.Code, rec.Body)
	}
	if !strings.Contains(rec.Body.String(), "event: ready") {
		t.Fatalf("the stream wrote %q, want the ready frame", rec.Body.String())
	}
}

func TestEventStreamWithPresentLeavesTheShowingWhenTheNotificationsAreUnreadable(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	var center *notify.Center
	h, _, _ := newTestAPIWith(t, ctx, withUnreadableNotifications(&center))

	var presenting bool
	rec := openStream(t, ctx, h, "?present=1", func() { presenting = center.Presenting() })

	if rec.Code != http.StatusOK {
		t.Fatalf("GET /events?present=1 = %d, want 200: %s", rec.Code, rec.Body)
	}
	if presenting {
		t.Fatal("the stream took the showing over with the notifications unreadable")
	}
	if !strings.Contains(rec.Body.String(), `"lastNotificationId":null`) {
		t.Fatalf("the ready frame is %q, want it to say the daemon kept the showing", rec.Body.String())
	}
}

func TestEventStreamReadyFrameNamesTheNewestNotification(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	h, st, _ := newTestAPIWith(t, ctx, withCenter(nil))
	seedNotification(t, st, "PR #41")
	newest := seedNotification(t, st, "PR #42")

	rec := openStream(t, ctx, h, "?present=1", nil)

	want := fmt.Sprintf(`"lastNotificationId":%d`, newest.ID)
	if !strings.Contains(rec.Body.String(), want) {
		t.Fatalf("the ready frame is %q, want it to carry %s", rec.Body.String(), want)
	}
}

type flushRecorder struct {
	*httptest.ResponseRecorder
	flushed chan struct{}
	once    sync.Once
}

func newFlushRecorder() *flushRecorder {
	return &flushRecorder{ResponseRecorder: httptest.NewRecorder(), flushed: make(chan struct{})}
}

func (r *flushRecorder) Flush() {
	r.ResponseRecorder.Flush()
	r.once.Do(func() { close(r.flushed) })
}

type spyNotifier struct {
	mu   sync.Mutex
	sent []notify.Notification
}

func (s *spyNotifier) Send(_ context.Context, n notify.Notification) (notify.Result, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sent = append(s.sent, n)
	return notify.Result{Backend: "spy"}, nil
}

func (s *spyNotifier) count() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.sent)
}

type heldNotificationWrite struct {
	*store.Store
	entered chan struct{}
	release chan struct{}
}

func (h heldNotificationWrite) AddNotification(ctx context.Context, n store.Notification) (store.Notification, error) {
	close(h.entered)
	<-h.release
	return h.Store.AddNotification(ctx, n)
}

func withHeldCenter(out **notify.Center, desktop notify.Notifier, held heldNotificationWrite) func(*Deps) {
	return func(d *Deps) {
		held.Store = d.Store.(*store.Store)
		d.Store = held
		center := notify.NewCenter(notify.CenterDeps{Store: held, Desktop: desktop})
		d.Notifications = center
		*out = center
	}
}

func TestEventStreamLeavesNoNotificationToNobodyWhenItCloses(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	held := heldNotificationWrite{entered: make(chan struct{}), release: make(chan struct{})}
	desktop := &spyNotifier{}
	var center *notify.Center
	h, _, _ := newTestAPIWith(t, ctx, withHeldCenter(&center, desktop, held))

	streamCtx, stopStream := context.WithCancel(ctx)
	rec := newFlushRecorder()
	served := make(chan struct{})
	go func() {
		defer close(served)
		req := httptest.NewRequest(http.MethodGet, Prefix+"/events?present=1", nil).WithContext(streamCtx)
		h.ServeHTTP(rec, req)
	}()
	<-rec.flushed

	posted := make(chan struct{})
	go func() {
		defer close(posted)
		_, err := center.Post(ctx, notify.Item{
			Kind:   store.NotificationChecks,
			Repo:   "octo/hello",
			Number: 42,
			Title:  "PR #42", Message: "checks failed",
		})
		if err != nil {
			t.Error(err)
		}
	}()
	<-held.entered

	stopStream()
	if !testutil.Within(testutil.Timeout, func() bool { return !center.Presenting() }) {
		t.Error("the stream held the showing while a post was inside the handover")
	}
	close(held.release)
	<-posted
	<-served
	center.Wait()

	if strings.Contains(rec.Body.String(), string(events.NotificationAdded)) {
		t.Fatalf("the stream wrote %q, want nothing of the row that landed after it stopped", rec.Body.String())
	}
	if desktop.count() != 1 {
		t.Fatalf("the daemon drew %d notifications of the stream it outlived, want 1", desktop.count())
	}
}
