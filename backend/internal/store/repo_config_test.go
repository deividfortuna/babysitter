package store

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/deividfortuna/babysitter/internal/dependabot"
)

func TestANewRepositoryHasAnEmptyConfiguration(t *testing.T) {
	t.Parallel()
	s, _ := openTemp(t)
	c, err := s.GetRepoConfig(context.Background(), repoID(t, s))
	if err != nil {
		t.Fatal(err)
	}
	if c.AutoStarts() || c.DependabotScope != dependabot.Patch || c.DependabotApproval != ApproveNever || c.DependabotLimit != 1 {
		t.Fatalf("GetRepoConfig() = %+v, want the empty configuration with patch, never and 1", c)
	}
}

func TestTheRepositoryConfigurationKeepsEachField(t *testing.T) {
	t.Parallel()
	s, _ := openTemp(t)
	ctx := context.Background()
	since := time.Date(2026, 9, 24, 10, 0, 0, 0, time.UTC)
	two, yes, no := 2, true, false
	want := RepoConfig{
		RepoID: repoID(t, s), CheckoutDir: "/code/hello", OwnSince: &since, IncludeDrafts: true, DependabotSince: &since,
		Overrides: WatchOverrides{
			Provider: "copilot", Model: "gpt-5", ApprovalMode: ApprovalAuto, MergeMethod: "squash",
			ApprovalsSet: true, Approvals: &two, IncludeExisting: &yes,
			AutoApproveRebase: &yes, IncludeOwn: &no, KeepWorktree: &yes,
		},
		DependabotScope: dependabot.Minor, DependabotApproval: ApproveGreen, DependabotLimit: 3,
	}
	if _, err := s.SaveRepoConfig(ctx, want); err != nil {
		t.Fatal(err)
	}
	got, err := s.GetRepoConfig(ctx, want.RepoID)
	if err != nil {
		t.Fatal(err)
	}
	if got.CheckoutDir != want.CheckoutDir || !got.OwnSince.Equal(since) || !got.DependabotSince.Equal(since) ||
		!got.IncludeDrafts || got.Overrides.Provider != "copilot" || got.Overrides.Model != "gpt-5" ||
		got.Overrides.ApprovalMode != ApprovalAuto || got.Overrides.MergeMethod != "squash" || !got.Overrides.ApprovalsSet ||
		*got.Overrides.Approvals != 2 || !*got.Overrides.IncludeExisting ||
		!*got.Overrides.AutoApproveRebase || *got.Overrides.IncludeOwn || !*got.Overrides.KeepWorktree ||
		got.DependabotScope != dependabot.Minor || got.DependabotApproval != ApproveGreen || got.DependabotLimit != 3 {
		t.Fatalf("GetRepoConfig() = %+v, want %+v", got, want)
	}
}

func TestAnEmptyOverrideStaysUnset(t *testing.T) {
	t.Parallel()
	s, _ := openTemp(t)
	ctx := context.Background()
	c, err := s.SaveRepoConfig(ctx, DefaultRepoConfig(repoID(t, s)))
	if err != nil {
		t.Fatal(err)
	}
	o := c.Overrides
	if o.IncludeExisting != nil || o.AutoApproveRebase != nil || o.IncludeOwn != nil || o.KeepWorktree != nil {
		t.Fatalf("Overrides = %+v, want each switch unset so the daemon decides", o)
	}
}

func TestTheRepositoryConfigurationRefusesBadValues(t *testing.T) {
	t.Parallel()
	s, _ := openTemp(t)
	now := time.Now()
	bad := map[string]func(*RepoConfig){
		"a toggle without a checkout": func(c *RepoConfig) { c.OwnSince = &now },
		"a limit of 0":                func(c *RepoConfig) { c.DependabotLimit = 0 },
		"an unknown scope":            func(c *RepoConfig) { c.DependabotScope = "huge" },
		"an unknown approval":         func(c *RepoConfig) { c.DependabotApproval = "always" },
		"an unknown merge method":     func(c *RepoConfig) { c.Overrides.MergeMethod = "octopus" },
	}
	for name, change := range bad {
		c := DefaultRepoConfig(repoID(t, s))
		change(&c)
		if _, err := s.SaveRepoConfig(context.Background(), c); !errors.Is(err, ErrInvalidRepoConfig) {
			t.Errorf("%s: SaveRepoConfig() = %v, want ErrInvalidRepoConfig", name, err)
		}
	}
}

