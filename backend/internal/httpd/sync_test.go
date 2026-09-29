package httpd

import (
	"context"
	"net/http"
	"sync/atomic"
	"testing"
)

type recordingSyncer struct {
	kicks atomic.Int32
	syncs atomic.Int32
}

func (s *recordingSyncer) Kick() { s.kicks.Add(1) }

func (s *recordingSyncer) Sync() { s.syncs.Add(1) }

func TestManualSyncAsksForASyncAndAddingARepoOnlyKicks(t *testing.T) {
	t.Parallel()
	syncer := &recordingSyncer{}
	h, _, _ := newTestAPIWith(t, context.Background(), func(d *Deps) { d.Syncer = syncer })

	if rec := call(t, h, http.MethodPost, "/sync", "", nil); rec.Code != http.StatusAccepted {
		t.Fatalf("sync: %d %s", rec.Code, rec.Body)
	}
	if syncs, kicks := syncer.syncs.Load(), syncer.kicks.Load(); syncs != 1 || kicks != 0 {
		t.Fatalf("after /sync: %d syncs and %d kicks, want 1 sync and no kick", syncs, kicks)
	}

	if rec := call(t, h, http.MethodPost, "/repos", `{"fullName":"o/r"}`, nil); rec.Code != http.StatusCreated {
		t.Fatalf("add repo: %d %s", rec.Code, rec.Body)
	}
	if syncs, kicks := syncer.syncs.Load(), syncer.kicks.Load(); syncs != 1 || kicks != 1 {
		t.Fatalf("after adding a repo: %d syncs and %d kicks, want the 1 sync of before and 1 kick", syncs, kicks)
	}
}
