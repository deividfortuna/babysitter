package prwatch

import (
	"context"
	"errors"
	"net/http"
	"slices"
	"strings"
	"testing"

	"github.com/deividfortuna/babysitter/internal/dependabot"
	"github.com/deividfortuna/babysitter/internal/ghclient/ghfake"
	"github.com/deividfortuna/babysitter/internal/notify"
	"github.com/deividfortuna/babysitter/internal/store"
)

func (fx *fixture) notificationKinds() []store.NotificationKind {
	fx.notes.mu.Lock()
	defer fx.notes.mu.Unlock()
	out := make([]store.NotificationKind, 0, len(fx.notes.items))
	for _, item := range fx.notes.items {
		out = append(out, item.Kind)
	}
	return out
}

func TestAnAutoStartRecordsWhyAndNotifiesOnce(t *testing.T) {
	t.Parallel()
	fx := newFixture(t)
	req := fx.startRequest()
	req.AutoReason = store.AutoMine
	w, err := fx.svc.Start(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if w.AutoReason != store.AutoMine {
		t.Fatalf("AutoReason = %q, want mine", w.AutoReason)
	}
	rows := fx.activity(w)
	i := slices.IndexFunc(rows, func(a store.Activity) bool { return a.Kind == store.ActivityAutoStarted })
	if i < 0 || !strings.Contains(rows[i].Summary, "you opened it") {
		t.Fatalf("activity = %v, want an auto_started row that says you opened it", fx.kinds(w))
	}
	kinds := fx.notificationKinds()
	if !slices.Contains(kinds, store.NotificationAuto) || slices.Contains(kinds, store.NotificationWatch) {
		t.Fatalf("notifications = %v, want auto and no watch", kinds)
	}
}

func TestAStartByHandRecordsNoAutoStart(t *testing.T) {
	t.Parallel()
	fx := newFixture(t)
	w := fx.start()
	if slices.Contains(fx.kinds(w), string(store.ActivityAutoStarted)) {
		t.Fatalf("activity = %v, want no auto_started row", fx.kinds(w))
	}
	if kinds := fx.notificationKinds(); slices.Contains(kinds, store.NotificationAuto) {
		t.Fatalf("notifications = %v, want no auto", kinds)
	}
}

func TestADependabotAutoStartKeepsTheUpdateTypeAndMergeWhenReady(t *testing.T) {
	t.Parallel()
	fx := newFixture(t)
	req := fx.startRequest()
	on := true
	req.AutoReason, req.UpdateType, req.MergeWhenReady = store.AutoDependabot, dependabot.Patch, &on
	w, err := fx.svc.Start(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if w.UpdateType != dependabot.Patch || !w.MergeWhenReady {
		t.Fatalf("watch = update %q, merge when ready %v, want patch and on", w.UpdateType, w.MergeWhenReady)
	}
}

const groupedBody = "Updates `a` from 1.2.0 to 1.2.1\nUpdates `b` from 1.2.0 to 2.0.0\n"

func (fx *fixture) dependabotPR(title, body string) {
	fx.update(func() {
		fx.pr.Author, fx.pr.Title, fx.pr.Body = "dependabot[bot]", title, body
		fx.pr.Reviews = nil
	})
}

func (fx *fixture) policy(scope dependabot.Level, approval store.DependabotApproval) {
	fx.t.Helper()
	ctx := context.Background()
	repo, err := fx.st.AddRepo(ctx, "octo", "hello")
	if err != nil {
		fx.t.Fatal(err)
	}
	cfg := store.DefaultRepoConfig(repo.ID)
	cfg.DependabotScope, cfg.DependabotApproval = scope, approval
	if _, err := fx.st.SaveRepoConfig(ctx, cfg); err != nil {
		fx.t.Fatal(err)
	}
}

func (fx *fixture) startAuto(reason store.AutoReason, mergeWhenReady bool) store.Watch {
	fx.t.Helper()
	req := fx.startRequest()
	req.AutoReason, req.MergeWhenReady = reason, &mergeWhenReady
	w, err := fx.svc.Start(context.Background(), req)
	if err != nil {
		fx.t.Fatalf("Start() error = %v", err)
	}
	return w
}

func (fx *fixture) reviews() []ghfake.Review {
	var out []ghfake.Review
	fx.update(func() { out = slices.Clone(fx.pr.Reviews) })
	return out
}

func (fx *fixture) notification(kind store.NotificationKind, text string) (notify.Item, bool) {
	fx.notes.mu.Lock()
	defer fx.notes.mu.Unlock()
	for _, item := range fx.notes.items {
		if item.Kind == kind && strings.Contains(item.Message, text) {
			return item, true
		}
	}
	return notify.Item{}, false
}

func (fx *fixture) pollUntilStopped(w store.Watch, polls int) store.Watch {
	fx.t.Helper()
	for range polls {
		if got := fx.watch(w); got.Status == store.WatchStopped {
			return got
		}
		fx.poll(w)
	}
	return fx.watch(w)
}

func TestAGreenPatchIsApprovedOnceAndMergesOnItsOwn(t *testing.T) {
	t.Parallel()
	fx := newFixture(t)
	fx.dependabotPR("Bump lib from 1.2.0 to 1.2.1", "")
	fx.policy(dependabot.Patch, store.ApproveGreen)
	w := fx.startAuto(store.AutoDependabot, true)
	fx.agentIdle(w)

	got := fx.pollUntilStopped(w, 5)

	if got.Status != store.WatchStopped || got.StopReason != store.StopMerged {
		t.Fatalf("watch = %s (%s), want stopped by the merge; kinds %v", got.Status, got.StopReason, fx.kinds(w))
	}
	reviews := fx.reviews()
	if len(reviews) != 1 || reviews[0].State != "APPROVED" || reviews[0].CommitID != "abc" || !strings.Contains(reviews[0].Body, "Dependabot policy of babysitter") {
		t.Fatalf("reviews = %+v, want one approval of abc that names the policy", reviews)
	}
	if merges := fx.merges(); len(merges) != 1 || merges[0].SHA != "abc" {
		t.Fatalf("merges = %+v, want one of abc", merges)
	}
	if rows := fx.activity(w); !slices.ContainsFunc(rows, func(a store.Activity) bool {
		return a.Kind == store.ActivityMergeReady && strings.HasSuffix(a.Summary, "merge when ready merges it now")
	}) {
		t.Errorf("the merge_ready row asks the author to merge: %v", rows)
	}
	kinds := fx.kinds(w)
	for _, want := range []store.ActivityKind{store.ActivityAutoStarted, store.ActivityApproved, store.ActivityMergeReady, store.ActivityMerged} {
		if !slices.Contains(kinds, string(want)) {
			t.Errorf("kinds = %v, want %s", kinds, want)
		}
	}
	if _, ok := fx.notification(store.NotificationMerge, "merge when ready"); !ok {
		t.Errorf("notifications = %v, want a merge one that names merge when ready", fx.notes.messages())
	}
}

func TestAGroupedUpdateWithAMajorWaitsAndIsNotApproved(t *testing.T) {
	t.Parallel()
	fx := newFixture(t)
	fx.dependabotPR("Bump the go group with 2 updates", groupedBody)
	fx.policy(dependabot.Minor, store.ApproveGreen)
	req := fx.startRequest()
	req.AutoReason = store.AutoDependabot
	w, err := fx.svc.Start(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if w.UpdateType != dependabot.Major || w.MergeWhenReady {
		t.Fatalf("watch = update %q, merge when ready %v, want major and off", w.UpdateType, w.MergeWhenReady)
	}
	fx.agentIdle(w)
	fx.update(func() { fx.pr.Reviews = []ghfake.Review{approvalFromBob} })
	got := fx.pollUntilStopped(w, 4)

	if got.Status != store.WatchActive || got.ReadySince == nil {
		t.Fatalf("watch = %s, ready since %v, want active and ready to merge", got.Status, got.ReadySince)
	}
	if reviews := fx.reviews(); len(reviews) != 1 {
		t.Fatalf("reviews = %+v, want only the approval of bob", reviews)
	}
	if merges := fx.merges(); len(merges) != 0 {
		t.Fatalf("merges = %+v, want none", merges)
	}
}

func TestAskNotifiesWithApproveAndMergeAndTheMergeApproves(t *testing.T) {
	t.Parallel()
	fx := newFixture(t)
	fx.dependabotPR("Bump golang.org/x/net from 0.33.0 to 0.34.0", "")
	fx.policy(dependabot.Minor, store.ApproveAsk)
	w := fx.startAuto(store.AutoDependabot, true)
	fx.agentIdle(w)
	fx.poll(w)
	fx.poll(w)

	item, ok := fx.notification(store.NotificationAuto, "waits on your review")
	if !ok || item.Action != store.ActionApproveMerge || item.WatchID != w.ID {
		t.Fatalf("notification = %+v, want one with the approve_merge action", item)
	}
	if n := strings.Count(strings.Join(fx.kinds(w), ","), string(store.ActivityApprovalAsked)); n != 1 {
		t.Fatalf("kinds = %v, want one approval_asked", fx.kinds(w))
	}
	if reviews := fx.reviews(); len(reviews) != 0 {
		t.Fatalf("reviews = %+v, want none before the author asks", reviews)
	}

	got, err := fx.svc.Merge(context.Background(), w.ID, MergeOptions{Approve: true})
	if err != nil {
		t.Fatalf("Merge(approve) error = %v", err)
	}
	if got.Status != store.WatchStopped || len(fx.reviews()) != 1 || len(fx.merges()) != 1 {
		t.Fatalf("watch = %s, reviews %d, merges %d, want merged with one approval", got.Status, len(fx.reviews()), len(fx.merges()))
	}
}

func TestTheDaemonNeverApprovesOutsideTheScopeOrAnotherAuthor(t *testing.T) {
	t.Parallel()
	fx := newFixture(t)
	fx.policy(dependabot.Patch, store.ApproveGreen)
	w := fx.start()
	if _, err := fx.svc.Merge(context.Background(), w.ID, MergeOptions{Approve: true}); !errors.Is(err, ErrNotDependabot) {
		t.Fatalf("Merge(approve) of a pull request of alice = %v, want ErrNotDependabot", err)
	}

	major := newFixture(t)
	major.dependabotPR("Bump react from 18.3.1 to 19.0.0", "")
	major.policy(dependabot.Patch, store.ApproveGreen)
	mw := major.startAuto(store.AutoDependabot, false)
	major.agentIdle(mw)
	major.poll(mw)
	major.poll(mw)
	if _, err := major.svc.Merge(context.Background(), mw.ID, MergeOptions{Approve: true}); !errors.Is(err, ErrOutOfScope) {
		t.Fatalf("Merge(approve) of a major = %v, want ErrOutOfScope", err)
	}
	if reviews := major.reviews(); len(reviews) != 0 {
		t.Fatalf("reviews = %+v, want none", reviews)
	}
}

func TestAMergeWhenReadyThatFailsTriesOnceForEachHead(t *testing.T) {
	t.Parallel()
	fx := newFixture(t)
	fx.good()
	fx.update(func() { fx.pr.RefuseMerge = http.StatusMethodNotAllowed })
	w := fx.startAuto(store.AutoNone, true)
	fx.agentIdle(w)
	for range 5 {
		fx.poll(w)
	}
	if n := len(fx.merges()); n != 1 {
		t.Fatalf("merge calls = %d, want 1 for the head abc", n)
	}
	if n := strings.Count(strings.Join(fx.kinds(w), ","), string(store.ActivityMergeFailed)); n != 1 {
		t.Fatalf("kinds = %v, want one merge_failed", fx.kinds(w))
	}
	if _, ok := fx.notification(store.NotificationMerge, "refused"); !ok {
		t.Fatal("no merge notification for the failure")
	}

	fx.update(func() { fx.pr.HeadSHA = "def" })
	fx.agentIdle(w)
	for range 4 {
		fx.poll(w)
		fx.agentIdle(w)
	}
	if n := len(fx.merges()); n != 2 {
		t.Fatalf("merge calls = %d, want a second one for the new head", n)
	}
}

func TestMergeWhenReadyOffWaitsForTheAuthor(t *testing.T) {
	t.Parallel()
	fx := newFixture(t)
	fx.good()
	w := fx.start()
	fx.agentIdle(w)
	for range 4 {
		fx.poll(w)
	}
	if got := fx.watch(w); got.Status != store.WatchActive || len(fx.merges()) != 0 {
		t.Fatalf("watch = %s with %d merges, want active and no merge", got.Status, len(fx.merges()))
	}
}

func TestMergeRulesTurnMergeWhenReadyOn(t *testing.T) {
	t.Parallel()
	fx := newFixture(t)
	fx.good()
	w := fx.start()
	on := true
	got, err := fx.svc.SetMergeRules(context.Background(), w.ID, MergeRulesChange{MergeWhenReady: &on})
	if err != nil || !got.MergeWhenReady {
		t.Fatalf("SetMergeRules() = %v, %v, want merge when ready on", got.MergeWhenReady, err)
	}
	fx.agentIdle(w)
	if stopped := fx.pollUntilStopped(w, 4); stopped.StopReason != store.StopMerged {
		t.Fatalf("watch = %s (%s), want merged", stopped.Status, stopped.StopReason)
	}
}

func TestApproveAndMergeApprovesOnceForEachHeadWhenTheMergeWaits(t *testing.T) {
	t.Parallel()
	fx := newFixture(t)
	fx.dependabotPR("Bump golang.org/x/net from 0.33.0 to 0.34.0", "")
	fx.policy(dependabot.Minor, store.ApproveAsk)
	w := fx.startAuto(store.AutoDependabot, false)

	if _, err := fx.svc.Merge(context.Background(), w.ID, MergeOptions{Approve: true}); !errors.Is(err, ErrNotReady) {
		t.Fatalf("Merge(approve) while the agent works = %v, want ErrNotReady", err)
	}
	fx.agentIdle(w)
	if _, err := fx.svc.Merge(context.Background(), w.ID, MergeOptions{Approve: true}); err != nil {
		t.Fatalf("second Merge(approve) = %v", err)
	}
	if reviews := fx.reviews(); len(reviews) != 1 {
		t.Fatalf("reviews = %+v, want one approval for the head", reviews)
	}
	if merges := fx.merges(); len(merges) != 1 {
		t.Fatalf("merges = %+v, want one", merges)
	}
}

func TestApproveUsesTheUpdateTypeOfThePullRequestNow(t *testing.T) {
	t.Parallel()
	fx := newFixture(t)
	fx.dependabotPR("Bump lib from 1.2.0 to 1.2.1", "")
	fx.policy(dependabot.Patch, store.ApproveAsk)
	w := fx.startAuto(store.AutoDependabot, true)
	fx.agentIdle(w)
	fx.update(func() { fx.pr.Title = "Bump lib from 1.2.0 to 2.0.0" })

	if _, err := fx.svc.Merge(context.Background(), w.ID, MergeOptions{Approve: true}); !errors.Is(err, ErrOutOfScope) {
		t.Fatalf("Merge(approve) after the update became a major = %v, want ErrOutOfScope", err)
	}
	if reviews := fx.reviews(); len(reviews) != 0 {
		t.Fatalf("reviews = %+v, want none", reviews)
	}
}

func TestMergeWhenReadyStopsWhenTheUpdateLeavesTheScope(t *testing.T) {
	t.Parallel()
	fx := newFixture(t)
	fx.dependabotPR("Bump lib from 1.2.0 to 1.2.1", "")
	fx.policy(dependabot.Patch, store.ApproveNever)
	w := fx.startAuto(store.AutoDependabot, true)
	fx.agentIdle(w)
	fx.update(func() {
		fx.pr.Title = "Bump lib from 1.2.0 to 2.0.0"
		fx.pr.Reviews = []ghfake.Review{approvalFromBob}
	})
	for range 4 {
		fx.poll(w)
	}
	if merges := fx.merges(); len(merges) != 0 {
		t.Fatalf("merges = %+v, want none for a major outside the patch scope", merges)
	}
	got := fx.watch(w)
	if got.UpdateType != dependabot.Major || got.MergeWhenReady {
		t.Fatalf("watch = update %q, merge when ready %v, want major and off", got.UpdateType, got.MergeWhenReady)
	}
}
