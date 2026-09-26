package notify

import (
	"context"
	"log/slog"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/deividfortuna/babysitter/internal/store"
)

type Store interface {
	AddNotification(ctx context.Context, n store.Notification) (store.Notification, error)
	ListNotifications(ctx context.Context, o store.ListNotificationsOptions) ([]store.Notification, error)
	Settings(ctx context.Context) (store.Settings, error)
}

type Item struct {
	Kind    store.NotificationKind
	WatchID int64
	Repo    string
	Number  int
	Notification
}

type Poster interface {
	Post(ctx context.Context, item Item) (store.Notification, error)
}

type Center struct {
	store       Store
	desktop     Notifier
	log         *slog.Logger
	showTimeout time.Duration
	presenters  atomic.Int64
	showing     sync.WaitGroup
	handover    sync.RWMutex
}

type CenterDeps struct {
	Store       Store
	Desktop     Notifier
	Log         *slog.Logger
	ShowTimeout time.Duration
}

const DefaultShowTimeout = time.Minute

func NewCenter(d CenterDeps) *Center {
	log := d.Log
	if log == nil {
		log = slog.New(slog.DiscardHandler)
	}
	timeout := d.ShowTimeout
	if timeout <= 0 {
		timeout = DefaultShowTimeout
	}
	return &Center{store: d.Store, desktop: d.Desktop, log: log, showTimeout: timeout}
}

func (c *Center) Present(ctx context.Context) (mark int64, release func(), err error) {
	c.handover.Lock()
	defer c.handover.Unlock()
	mark, err = c.newest(ctx)
	if err != nil {
		return 0, nil, err
	}
	c.presenters.Add(1)
	var once atomic.Bool
	return mark, func() {
		if once.CompareAndSwap(false, true) {
			c.presenters.Add(-1)
		}
	}, nil
}

func (c *Center) newest(ctx context.Context) (int64, error) {
	if c.store == nil {
		return 0, nil
	}
	rows, err := c.store.ListNotifications(ctx, store.ListNotificationsOptions{Limit: 1})
	if err != nil || len(rows) == 0 {
		return 0, err
	}
	return rows[0].ID, nil
}

func (c *Center) Presenting() bool { return c.presenters.Load() > 0 }

func (c *Center) Post(ctx context.Context, item Item) (store.Notification, error) {
	item.Message = strings.TrimSpace(item.Message)
	item.Title = strings.TrimSpace(item.Title)
	if item.Title == "" {
		item.Title = DefaultTitle
	}
	row := store.Notification{
		WatchID: item.WatchID,
		Kind:    item.Kind,
		Repo:    item.Repo,
		Number:  item.Number,
		Title:   item.Title,
		Body:    body(item),
		URL:     item.URL,
		Silent:  item.Silent,
	}
	if err := row.Validate(); err != nil {
		return store.Notification{}, err
	}
	c.handover.RLock()
	defer c.handover.RUnlock()
	if c.store != nil {
		stored, err := c.store.AddNotification(ctx, row)
		if err != nil {
			return store.Notification{}, err
		}
		row = stored
	}
	c.show(ctx, item)
	return row, nil
}

func (c *Center) Wait() { c.showing.Wait() }

func (c *Center) WaitUntil(ctx context.Context) bool {
	done := make(chan struct{})
	go func() {
		c.showing.Wait()
		close(done)
	}()
	select {
	case <-done:
		return true
	case <-ctx.Done():
		return false
	}
}

func body(item Item) string {
	if item.Message == "" || item.Subtitle == "" || item.Repo != "" {
		return item.Message
	}
	return item.Subtitle + ": " + item.Message
}

func (c *Center) show(ctx context.Context, item Item) {
	if c.desktop == nil || c.Presenting() {
		return
	}
	settings := store.DefaultSettings()
	if c.store != nil {
		stored, err := c.store.Settings(ctx)
		if err != nil {
			c.log.Warn("notification settings unreadable, showing nothing", "err", err)
			return
		}
		settings = stored
	}
	if !settings.ShowsNotification(item.Kind) {
		return
	}
	n := item.Notification
	n.Silent = n.Silent || !settings.NotificationSound
	banner, done := context.WithTimeout(context.Background(), c.showTimeout)
	c.showing.Go(func() {
		defer done()
		if _, err := c.desktop.Send(banner, n); err != nil {
			c.log.Warn("notification failed", "title", n.Title, "err", err)
		}
	})
}
