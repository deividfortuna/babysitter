package events

import (
	"sync"
	"time"
)

type Type string

const (
	RepoAdded   Type = "repo_added"
	RepoRemoved Type = "repo_removed"
	RepoSynced  Type = "repo_synced"
	RepoChanged Type = "repo_changed"
	PullUpdated Type = "pull_updated"

	WatchStarted  Type = "watch_started"
	WatchStopped  Type = "watch_stopped"
	WatchActivity Type = "watch_activity"
	WatchSession  Type = "watch_session"
	WatchReady    Type = "watch_ready"
	WatchProposal Type = "watch_proposal"
	WatchChanged  Type = "watch_changed"

	NotificationAdded Type = "notification_added"
	NotificationsRead Type = "notifications_read"

	SettingsChanged Type = "settings_changed"

	LogLevelChanged Type = "log_level_changed"

	AuthChanged Type = "auth_changed"
)

type Event struct {
	Seq    int64     `json:"seq"`
	Type   Type      `json:"type"`
	At     time.Time `json:"at"`
	Repo   string    `json:"repo,omitempty"`
	Number int       `json:"number,omitempty"`
}

type Bus struct {
	mu   sync.Mutex
	seq  int64
	next int
	subs map[int]func(Event)
	now  func() time.Time
}

func NewBus() *Bus {
	return &Bus{subs: make(map[int]func(Event)), now: time.Now}
}

func (b *Bus) Publish(t Type, repo string, number int) Event {
	b.mu.Lock()
	b.seq++
	e := Event{Seq: b.seq, Type: t, At: b.now().UTC(), Repo: repo, Number: number}
	subs := make([]func(Event), 0, len(b.subs))
	for _, fn := range b.subs {
		subs = append(subs, fn)
	}
	b.mu.Unlock()

	for _, fn := range subs {
		fn(e)
	}
	return e
}

func (b *Bus) Subscribe(fn func(Event)) (unsubscribe func()) {
	b.mu.Lock()
	id := b.next
	b.next++
	b.subs[id] = fn
	b.mu.Unlock()
	return func() {
		b.mu.Lock()
		delete(b.subs, id)
		b.mu.Unlock()
	}
}

func (b *Bus) LatestSeq() int64 {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.seq
}
