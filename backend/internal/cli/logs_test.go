package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"slices"
	"strings"
	"testing"

	"github.com/deividfortuna/babysitter/internal/logbook"
)

const daemonLogRecords = `{"records":[
	{"seq":1,"time":"2026-09-28T10:00:00Z","level":"debug","msg":"http","attrs":[{"key":"path","value":"/api/v1/watches"}]},
	{"seq":2,"time":"2026-09-28T10:00:01Z","level":"info","msg":"daemon listening","attrs":[{"key":"addr","value":"127.0.0.1:4000"}]},
	{"seq":3,"time":"2026-09-28T10:00:02Z","level":"warn","msg":"poll failed","attrs":[{"key":"err","value":"read tcp: timeout"}]}
],"path":"/data/logs/daemon.log"}`

func serveLogs(d *fakeDaemon) *[]string {
	var levels []string
	d.mux.HandleFunc("/api/v1/logs", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, daemonLogRecords)
	})
	d.mux.HandleFunc("/api/v1/logs/stream", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "event: ready\ndata: {}\n\n")
		fmt.Fprintf(w, "id: 4\nevent: log\ndata: %s\n\n", `{"seq":4,"time":"2026-09-28T10:00:03Z","level":"info","msg":"after `+r.URL.Query().Get("after")+`","attrs":[]}`)
		fmt.Fprintf(w, "id: 5\nevent: log\ndata: %s\n\n", `{"seq":5,"time":"2026-09-28T10:00:04Z","level":"debug","msg":"quiet","attrs":[]}`)
	})
	d.mux.HandleFunc("/api/v1/logs/level", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPut {
			var body map[string]string
			_ = json.NewDecoder(r.Body).Decode(&body)
			levels = append(levels, body["level"])
			fmt.Fprintf(w, `{"level":%q}`, body["level"])
			return
		}
		fmt.Fprint(w, `{"level":"info"}`)
	})
	return &levels
}

func TestDaemonLogsPrintTheLastRecordsOfTheDaemon(t *testing.T) {
	t.Parallel()
	d := newFakeDaemon()
	serveLogs(d)

	out, err := runAgainstDaemon(t, d, "daemon", "logs", "-n", "2")
	if err != nil {
		t.Fatalf("daemon logs error = %v", err)
	}
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) != 2 {
		t.Fatalf("daemon logs = %q, want the last two records", out)
	}
	if !strings.Contains(lines[0], "INFO  daemon listening addr=127.0.0.1:4000") {
		t.Fatalf("line = %q, want the level, the message and the attributes", lines[0])
	}
	if !strings.Contains(lines[1], `WARN  poll failed err="read tcp: timeout"`) {
		t.Fatalf("line = %q, want the value with a space quoted", lines[1])
	}
}

func TestDaemonLogsKeepOnlyTheLevelAskedFor(t *testing.T) {
	t.Parallel()
	d := newFakeDaemon()
	serveLogs(d)

	out, err := runAgainstDaemon(t, d, "daemon", "logs", "--level", "warn")
	if err != nil {
		t.Fatalf("daemon logs error = %v", err)
	}
	if strings.Contains(out, "daemon listening") || !strings.Contains(out, "poll failed") {
		t.Fatalf("daemon logs = %q, want only the warning", out)
	}
}

func TestDaemonLogsFollowPrintsTheNewRecordsAfterTheLastOne(t *testing.T) {
	t.Parallel()
	d := newFakeDaemon()
	serveLogs(d)

	out, err := runAgainstDaemon(t, d, "daemon", "logs", "-f", "--level", "info")
	if err == nil || !strings.Contains(err.Error(), "closed the log stream") {
		t.Fatalf("daemon logs -f error = %v, want the end of the stream", err)
	}
	if !strings.Contains(out, "after 3") {
		t.Fatalf("daemon logs -f = %q, want the stream to start after seq 3", out)
	}
	if strings.Contains(out, "quiet") {
		t.Fatalf("daemon logs -f = %q, want the debug record left out", out)
	}
}

func TestDaemonLogsPrintJSON(t *testing.T) {
	t.Parallel()
	d := newFakeDaemon()
	serveLogs(d)

	out, err := runAgainstDaemon(t, d, "daemon", "logs", "--output", "json")
	if err != nil {
		t.Fatalf("daemon logs error = %v", err)
	}
	var got logList
	if err := json.Unmarshal([]byte(out), &got); err != nil || len(got.Records) != 3 {
		t.Fatalf("daemon logs = %q, %v, want the three records as JSON", out, err)
	}
}

