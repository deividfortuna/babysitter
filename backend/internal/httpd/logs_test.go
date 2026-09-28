package httpd

import (
	"bufio"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/deividfortuna/babysitter/internal/events"
	"github.com/deividfortuna/babysitter/internal/logbook"
	"github.com/deividfortuna/babysitter/internal/testutil"
)

func newLogAPI(t *testing.T) (http.Handler, *logbook.Book, *events.Bus) {
	t.Helper()
	book, err := logbook.Open(logbook.Options{Level: slog.LevelInfo})
	if err != nil {
		t.Fatal(err)
	}
	bus := events.NewBus()
	h := NewRouter(Deps{Log: slog.New(book.Handler()), Bus: bus, Logs: book})
	return h, book, bus
}

func TestLogsAnswerTheKeptRecordsAfterASeq(t *testing.T) {
	t.Parallel()
	h, book, _ := newLogAPI(t)
	log := slog.New(book.Handler()).With("component", "prwatch")
	log.Info("first")
	log.Warn("second", "watch", 7)
	log.Error("third")

	var got LogList
	if rec := call(t, h, http.MethodGet, "/logs?after=1&limit=1", "", &got); rec.Code != http.StatusOK {
		t.Fatalf("GET /logs = %d, want 200: %s", rec.Code, rec.Body)
	}
	if len(got.Records) != 1 || got.Records[0].Msg != "third" || got.Records[0].Level != "error" {
		t.Fatalf("records = %+v, want only the last one", got.Records)
	}

	call(t, h, http.MethodGet, "/logs?after=1", "", &got)
	second := got.Records[0]
	if second.Seq != 2 || second.Level != "warn" || len(second.Attrs) != 2 || second.Attrs[1] != (LogAttr{Key: "watch", Value: "7"}) {
		t.Fatalf("second = %+v, want warn with its attributes", second)
	}
}

func TestLogsRefuseANegativeLimit(t *testing.T) {
	t.Parallel()
	h, _, _ := newLogAPI(t)

	if rec := call(t, h, http.MethodGet, "/logs?limit=-1", "", nil); rec.Code != http.StatusBadRequest {
		t.Fatalf("GET /logs?limit=-1 = %d, want 400", rec.Code)
	}
}

func TestLogStreamRefusesANegativeAfter(t *testing.T) {
	t.Parallel()
	h, _, _ := newLogAPI(t)

	if rec := call(t, h, http.MethodGet, "/logs/stream?after=-1", "", nil); rec.Code != http.StatusBadRequest {
		t.Fatalf("GET /logs/stream?after=-1 = %d, want 400", rec.Code)
	}
}

func TestLogsRefuseALimitPastTheMaximum(t *testing.T) {
	t.Parallel()
	h, _, _ := newLogAPI(t)

	rec := call(t, h, http.MethodGet, "/logs?limit=9223372036854775807", "", nil)
	if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "limit must be 100000 or less") {
		t.Fatalf("GET /logs with a huge limit = %d %s, want 400", rec.Code, rec.Body)
	}
}

func TestLogsAreUnavailableWithoutABook(t *testing.T) {
	t.Parallel()
	h := NewRouter(Deps{Log: testutil.Logger(t), Bus: events.NewBus()})

	for _, path := range []string{"/logs", "/logs/level", "/logs/stream"} {
		if rec := call(t, h, http.MethodGet, path, "", nil); rec.Code != http.StatusServiceUnavailable {
			t.Fatalf("GET %s = %d, want 503", path, rec.Code)
		}
	}
}

func TestLogLevelChangesWhatTheDaemonRecordsAndTellsTheApp(t *testing.T) {
	t.Parallel()
	h, book, bus := newLogAPI(t)
	var published []events.Type
	bus.Subscribe(func(e events.Event) { published = append(published, e.Type) })

	var level LogLevel
	call(t, h, http.MethodGet, "/logs/level", "", &level)
	if level.Level != "info" {
		t.Fatalf("level = %q, want info", level.Level)
	}

	if rec := call(t, h, http.MethodPut, "/logs/level", `{"level":"debug"}`, &level); rec.Code != http.StatusOK || level.Level != "debug" {
		t.Fatalf("PUT /logs/level = %d %+v, want debug", rec.Code, level)
	}
	if book.Level() != slog.LevelDebug {
		t.Fatalf("book level = %v, want debug", book.Level())
	}
	if len(published) != 1 || published[0] != events.LogLevelChanged {
		t.Fatalf("published = %v, want one log_level_changed", published)
	}

	call(t, h, http.MethodPut, "/logs/level", `{"level":"debug"}`, &level)
	if len(published) != 1 {
		t.Fatalf("published = %v, want no event for the same level", published)
	}
}

func TestLogLevelRefusesAnUnknownName(t *testing.T) {
	t.Parallel()
	h, book, _ := newLogAPI(t)

	rec := call(t, h, http.MethodPut, "/logs/level", `{"level":"verbose"}`, nil)
	if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "debug, info, warn, error") {
		t.Fatalf("PUT /logs/level = %d %s, want 400 with the names", rec.Code, rec.Body)
	}
	if book.Level() != slog.LevelInfo {
		t.Fatalf("book level = %v, want info kept", book.Level())
	}
}

func TestLogStreamSendsTheBacklogThenEachNewRecord(t *testing.T) {
	t.Parallel()
	h, book, _ := newLogAPI(t)
	log := slog.New(book.Handler())
	log.Info("old")
	log.Info("kept")
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, srv.URL+Prefix+"/logs/stream?after=1", nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	frames := bufio.NewScanner(resp.Body)

	if got := nextLogFrame(t, frames); got.Msg != "kept" || got.Seq != 2 {
		t.Fatalf("first frame = %+v, want the kept record after seq 1", got)
	}
	log.Warn("new")
	if got := nextLogFrame(t, frames); got.Msg != "new" || got.Level != "warn" {
		t.Fatalf("next frame = %+v, want the new record", got)
	}
}

func nextLogFrame(t *testing.T, frames *bufio.Scanner) LogRecord {
	t.Helper()
	inLog := false
	for frames.Scan() {
		line := frames.Text()
		if line == "event: log" {
			inLog = true
			continue
		}
		data, ok := strings.CutPrefix(line, "data: ")
		if !ok || !inLog {
			continue
		}
		var rec LogRecord
		if err := json.Unmarshal([]byte(data), &rec); err != nil {
			t.Fatalf("frame %q: %v", data, err)
		}
		return rec
	}
	t.Fatalf("the stream ended: %v", frames.Err())
	return LogRecord{}
}
