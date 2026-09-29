package prwatch

import (
	"fmt"
	"sync"
	"time"
)

const defaultMaxInterval = 15 * time.Minute

type cadence struct {
	shortest time.Duration
	longest  time.Duration
}

func (c cadence) clamp(d time.Duration) time.Duration {
	return min(max(d, c.shortest), c.longest)
}

func (c cadence) adaptive() bool { return c.longest > c.shortest }

func (c cadence) String() string {
	if !c.adaptive() {
		return "every " + c.shortest.String()
	}
	return fmt.Sprintf("every %s, up to %s when quiet", c.shortest, c.longest)
}

type pace int

const (
	keepPace pace = iota
	speedUp
	slowDown
)

type slot struct {
	polled time.Time
	wait   time.Duration
}

func (sl slot) next(c cadence) time.Time { return sl.polled.Add(c.clamp(sl.wait)) }

type schedule struct {
	mu    sync.Mutex
	now   func() time.Time
	slots map[int64]slot
}

func newSchedule(now func() time.Time) *schedule {
	return &schedule{now: now, slots: map[int64]slot{}}
}

func (sc *schedule) due(id int64, c cadence) bool {
	sc.mu.Lock()
	defer sc.mu.Unlock()
	sl, ok := sc.slots[id]
	return !ok || !sc.now().Before(sl.next(c))
}

func (sc *schedule) polled(id int64, p pace, c cadence) {
	sc.mu.Lock()
	defer sc.mu.Unlock()
	sl := sc.slots[id]
	sl.polled = sc.now()
	switch p {
	case speedUp:
		sl.wait = c.shortest
	case slowDown:
		sl.wait = c.clamp(2 * c.clamp(sl.wait))
	case keepPace:
	}
	sc.slots[id] = sl
}

func (sc *schedule) wake(id int64) {
	sc.mu.Lock()
	defer sc.mu.Unlock()
	sl := sc.slots[id]
	sl.polled = time.Time{}
	sc.slots[id] = sl
}

func (sc *schedule) restart() {
	sc.mu.Lock()
	defer sc.mu.Unlock()
	for id, sl := range sc.slots {
		sl.wait = 0
		sc.slots[id] = sl
	}
}

func (sc *schedule) keep(active map[int64]bool) {
	sc.mu.Lock()
	defer sc.mu.Unlock()
	for id := range sc.slots {
		if !active[id] {
			delete(sc.slots, id)
		}
	}
}

func (sc *schedule) untilNext(c cadence) time.Duration {
	sc.mu.Lock()
	defer sc.mu.Unlock()
	now := sc.now()
	wait := c.shortest
	for _, sl := range sc.slots {
		wait = min(wait, sl.next(c).Sub(now))
	}
	return max(wait, 0)
}
