package ghauth

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"

	"github.com/deividfortuna/babysitter/internal/testutil"
)

func TestSignInsAnswersTheSamePromptWhileItWaits(t *testing.T) {
	h := newHarness(t)
	var ends atomic.Int32
	s := h.auth().SignIns(t.Context(), h.whoami(t), func() { ends.Add(1) })

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

	h.g.ApproveDevice()
	testutil.Eventually(t, func() bool { return ends.Load() == 1 }, "the sign in ends")
	if _, waiting := s.Current(); waiting {
		t.Fatal("the sign in still waits after GitHub answered")
	}
	if err := s.Err(); err != nil {
		t.Fatalf("Err = %v", err)
	}
	c, err := NewFile(h.dir).Load()
	if err != nil || c.Login != "alice" {
		t.Fatalf("Load = %+v, %v", c, err)
	}
}

func TestSignInsCancelLeavesNoError(t *testing.T) {
	h := newHarness(t)
	var ends atomic.Int32
	s := h.auth().SignIns(t.Context(), h.whoami(t), func() { ends.Add(1) })
	if _, err := s.Start(context.Background()); err != nil {
		t.Fatal(err)
	}

	s.Cancel()

	testutil.Eventually(t, func() bool { return ends.Load() == 1 }, "the sign in ends")
	if err := s.Err(); err != nil {
		t.Fatalf("Err = %v, want nil after a cancel", err)
	}
}

func TestSignInsKeepsTheRefusal(t *testing.T) {
	h := newHarness(t)
	var ends atomic.Int32
	s := h.auth().SignIns(t.Context(), h.whoami(t), func() { ends.Add(1) })
	if _, err := s.Start(context.Background()); err != nil {
		t.Fatal(err)
	}

	h.g.DenyDevice()

	testutil.Eventually(t, func() bool { return ends.Load() == 1 }, "the sign in ends")
	if err := s.Err(); !errors.Is(err, ErrDenied) {
		t.Fatalf("Err = %v, want ErrDenied", err)
	}
}
