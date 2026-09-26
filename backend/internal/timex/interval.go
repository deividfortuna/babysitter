package timex

import (
	"sync"
	"time"
)

type Interval struct {
	mu     sync.RWMutex
	d      time.Duration
	retune chan struct{}
}

func NewInterval(d time.Duration) *Interval {
	return &Interval{d: d, retune: make(chan struct{}, 1)}
}

func (i *Interval) Duration() time.Duration {
	i.mu.RLock()
	defer i.mu.RUnlock()
	return i.d
}

func (i *Interval) Set(d time.Duration) {
	if d <= 0 {
		return
	}
	i.mu.Lock()
	i.d = d
	i.mu.Unlock()
	select {
	case i.retune <- struct{}{}:
	default:
	}
}

func (i *Interval) Retuned() <-chan struct{} { return i.retune }
