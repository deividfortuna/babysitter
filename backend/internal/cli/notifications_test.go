package cli

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

func notificationRoutes(d *fakeDaemon) {
	d.notifications = `[{"id":2,"kind":"review","repo":"octo/hello","number":3,"title":"PR #3","body":"bob commented: hi","url":"https://github.com/octo/hello/pull/3","createdAt":"2026-09-07T12:03:00Z"},
		{"id":1,"kind":"watch","repo":"octo/hello","number":3,"title":"PR #3","body":"Watching fix","url":"","createdAt":"2026-09-07T12:00:00Z","readAt":"2026-09-07T12:01:00Z"}]`
	d.mux.HandleFunc("/api/v1/notifications", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			var body map[string]any
			_ = json.NewDecoder(r.Body).Decode(&body)
			d.posted = append(d.posted, body)
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"id":9,"kind":"agent","title":"babysitter","body":"Reply to alice","createdAt":"2026-09-07T12:05:00Z"}`))
			return
		}
		d.listQuery = r.URL.RawQuery
		_, _ = w.Write([]byte(`{"notifications":` + d.notifications + `,"unreadCount":1}`))
	})
	d.mux.HandleFunc("/api/v1/notifications/read", func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		d.reads = append(d.reads, body)
		_, _ = w.Write([]byte(`{"unreadCount":0}`))
	})
}

func runNotifications(t *testing.T, d *fakeDaemon, args ...string) (string, error) {
	t.Helper()
	return runAgainstDaemon(t, d, "notifications", args...)
}

func TestNotificationsListPrintsTheHistory(t *testing.T) {
	t.Parallel()
	d := newFakeDaemon()
	notificationRoutes(d)

	out, err := runNotifications(t, d, "list")
	if err != nil {
		t.Fatalf("notifications list error = %v", err)
	}
	for _, want := range []string{"bob commented: hi", "review", "1 unread"} {
		if !strings.Contains(out, want) {
			t.Fatalf("notifications list = %q, want it to carry %q", out, want)
		}
	}
}

func TestNotificationsListAsksForTheUnreadOnes(t *testing.T) {
	t.Parallel()
	d := newFakeDaemon()
	notificationRoutes(d)

	if _, err := runNotifications(t, d, "list", "--unread", "--limit", "5"); err != nil {
		t.Fatalf("notifications list error = %v", err)
	}
	if !strings.Contains(d.listQuery, "status=unread") || !strings.Contains(d.listQuery, "limit=5") {
		t.Fatalf("the daemon got %q, want the unread filter and the limit", d.listQuery)
	}
}

func TestNotificationsReadMarksThemAll(t *testing.T) {
	t.Parallel()
	d := newFakeDaemon()
	notificationRoutes(d)

	out, err := runNotifications(t, d, "read")
	if err != nil {
		t.Fatalf("notifications read error = %v", err)
	}
	if len(d.reads) != 1 {
		t.Fatalf("the daemon got %d writes, want 1", len(d.reads))
	}
	if _, ok := d.reads[0]["ids"]; ok {
		t.Fatalf("body = %v, want no ids so that every unread row is marked", d.reads[0])
	}
	if !strings.Contains(out, "0 unread") {
		t.Fatalf("notifications read = %q, want the count that is left", out)
	}
}

func TestNotificationsReadMarksTheGivenRows(t *testing.T) {
	t.Parallel()
	d := newFakeDaemon()
	notificationRoutes(d)

	if _, err := runNotifications(t, d, "read", "2"); err != nil {
		t.Fatalf("notifications read error = %v", err)
	}
	ids, ok := d.reads[0]["ids"].([]any)
	if !ok || len(ids) != 1 || ids[0] != float64(2) {
		t.Fatalf("body = %v, want the one id the command was given", d.reads[0])
	}
}

func TestNotificationsReadRefusesAnIDThatIsNotANumber(t *testing.T) {
	t.Parallel()
	d := newFakeDaemon()
	notificationRoutes(d)

	if _, err := runNotifications(t, d, "read", "two"); err == nil {
		t.Fatal("notifications read took an id that is not a number")
	}
}
