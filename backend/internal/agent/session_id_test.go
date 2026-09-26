package agent

import "testing"

func TestNewSessionIDIsAUUIDOfVersionFour(t *testing.T) {
	t.Parallel()
	seen := map[string]bool{}
	for range 100 {
		id := NewSessionID()
		if len(id) != 36 || id[14] != '4' {
			t.Fatalf("session id = %q, want a version 4 UUID", id)
		}
		if seen[id] {
			t.Fatalf("session id %q came twice", id)
		}
		seen[id] = true
	}
}
