package ghauth

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/deividfortuna/babysitter/internal/ghclient/ghfake"
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
	testutil.Eventually(t, func() bool { return changes.Load() == 1 }, "the start is told")

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

func TestSignInsStartAfterCancelGivesANewCode(t *testing.T) {
	h := newHarness(t)
	s, _ := h.signIns(t)
	first, err := s.Start(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	s.Cancel()
	second, err := s.Start(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if second.UserCode == first.UserCode {
		t.Fatalf("Start after Cancel answered the cancelled code %s", first.UserCode)
	}
	h.g.ApproveDevice()
	waitForEnd(t, s)
	if c, err := newCredentialsFile(h.dir).Load(); err != nil || c.Login != "alice" {
		t.Fatalf("Load = %+v, %v; want the sign in of the new code", c, err)
	}
}

func TestSignInsCurrentDoesNotWaitForASlowCode(t *testing.T) {
	h := newHarness(t)
	release := make(chan struct{})
	h.g.React(ghfake.RouteDeviceCode, func(ghfake.Action) (ghfake.Response, bool) {
		<-release
		return ghfake.Response{}, false
	})
	s, _ := h.signIns(t)
	started := make(chan error, 1)
	go func() {
		_, err := s.Start(context.Background())
		started <- err
	}()
	testutil.Eventually(t, func() bool { return h.g.Count(ghfake.RouteDeviceCode) == 1 }, "GitHub is asked for a code")

	answered := make(chan struct{})
	go func() {
		s.Current()
		s.Err()
		close(answered)
	}()
	select {
	case <-answered:
	case <-time.After(2 * time.Second):
		t.Fatal("Current waited for the code request")
	}
	close(release)
	if err := <-started; err != nil {
		t.Fatal(err)
	}
}

func TestCancelledSignInDoesNotSave(t *testing.T) {
	h := newHarness(t)
	a := h.auth()
	code, err := a.RequestCode(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	h.g.ApproveDevice()
	ctx, cancel := context.WithCancel(context.Background())
	whoami := func(ctx context.Context, token string) (Identity, error) {
		cancel()
		return h.whoami(t)(ctx, token)
	}

	_, err = a.Complete(ctx, code, whoami)

	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Complete = %v, want context.Canceled", err)
	}
	if _, err := newCredentialsFile(h.dir).Load(); !errors.Is(err, ErrSignedOut) {
		t.Fatalf("Load = %v, want ErrSignedOut: a cancelled sign in saved", err)
	}
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
