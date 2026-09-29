package prwatch

import (
	"fmt"
	"sync"
	"time"
)

type cadence struct {
	shortest time.Duration
	longest  time.Duration
}

func (c cadence) clamp(d time.Duration) time.Duration {
	return min(max(d, c.shortest), c.longest)
}

func (c cadence) String() string {
	if c.longest == c.shortest {
		return "every " + interval(c.shortest)
	}
	return fmt.Sprintf("every %s, up to %s when quiet", interval(c.shortest), interval(c.longest))
}

type pace int

const (
	keepPace pace = iota
	speedUp
	slowDown
)

type slot struct {
	polled  time.Time
	wait    time.Duration
	stirred bool
}

func (sl slot) next(c cadence) time.Time { return sl.polled.Add(c.clamp(sl.wait)) }

func (sl slot) slower(c cadence) time.Duration {
	if sl.stirred {
		return c.shortest
	}
	return c.clamp(2 * c.clamp(sl.wait))
}

type schedule struct {
	mu    sync.Mutex
	slots map[int64]slot
}

func newSchedule() *schedule {
	return &schedule{slots: map[int64]slot{}}
}

func (sc *schedule) due(id int64, now time.Time, c cadence) bool {
	sc.mu.Lock()
	defer sc.mu.Unlock()
	sl, ok := sc.slots[id]
	return !ok || !now.Before(sl.next(c))
}

func (sc *schedule) polled(id int64, at time.Time, p pace, c cadence) {
	sc.mu.Lock()
	defer sc.mu.Unlock()
	sl := sc.slots[id]
	sl.polled = at
	switch p {
	case speedUp:
		sl.wait = c.shortest
	case slowDown:
		sl.wait = sl.slower(c)
	case keepPace:
	}
	sl.stirred = false
	sc.slots[id] = sl
}

func (sc *schedule) stir(id int64) {
	sc.mu.Lock()
	defer sc.mu.Unlock()
	sl := sc.slots[id]
	sl.stirred = true
	sl.wait = 0
	sc.slots[id] = sl
}

func (sc *schedule) calm(id int64) {
	sc.mu.Lock()
	defer sc.mu.Unlock()
	sl := sc.slots[id]
	sl.stirred = false
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

func (sc *schedule) untilNext(now time.Time, c cadence) time.Duration {
	sc.mu.Lock()
	defer sc.mu.Unlock()
	wait := c.shortest
	for _, sl := range sc.slots {
		wait = min(wait, sl.next(c).Sub(now))
	}
	return max(wait, 0)
}
