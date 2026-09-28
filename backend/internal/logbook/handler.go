package logbook

import (
	"context"
	"log/slog"
	"slices"
	"time"
)

type handler struct {
	book   *Book
	attrs  []Attr
	prefix string
}

func (h *handler) Enabled(_ context.Context, l slog.Level) bool {
	return l >= h.book.level.Level()
}

func (h *handler) Handle(_ context.Context, r slog.Record) error {
	attrs := make([]Attr, len(h.attrs), len(h.attrs)+r.NumAttrs())
	copy(attrs, h.attrs)
	r.Attrs(func(a slog.Attr) bool {
		attrs = appendAttr(attrs, h.prefix, a)
		return true
	})
	at := r.Time
	if at.IsZero() {
		at = time.Now()
	}
	h.book.add(Record{Time: at.UTC(), Level: LevelName(r.Level), Msg: r.Message, Attrs: attrs})
	return nil
}

func (h *handler) WithAttrs(as []slog.Attr) slog.Handler {
	next := *h
	next.attrs = slices.Clone(h.attrs)
	for _, a := range as {
		next.attrs = appendAttr(next.attrs, h.prefix, a)
	}
	return &next
}

func (h *handler) WithGroup(name string) slog.Handler {
	if name == "" {
		return h
	}
	next := *h
	next.prefix = h.prefix + name + "."
	return &next
}

func appendAttr(attrs []Attr, prefix string, a slog.Attr) []Attr {
	a.Value = a.Value.Resolve()
	if a.Equal(slog.Attr{}) {
		return attrs
	}
	if a.Value.Kind() != slog.KindGroup {
		return append(attrs, Attr{Key: prefix + a.Key, Value: a.Value.String()})
	}
	group := prefix
	if a.Key != "" {
		group += a.Key + "."
	}
	for _, member := range a.Value.Group() {
		attrs = appendAttr(attrs, group, member)
	}
	return attrs
}
