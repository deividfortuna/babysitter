package prwatch

import (
	"sync"
	"testing"
	"time"

	"github.com/deividfortuna/babysitter/internal/testutil"
)

func (k *keyedLocks) count() int {
	k.mu.Lock()
	defer k.mu.Unlock()
	return len(k.m)
}

func TestKeyedLocksForgetTheMutexOfAWatchNobodyHolds(t *testing.T) {
	t.Parallel()
	var k keyedLocks
	for id := range int64(500) {
		unlock := k.lock(id)
		unlock()
	}
	if n := k.count(); n != 0 {
		t.Fatalf("the registry holds %d mutexes of watches nobody works on, want 0", n)
	}
}

func TestKeyedLocksKeepTheMutexWhileACallerWaits(t *testing.T) {
	t.Parallel()
	var k keyedLocks
	unlock := k.lock(7)
	waiting := make(chan func())
	go func() { waiting <- k.lock(7) }()
	testutil.Eventually(t, func() bool { return k.count() == 1 }, "the second caller to wait on the lock")
	unlock()
	(<-waiting)()
	if n := k.count(); n != 0 {
		t.Fatalf("the registry holds %d mutexes after the last caller left, want 0", n)
	}
}

func TestKeyedLocksLetOneCallerOfAWatchThroughAtATime(t *testing.T) {
	t.Parallel()
	var k keyedLocks
	var wg sync.WaitGroup
	inside, most := 0, 0
	var count sync.Mutex
	for range 50 {
		wg.Go(func() {
			unlock := k.lock(3)
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
		t.Fatalf("%d callers of one watch worked at once, want 1", most)
	}
	if n := k.count(); n != 0 {
		t.Fatalf("the registry holds %d mutexes, want 0", n)
	}
}