func TestDaemonLogsFollowPrintJSONLines(t *testing.T) {
	t.Parallel()
	d := newFakeDaemon()
	serveLogs(d)

	out, _ := runAgainstDaemon(t, d, "daemon", "logs", "-f", "--output", "json")

	lines := strings.Split(strings.TrimSpace(out), "\n")
	var seqs []int64
	for _, line := range lines {
		var r logbook.Record
		if err := json.Unmarshal([]byte(line), &r); err != nil {
			t.Fatalf("line %q is not one JSON record: %v", line, err)
		}
		seqs = append(seqs, r.Seq)
	}
	if want := []int64{1, 2, 3, 4, 5}; !slices.Equal(seqs, want) {
		t.Fatalf("seqs = %v, want the kept records then the streamed ones", seqs)
	}
}

func writeLogFile(t *testing.T, path string, msgs ...string) {
	t.Helper()
	book, err := logbook.Open(logbook.Options{Path: path})
	if err != nil {
		t.Fatal(err)
	}
	log := slog.New(book.Handler())
	for _, msg := range msgs {
		log.Info(msg)
	}
	if err := book.Close(); err != nil {
		t.Fatal(err)
	}
}

func runWithoutDaemon(t *testing.T, dataDir string, args ...string) (string, error) {
	t.Helper()
	var out bytes.Buffer
	root := NewRootCmd()
	root.SetOut(&out)
	root.SetErr(&bytes.Buffer{})
	root.SetArgs(append([]string{"daemon", "--data-dir", dataDir}, args...))
	err := root.ExecuteContext(context.Background())
	return out.String(), err
}

func TestDaemonLogsReadTheFileWhenNoDaemonRuns(t *testing.T) {
	t.Parallel()
	dataDir := t.TempDir()
	writeLogFile(t, daemonLogPath(dataDir), "before the stop")

	out, err := runWithoutDaemon(t, dataDir, "logs")
	if err != nil || !strings.Contains(out, "before the stop") {
		t.Fatalf("daemon logs = %q, %v, want the record of the file", out, err)
	}

	if _, err := runWithoutDaemon(t, dataDir, "logs", "-f"); err == nil || !strings.Contains(err.Error(), "no daemon is running") {
		t.Fatalf("daemon logs -f error = %v, want no daemon", err)
	}
}

func TestDaemonLogsAppReadTheLogOfTheDesktopApp(t *testing.T) {
	t.Parallel()
	dataDir := t.TempDir()
	writeLogFile(t, appLogPath(dataDir), "daemon: attached to pid 42")

	out, err := runWithoutDaemon(t, dataDir, "logs", "--app")
	if err != nil || !strings.Contains(out, "daemon: attached to pid 42") {
		t.Fatalf("daemon logs --app = %q, %v, want the record of the app", out, err)
	}
	if _, err := runWithoutDaemon(t, dataDir, "logs", "--app", "-f"); err == nil || !strings.Contains(err.Error(), "--follow reads the daemon only") {
		t.Fatalf("daemon logs --app -f error = %v, want a refusal", err)
	}
}

func TestDaemonLogLevelPrintsAndChangesTheLevel(t *testing.T) {
	t.Parallel()
	d := newFakeDaemon()
	levels := serveLogs(d)

	out, err := runAgainstDaemon(t, d, "daemon", "log-level")
	if err != nil || !strings.Contains(out, "Log level: info") {
		t.Fatalf("daemon log-level = %q, %v, want info", out, err)
	}

	out, err = runAgainstDaemon(t, d, "daemon", "log-level", "DEBUG")
	if err != nil || !strings.Contains(out, "Log level: debug") {
		t.Fatalf("daemon log-level DEBUG = %q, %v, want debug", out, err)
	}
	if len(*levels) != 1 || (*levels)[0] != "debug" {
		t.Fatalf("levels sent = %v, want debug", *levels)
	}

	if _, err := runAgainstDaemon(t, d, "daemon", "log-level", "verbose"); err == nil || !strings.Contains(err.Error(), "unknown log level") {
		t.Fatalf("daemon log-level verbose error = %v, want a refusal", err)
	}
	if len(*levels) != 1 {
		t.Fatalf("levels sent = %v, want nothing sent for an unknown level", *levels)
	}
}