func TestRemovingTheRepositoryRemovesItsConfigurationAndClaims(t *testing.T) {
	t.Parallel()
	s, _ := openTemp(t)
	ctx := context.Background()
	id := repoID(t, s)
	c := DefaultRepoConfig(id)
	c.CheckoutDir = "/code/hello"
	if _, err := s.SaveRepoConfig(ctx, c); err != nil {
		t.Fatal(err)
	}
	if err := s.ClaimAutoStart(ctx, id, 7, time.Now(), time.Now().Add(-time.Hour)); err != nil {
		t.Fatal(err)
	}
	if err := s.RemoveRepo(ctx, "octo", "hello"); err != nil {
		t.Fatal(err)
	}
	var rows int
	if err := s.db.QueryRowContext(ctx, "SELECT (SELECT COUNT(*) FROM repo_config) + (SELECT COUNT(*) FROM auto_start_claims)").Scan(&rows); err != nil {
		t.Fatal(err)
	}
	if rows != 0 {
		t.Fatalf("%d rows of configuration and claims remain, want 0", rows)
	}
}

func TestAPullRequestIsClaimedForAutoStartOnce(t *testing.T) {
	t.Parallel()
	s, _ := openTemp(t)
	ctx := context.Background()
	id := repoID(t, s)
	if err := s.ClaimAutoStart(ctx, id, 7, time.Now(), time.Now().Add(-time.Hour)); err != nil {
		t.Fatal(err)
	}
	if err := s.ClaimAutoStart(ctx, id, 7, time.Now(), time.Now().Add(-time.Hour)); !errors.Is(err, ErrAlreadyClaimed) {
		t.Fatalf("second ClaimAutoStart() = %v, want ErrAlreadyClaimed", err)
	}
	if err := s.ReleaseAutoStart(ctx, id, 7); err != nil {
		t.Fatal(err)
	}
	if err := s.ClaimAutoStart(ctx, id, 7, time.Now(), time.Now().Add(-time.Hour)); err != nil {
		t.Fatalf("ClaimAutoStart() after a release = %v", err)
	}
}

func TestHadWatchSeesAStoppedWatch(t *testing.T) {
	t.Parallel()
	s, _ := openTemp(t)
	ctx := context.Background()
	key := WatchKey{Owner: "octo", Name: "hello", Number: 9}
	if had, err := s.HadWatch(ctx, key); err != nil || had {
		t.Fatalf("HadWatch() before a watch = %v, %v", had, err)
	}
	w, err := s.CreateWatch(ctx, Watch{Owner: "octo", Name: "hello", Number: 9, StartedAt: time.Now()})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.StopWatch(ctx, w.ID, StopUser, nil, time.Now()); err != nil {
		t.Fatal(err)
	}
	if had, err := s.HadWatch(ctx, key); err != nil || !had {
		t.Fatalf("HadWatch() after a stopped watch = %v, %v, want true", had, err)
	}
}

func TestAPullRequestKeepsItsAssigneesForkAndUpdateType(t *testing.T) {
	t.Parallel()
	s, _ := openTemp(t)
	ctx := context.Background()
	now := time.Now()
	pr := PullRequest{
		RepoID: repoID(t, s), Number: 4, State: StateOpen, CreatedAt: now, UpdatedAt: now, SyncedAt: now,
		Assignees: []string{"Alice"}, Fork: true, UpdateType: dependabot.Minor,
	}
	if err := s.UpsertPR(ctx, pr); err != nil {
		t.Fatal(err)
	}
	prs, err := s.OpenPRs(ctx, pr.RepoID)
	if err != nil || len(prs) != 1 {
		t.Fatalf("OpenPRs() = %v, %v", prs, err)
	}
	got := prs[0]
	if !got.AssignedTo("alice") || !got.Fork || got.UpdateType != dependabot.Minor {
		t.Fatalf("OpenPRs()[0] = %+v, want assignee Alice, fork and minor", got)
	}
}

