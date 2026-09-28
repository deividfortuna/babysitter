package prwatch

import "sync"

type registry[V any] struct {
	mu sync.Mutex
	m  map[int64]V
}

func (r *registry[V]) get(id int64) V {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.m[id]
}

func (r *registry[V]) set(id int64, v V) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.store(id, v)
}

func (r *registry[V]) drop(id int64) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.m, id)
}

func (r *registry[V]) getOrMake(id int64, newValue func() V) (V, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if v, ok := r.m[id]; ok {
		return v, false
	}
	v := newValue()
	r.store(id, v)
	return v, true
}

func (r *registry[V]) take(id int64) V {
	r.mu.Lock()
	defer r.mu.Unlock()
	v := r.m[id]
	delete(r.m, id)
	return v
}

func (r *registry[V]) takeAll() []V {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]V, 0, len(r.m))
	for id, v := range r.m {
		out = append(out, v)
		delete(r.m, id)
	}
	return out
}

func (r *registry[V]) store(id int64, v V) {
	if r.m == nil {
		r.m = map[int64]V{}
	}
	r.m[id] = v
}

type keyedQueues struct {
	mu   sync.Mutex
	tail map[int64]chan struct{}
}

func (q *keyedQueues) queue(id int64, fn func()) (job func()) {
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.tail == nil {
		q.tail = map[int64]chan struct{}{}
	}
	before := q.tail[id]
	done := make(chan struct{})
	q.tail[id] = done
	return func() {
		defer q.finish(id, done)
		if before != nil {
			<-before
		}
		fn()
	}
}

func (q *keyedQueues) finish(id int64, done chan struct{}) {
	close(done)
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.tail[id] == done {
		delete(q.tail, id)
	}
}
