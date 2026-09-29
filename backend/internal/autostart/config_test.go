package autostart

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"testing"
	"time"

	"github.com/deividfortuna/babysitter/internal/store"
)

func gitCheckout(t *testing.T, origin string) string {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not installed")
	}
	dir := t.TempDir()
	for _, args := range [][]string{{"init", "-q"}, {"remote", "add", "origin", origin}} {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	return dir
}

func TestTheCheckoutMustHaveARemoteForTheRepository(t *testing.T) {
	fx := newFixture(t)
	ctx := context.Background()
	good := gitCheckout(t, "git@github.com:acme/billing.git")
	other := gitCheckout(t, "https://github.com/acme/web.git")

	if err := CheckCheckout(ctx, good, fx.repo); err != nil {
		t.Errorf("CheckCheckout(a checkout of acme/billing) = %v", err)
	}
	for name, dir := range map[string]string{"another repository": other, "not a checkout": t.TempDir(), "no folder": "/no/such/folder"} {
		if err := CheckCheckout(ctx, dir, fx.repo); !errors.Is(err, ErrBadCheckout) {
			t.Errorf("%s: CheckCheckout() = %v, want ErrBadCheckout", name, err)
		}
	}
	err := CheckCheckout(ctx, other, fx.repo)
	if want := other + " has no remote for acme/billing"; err == nil || err.Error() != want {
		t.Errorf("CheckCheckout() = %v, want %q", err, want)
	}
}

func TestAToggleGoesOnWithoutACheckout(t *testing.T) {
	fx := newFixture(t)
	on := true
	cfg, err := Configure(context.Background(), fx.st, fx.repo, Change{AutoStartMine: &on}, fx.now)
	if err != nil {
		t.Fatalf("Configure() = %v", err)
	}
	if !cfg.AutoStarts() || cfg.CheckoutDir != "" {
		t.Fatalf("config = %+v, want auto start on with no checkout", cfg)
	}
}

func TestAToggleRecordsWhenItWentOn(t *testing.T) {
	fx := newFixture(t)
	ctx := context.Background()
	dir := gitCheckout(t, "git@github.com:acme/billing.git")
	on, off := true, false
	first := time.Date(2026, 9, 24, 10, 0, 0, 0, time.UTC)

	cfg, err := Configure(ctx, fx.st, fx.repo, Change{CheckoutDir: &dir, AutoStartMine: &on, AutoWatchDependabot: &on}, first)
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.OwnSince.Equal(first) || !cfg.DependabotSince.Equal(first) {
		t.Fatalf("since = %v, %v, want %v", cfg.OwnSince, cfg.DependabotSince, first)
	}
	cfg, err = Configure(ctx, fx.st, fx.repo, Change{AutoStartMine: &on}, first.Add(time.Hour))
	if err != nil || !cfg.OwnSince.Equal(first) {
		t.Fatalf("a toggle that stays on moved to %v, %v", cfg.OwnSince, err)
	}
	if cfg, err = Configure(ctx, fx.st, fx.repo, Change{AutoStartMine: &off}, first.Add(2*time.Hour)); err != nil || cfg.OwnOn() {
		t.Fatalf("the toggle did not go off: %+v, %v", cfg, err)
	}
	again := first.Add(3 * time.Hour)
	if cfg, err = Configure(ctx, fx.st, fx.repo, Change{AutoStartMine: &on}, again); err != nil || !cfg.OwnSince.Equal(again) {
		t.Fatalf("a toggle that went on again = %v, %v, want %v", cfg.OwnSince, err, again)
	}
	if !cfg.DependabotSince.Equal(first) {
		t.Fatalf("the other toggle moved to %v", cfg.DependabotSince)
	}
}

func TestATogglePointingAtACheckoutThatIsGoneIsRefused(t *testing.T) {
	fx := newFixture(t)
	ctx := context.Background()
	dir := gitCheckout(t, "git@github.com:acme/billing.git")
	on := true
	if _, err := Configure(ctx, fx.st, fx.repo, Change{CheckoutDir: &dir, AutoStartMine: &on}, fx.now); err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(dir); err != nil {
		t.Fatal(err)
	}
	if _, err := Configure(ctx, fx.st, fx.repo, Change{AutoWatchDependabot: &on}, fx.now); !errors.Is(err, ErrBadCheckout) {
		t.Fatalf("Configure() with a checkout that is gone = %v, want ErrBadCheckout", err)
	}
	off := false
	if _, err := Configure(ctx, fx.st, fx.repo, Change{AutoStartMine: &off}, fx.now); err != nil {
		t.Fatalf("turning the toggle off with a checkout that is gone = %v, want it to work", err)
	}
}

func TestAnUnknownModelIsRefused(t *testing.T) {
	fx := newFixture(t)
	for _, o := range []store.WatchOverrides{{Provider: "claude", Model: "gpt-9"}, {Model: "opus"}} {
		if _, err := Configure(context.Background(), fx.st, fx.repo, Change{Overrides: &o}, fx.now); !errors.Is(err, store.ErrInvalidRepoConfig) {
			t.Errorf("%+v: Configure() = %v, want ErrInvalidRepoConfig", o, err)
		}
	}
}

func TestAnEffortTheModelDoesNotTakeIsRefused(t *testing.T) {
	fx := newFixture(t)
	for _, o := range []store.WatchOverrides{{Provider: "claude", Model: "haiku", Effort: "high"}, {Effort: "high"}} {
		if _, err := Configure(context.Background(), fx.st, fx.repo, Change{Overrides: &o}, fx.now); !errors.Is(err, store.ErrInvalidRepoConfig) {
			t.Errorf("%+v: Configure() = %v, want ErrInvalidRepoConfig", o, err)
		}
	}
}

func TestTheAgentIsStoredWithTheIDsOfTheManifest(t *testing.T) {
	fx := newFixture(t)
	o := store.WatchOverrides{Provider: "claude", Model: " Opus ", Effort: " High "}
	cfg, err := Configure(context.Background(), fx.st, fx.repo, Change{Overrides: &o}, fx.now)
	if err != nil {
		t.Fatalf("Configure() = %v", err)
	}
	if cfg.Overrides.Model != "opus" || cfg.Overrides.Effort != "high" {
		t.Fatalf("overrides = %+v, want model opus and effort high", cfg.Overrides)
	}
}

func TestAnUnknownProviderIsRefused(t *testing.T) {
	fx := newFixture(t)
	for _, provider := range []string{"gemini", "self"} {
		o := store.WatchOverrides{Provider: provider}
		if _, err := Configure(context.Background(), fx.st, fx.repo, Change{Overrides: &o}, fx.now); !errors.Is(err, store.ErrInvalidRepoConfig) {
			t.Errorf("provider %q: Configure() = %v, want ErrInvalidRepoConfig", provider, err)
		}
	}
	for _, provider := range []string{"", "claude", "copilot"} {
		o := store.WatchOverrides{Provider: provider}
		if _, err := Configure(context.Background(), fx.st, fx.repo, Change{Overrides: &o}, fx.now); err != nil {
			t.Errorf("provider %q: Configure() = %v", provider, err)
		}
	}
}