func TestAWatchKeepsItsAutoFieldsAndMergeWhenReady(t *testing.T) {
	t.Parallel()
	s, _ := openTemp(t)
	ctx := context.Background()
	w, err := s.CreateWatch(ctx, Watch{
		Owner: "octo", Name: "hello", Number: 5, StartedAt: time.Now(),
		AutoReason: AutoDependabot, UpdateType: dependabot.Patch, MergeWhenReady: true, KeepWorktree: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if w.AutoReason != AutoDependabot || w.UpdateType != dependabot.Patch || !w.MergeWhenReady || !w.KeepWorktree {
		t.Fatalf("CreateWatch() = %+v", w)
	}
	w, err = s.SetWatchMergeRules(ctx, w.ID, MergeRules{MergeMethod: "squash"})
	if err != nil || w.MergeWhenReady || w.MergeMethod != "squash" {
		t.Fatalf("SetWatchMergeRules() = %+v, %v, want merge when ready off", w, err)
	}
}

func TestANotificationKeepsItsAction(t *testing.T) {
	t.Parallel()
	s, _ := openTemp(t)
	ctx := context.Background()
	if _, err := s.AddNotification(ctx, Notification{Kind: NotificationAuto, Title: "t", Body: "b", Action: ActionApproveMerge}); err != nil {
		t.Fatal(err)
	}
	list, err := s.ListNotifications(ctx, ListNotificationsOptions{})
	if err != nil || len(list) != 1 || list[0].Action != ActionApproveMerge {
		t.Fatalf("ListNotifications() = %+v, %v, want the approve_merge action", list, err)
	}
	if _, err := s.AddNotification(ctx, Notification{Kind: NotificationAuto, Title: "t", Body: "b", Action: "explode"}); !errors.Is(err, ErrInvalidNotification) {
		t.Fatalf("AddNotification() with an unknown action = %v, want ErrInvalidNotification", err)
	}
}

func TestAStaleClaimCanBeTakenOverAndAFreshOneCannot(t *testing.T) {
	t.Parallel()
	s, _ := openTemp(t)
	ctx := context.Background()
	id := repoID(t, s)
	first := time.Date(2026, 9, 27, 10, 0, 0, 0, time.UTC)
	if err := s.ClaimAutoStart(ctx, id, 7, first, first.Add(-10*time.Minute)); err != nil {
		t.Fatal(err)
	}
	soon := first.Add(time.Minute)
	if err := s.ClaimAutoStart(ctx, id, 7, soon, soon.Add(-10*time.Minute)); !errors.Is(err, ErrAlreadyClaimed) {
		t.Fatalf("ClaimAutoStart() of a fresh claim = %v, want ErrAlreadyClaimed", err)
	}
	if claimed, err := s.AutoStartClaimed(ctx, id, 7, soon.Add(-10*time.Minute)); err != nil || !claimed {
		t.Fatalf("AutoStartClaimed() of a fresh claim = %v, %v, want true", claimed, err)
	}
	late := first.Add(time.Hour)
	if claimed, err := s.AutoStartClaimed(ctx, id, 7, late.Add(-10*time.Minute)); err != nil || claimed {
		t.Fatalf("AutoStartClaimed() of a stale claim = %v, %v, want false", claimed, err)
	}
	if err := s.ClaimAutoStart(ctx, id, 7, late, late.Add(-10*time.Minute)); err != nil {
		t.Fatalf("ClaimAutoStart() of a stale claim = %v, want the claim", err)
	}
}
