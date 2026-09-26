package httpd

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/deividfortuna/babysitter/internal/events"
)

const eventsBuffer = 256

var heartbeatInterval = 15 * time.Second

func (a *api) presents(r *http.Request) bool {
	if a.notifications == nil {
		return false
	}
	return r.URL.Query().Get("present") == "1"
}

func (a *api) handleEvents(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, "sse_unsupported", "streaming is not supported")
		return
	}
	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()

	mark := "null"
	if a.presents(r) {
		id, release, err := a.notifications.Present(ctx)
		if err != nil {
			a.log.Warn("notifications unreadable, the daemon keeps showing them", "err", err)
		} else {
			mark = strconv.FormatInt(id, 10)
			defer release()
		}
	}

	live := make(chan events.Event, eventsBuffer)
	unsubscribe := a.bus.Subscribe(func(e events.Event) {
		select {
		case live <- e:
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
	if _, err := fmt.Fprintf(w, "event: ready\ndata: {\"seq\":%d,\"lastNotificationId\":%s}\n\n", a.bus.LatestSeq(), mark); err != nil {
		return
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
		case e := <-live:
			data, err := json.Marshal(e)
			if err != nil {
				return
			}
			if _, err := fmt.Fprintf(w, "id: %d\nevent: %s\ndata: %s\n\n", e.Seq, e.Type, data); err != nil {
				return
			}
			flusher.Flush()
		}
	}
}
