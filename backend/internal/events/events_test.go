package events

import "testing"

func TestBusFanOut(t *testing.T) {
	b := NewBus()
	var got []Event
	unsub := b.Subscribe(func(e Event) { got = append(got, e) })

	first := b.Publish(RepoAdded, "owner/name", 0)
	if first.Seq != 1 || first.Type != RepoAdded || first.Repo != "owner/name" {
		t.Fatalf("unexpected event %+v", first)
	}
	unsub()
	b.Publish(RepoRemoved, "owner/name", 0)

	if len(got) != 1 {
		t.Fatalf("subscriber saw %d events, want 1", len(got))
	}
	if b.LatestSeq() != 2 {
		t.Fatalf("LatestSeq = %d, want 2", b.LatestSeq())
	}
}
