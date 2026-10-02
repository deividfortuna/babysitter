package ghauth

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"

	"github.com/deividfortuna/babysitter/internal/testutil"
)

func (h *harness) signIns(t *testing.T) (*SignIns, *atomic.Int32) {
	t.Helper()
	var changes atomic.Int32
	a := h.auth()
	a.OnChange(func() { changes.Add(1) })
	return a.SignIns(t.Context(), h.whoami(t)), &changes
}

func waitForEnd(t *testing.T, s *SignIns) {
	t.Helper()
	testutil.Eventually(t, func() bool {
		_, waiting := s.Current()
		return !waiting
	}, "the sign in ends")
}

func TestSignInsAnswersTheSamePromptWhileItWaits(t *testing.T) {
	h := newHarness(t)
	s, changes := h.signIns(t)

	first, err := s.Start(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	second, err := s.Start(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if first != second {
		t.Fatalf("second prompt = %+v, want the first %+v", second, first)
	}
	if changes.Load() != 1 {
		t.Fatalf("changes after the start = %d, want 1", changes.Load())
	}

	h.g.ApproveDevice()
	waitForEnd(t, s)
	if err := s.Err(); err != nil {
		t.Fatalf("Err = %v", err)
	}
	c, err := newCredentialsFile(h.dir).Load()
	if err != nil || c.Login != "alice" {
		t.Fatalf("Load = %+v, %v", c, err)
	}
	testutil.Eventually(t, func() bool { return changes.Load() == 3 }, "the start, the sign in and the end are each told")
}

func TestSignInsCancelLeavesNoError(t *testing.T) {
	h := newHarness(t)
	s, changes := h.signIns(t)
	if _, err := s.Start(context.Background()); err != nil {
		t.Fatal(err)
	}

	s.Cancel()

	waitForEnd(t, s)
	if err := s.Err(); err != nil {
		t.Fatalf("Err = %v, want nil after a cancel", err)
	}
	testutil.Eventually(t, func() bool { return changes.Load() == 2 }, "the start and the end are told")
}

func TestSignInsKeepsTheRefusal(t *testing.T) {
	h := newHarness(t)
	s, _ := h.signIns(t)
	if _, err := s.Start(context.Background()); err != nil {
		t.Fatal(err)
	}

	h.g.DenyDevice()

	waitForEnd(t, s)
	if err := s.Err(); !errors.Is(err, ErrDenied) {
		t.Fatalf("Err = %v, want ErrDenied", err)
	}
}
