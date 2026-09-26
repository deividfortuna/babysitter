package prwatch

import (
	"cmp"
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/deividfortuna/babysitter/internal/redact"
	"github.com/deividfortuna/babysitter/internal/store"
)

func (s *Service) report(ctx context.Context, w store.Watch, items []store.Activity) {
	for _, a := range items {
		s.log.Info(redact.Text(a.Summary), "watch", w.ID, "pr", prLabel(w), "kind", a.Kind, "actor", a.Actor)
		if item, ok := s.notification(w, a); ok {
			s.notify(ctx, item)
		}
	}
	if err := s.store.MarkActivityReported(ctx, ids(items), s.now()); err != nil {
		s.log.Error("mark activity reported", "watch", w.ID, "err", err)
	}
	s.store.PublishActivity(w.Key())
}

func clean(a store.Activity) store.Activity {
	a.Summary = redact.Text(a.Summary)
	a.URL = redact.Text(a.URL)
	if len(a.Payload) > 0 {
		a.Payload = []byte(redact.Text(string(a.Payload)))
	}
	return a
}

func (s *Service) record(ctx context.Context, w store.Watch, a store.Activity) (store.Activity, error) {
	row, isNew, err := s.insert(ctx, w, a)
	if err != nil {
		return store.Activity{}, err
	}
	if isNew {
		s.report(ctx, w, []store.Activity{row})
	}
	return row, nil
}

func (s *Service) insert(ctx context.Context, w store.Watch, a store.Activity) (store.Activity, bool, error) {
	a.WatchID = w.ID
	if a.At.IsZero() {
		a.At = s.now()
	}
	a.URL = cmp.Or(a.URL, w.URL)
	return s.store.InsertActivity(ctx, clean(a))
}

func (s *Service) agentFailed(ctx context.Context, w store.Watch, what string, cause error) {
	ctx = context.WithoutCancel(ctx)
	msg := redact.Text(cause.Error())
	s.log.Error("agent session failed", "watch", w.ID, "what", what, "err", msg)
	if _, err := s.record(ctx, w, store.Activity{
		Kind: store.ActivityAgentFailed, Ref: stampRef(what, s.now()),
		Summary: fmt.Sprintf("could not %s: %s", what, firstLine(msg)),
		Payload: mustJSON(map[string]any{"what": what, "error": msg}),
	}); err != nil {
		s.log.Error("record agent failure", "watch", w.ID, "err", err)
	}
}

func stampRef(key any, at time.Time) string {
	return fmt.Sprintf("%v@%s", key, at.UTC().Format(time.RFC3339))
}

func prLabel(w store.Watch) string {
	return fmt.Sprintf("%s#%d", w.Repo(), w.Number)
}

func mustJSON(v any) json.RawMessage {
	b, err := json.Marshal(v)
	if err != nil {
		return json.RawMessage("{}")
	}
	return b
}
