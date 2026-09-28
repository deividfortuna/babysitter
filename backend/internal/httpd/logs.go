package httpd

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/deividfortuna/babysitter/internal/events"
	"github.com/deividfortuna/babysitter/internal/logbook"
)

const (
	logsDefaultLimit = 500
	logsMaxLimit     = 100_000
	logsBuffer       = 1024
)

type LogAttr struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

type LogRecord struct {
	Seq   int64     `json:"seq" description:"Order of the record in this run of the daemon, from 1"`
	Time  time.Time `json:"time"`
	Level string    `json:"level" enum:"debug,info,warn,error"`
	Msg   string    `json:"msg"`
	Attrs []LogAttr `json:"attrs" description:"The attributes of the record in the order the code gave them; a group joins its keys with a dot"`
}

type LogList struct {
	Records []LogRecord `json:"records" description:"Oldest first"`
	Path    string      `json:"path" description:"The file the daemon writes its log to; empty when it writes none"`
}

type LogQuery struct {
	After int64 `query:"after" description:"Only records with a seq above this"`
	Limit int   `query:"limit" description:"At most this many records from the end, from 0 for all to 100000, default 500"`
}

type LogStreamQuery struct {
	After int64 `query:"after" description:"Send the kept records with a seq above this first, then each new one"`
}

type LogLevel struct {
	Level string `json:"level" enum:"debug,info,warn,error" description:"The lowest level the daemon records. It holds until the daemon stops."`
}

func logRecord(r logbook.Record) LogRecord {
	attrs := make([]LogAttr, len(r.Attrs))
	for i, a := range r.Attrs {
		attrs[i] = LogAttr(a)
	}
	return LogRecord{Seq: r.Seq, Time: r.Time, Level: r.Level, Msg: r.Msg, Attrs: attrs}
}

func logRecords(rs []logbook.Record) []LogRecord {
	out := make([]LogRecord, len(rs))
	for i, r := range rs {
		out[i] = logRecord(r)
	}
	return out
}

func (a *api) logBook(w http.ResponseWriter) (*logbook.Book, bool) {
	if a.logs == nil {
		writeError(w, http.StatusServiceUnavailable, "logs_unavailable", "this daemon keeps no log")
		return nil, false
	}
	return a.logs, true
}

func (a *api) handleListLogs(w http.ResponseWriter, r *http.Request) {
	book, ok := a.logBook(w)
	if !ok {
		return
	}
	after, ok := queryInt(w, r, "after", 0)
	if !ok {
		return
	}
	limit, ok := queryInt(w, r, "limit", logsDefaultLimit)
	if !ok {
		return
	}
	if after < 0 || limit < 0 {
		writeError(w, http.StatusBadRequest, "bad_request", "after and limit must not be negative")
		return
	}
	if limit > logsMaxLimit {
		writeError(w, http.StatusBadRequest, "bad_request", fmt.Sprintf("limit must be %d or less", logsMaxLimit))
		return
	}
	writeJSON(w, http.StatusOK, LogList{Records: logRecords(book.Since(after, int(limit))), Path: book.Path()})
}

func (a *api) handleGetLogLevel(w http.ResponseWriter, r *http.Request) {
	book, ok := a.logBook(w)
	if !ok {
		return
	}
	writeJSON(w, http.StatusOK, LogLevel{Level: logbook.LevelName(book.Level())})
}

func (a *api) handlePutLogLevel(w http.ResponseWriter, r *http.Request) {
	book, ok := a.logBook(w)
	if !ok {
		return
	}
	var req LogLevel
	if err := readJSON(w, r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", "body must be JSON with a level")
		return
	}
	level, err := logbook.ParseLevel(req.Level)
	if err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", err.Error())
		return
	}
	previous := book.Level()
	book.SetLevel(level)
	if level != previous {
		a.log.Info("log level changed", "from", logbook.LevelName(previous), "to", logbook.LevelName(level))
		a.bus.Publish(events.LogLevelChanged, "", 0)
	}
	writeJSON(w, http.StatusOK, LogLevel{Level: logbook.LevelName(level)})
}

func (a *api) handleStreamLogs(w http.ResponseWriter, r *http.Request) {
	book, ok := a.logBook(w)
	if !ok {
		return
	}
	after, ok := queryInt(w, r, "after", 0)
	if !ok {
		return
	}
	if after < 0 {
		writeError(w, http.StatusBadRequest, "bad_request", "after must not be negative")
		return
	}
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, "sse_unsupported", "streaming is not supported")
		return
	}
	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()

	live := make(chan logbook.Record, logsBuffer)
	unsubscribe := book.Subscribe(func(rec logbook.Record) {
		select {
		case live <- rec:
		default:
			cancel()
		}
	})
	defer unsubscribe()

	h := w.Header()
	h.Set("Content-Type", "text/event-stream; charset=utf-8")
	h.Set("Cache-Control", "no-cache")
	h.Set("Connection", "keep-alive")
	h.Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)
	if _, err := io.WriteString(w, "event: ready\ndata: {}\n\n"); err != nil {
		return
	}
	sent := after
	for _, rec := range book.Since(after, 0) {
		if writeLogFrame(w, rec) != nil {
			return
		}
		sent = rec.Seq
	}
	flusher.Flush()

	heartbeat := time.NewTicker(heartbeatInterval)
	defer heartbeat.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-heartbeat.C:
			if _, err := io.WriteString(w, ":\n\n"); err != nil {
				return
			}
			flusher.Flush()
		case rec := <-live:
			if rec.Seq <= sent {
				continue
			}
			if writeLogFrame(w, rec) != nil {
				return
			}
			sent = rec.Seq
			flusher.Flush()
		}
	}
}

func writeLogFrame(w io.Writer, rec logbook.Record) error {
	data, err := json.Marshal(logRecord(rec))
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(w, "id: %d\nevent: log\ndata: %s\n\n", rec.Seq, data)
	return err
}
