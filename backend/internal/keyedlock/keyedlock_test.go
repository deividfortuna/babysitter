package keyedlock

import (
	"sync"
	"testing"
	"time"

	"github.com/deividfortuna/babysitter/internal/testutil"
)

func (k *Locks[K]) count() int {
	k.mu.Lock()
	defer k.mu.Unlock()
	return len(k.m)
}

func TestLocksForgetTheMutexOfAKeyNobodyHolds(t *testing.T) {
	t.Parallel()
	var k Locks[int64]
	for id := range int64(500) {
		unlock := k.Lock(id)
		unlock()
	}
	if n := k.count(); n != 0 {
		t.Fatalf("the registry holds %d mutexes of keys nobody works on, want 0", n)
	}
}

func TestLocksKeepTheMutexWhileACallerWaits(t *testing.T) {
	t.Parallel()
	var k Locks[string]
	unlock := k.Lock("octo/hello")
	waiting := make(chan func())
	go func() { waiting <- k.Lock("octo/hello") }()
	testutil.Eventually(t, func() bool { return k.Users("octo/hello") == 2 }, "the second caller to wait on the lock")
	unlock()
	(<-waiting)()
	if n := k.count(); n != 0 {
		t.Fatalf("the registry holds %d mutexes after the last caller left, want 0", n)
	}
}

func TestLocksLetOneCallerOfAKeyThroughAtATime(t *testing.T) {
	t.Parallel()
	var k Locks[int64]
	var wg sync.WaitGroup
	inside, most := 0, 0
	var count sync.Mutex
	for range 50 {
		wg.Go(func() {
			unlock := k.Lock(3)
			defer unlock()
			count.Lock()
			inside++
			most = max(most, inside)
			count.Unlock()
			time.Sleep(time.Millisecond)
			count.Lock()
			inside--
			count.Unlock()
		})
	}
	wg.Wait()
	if most != 1 {
		t.Fatalf("%d callers of one key worked at once, want 1", most)
	}
	if n := k.count(); n != 0 {
		t.Fatalf("the registry holds %d mutexes, want 0", n)
	}
}
