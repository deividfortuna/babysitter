package keyedlock

import "sync"

type entry struct {
	mu    sync.Mutex
	users int
}

type Locks[K comparable] struct {
	mu sync.Mutex
	m  map[K]*entry
}

func (k *Locks[K]) Lock(key K) (unlock func()) {
	e := k.claim(key)
	e.mu.Lock()
	return func() {
		e.mu.Unlock()
		k.release(key)
	}
}

func (k *Locks[K]) Users(key K) int {
	k.mu.Lock()
	defer k.mu.Unlock()
	if e, ok := k.m[key]; ok {
		return e.users
	}
	return 0
}

func (k *Locks[K]) claim(key K) *entry {
	k.mu.Lock()
	defer k.mu.Unlock()
	if k.m == nil {
		k.m = map[K]*entry{}
	}
	e, ok := k.m[key]
	if !ok {
		e = &entry{}
		k.m[key] = e
	}
	e.users++
	return e
}

func (k *Locks[K]) release(key K) {
	k.mu.Lock()
	defer k.mu.Unlock()
	e, ok := k.m[key]
	if !ok {
		return
	}
	e.users--
	if e.users == 0 {
		delete(k.m, key)
	}
}
