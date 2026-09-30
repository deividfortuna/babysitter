package httpd

import (
	"context"
	"net/http"
	"reflect"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/deividfortuna/babysitter/internal/store"
)

func newTestAPISettings(t *testing.T) (http.Handler, *store.Store, *fakeWatches, func() []store.Settings) {
	t.Helper()
	var (
		mu      sync.Mutex
		applied []store.Settings
	)
	h, st, fw := newTestAPIWith(t, context.Background(), func(d *Deps) {
		d.ApplySettings = func(s store.Settings) {
			mu.Lock()
			defer mu.Unlock()
			applied = append(applied, s)
		}
	})
	return h, st, fw, func() []store.Settings {
		mu.Lock()
		defer mu.Unlock()
		return append([]store.Settings(nil), applied...)
	}
}

func seedWatch(t *testing.T, st *store.Store) {
	t.Helper()
	if _, err := st.CreateWatch(t.Context(), store.Watch{Owner: "octo", Name: "hello", Number: 3, StartedAt: time.Now()}); err != nil {
		t.Fatal(err)
	}
}

func TestGetSettingsAnswersTheDefaultsOfAFreshDaemon(t *testing.T) {
	t.Parallel()
	h, _, _ := newTestAPI(t)

	var got Settings
	if rec := call(t, h, http.MethodGet, "/settings", "", &got); rec.Code != http.StatusOK {
		t.Fatalf("get settings: %d %s", rec.Code, rec.Body)
	}
	if got.PollIntervalSeconds != 60 || got.WatchIntervalSeconds != 180 || got.WatchMaxIntervalSeconds != 900 || got.CheckMaxIntervalSeconds != 900 {
		t.Fatalf("intervals = %d/%d/%d/%d, want 60/180/900/900", got.PollIntervalSeconds, got.WatchIntervalSeconds, got.WatchMaxIntervalSeconds, got.CheckMaxIntervalSeconds)
	}
	if got.ApprovalsRequired != nil {
		t.Fatalf("approvals = %d, want none so the rule of the base branch decides", *got.ApprovalsRequired)
	}
}

func TestPutSettingsStoresThemAndHandsThemToTheDaemon(t *testing.T) {
	t.Parallel()
	h, st, _, applied := newTestAPISettings(t)

	var got Settings
	rec := call(t, h, http.MethodPut, "/settings",
		`{"pollIntervalSeconds":120,"watchIntervalSeconds":45,"watchMaxIntervalSeconds":600,"checkMaxIntervalSeconds":1200,"approvalsRequired":2,"mergeMethod":"rebase","includeExisting":true,"includeOwn":true,"keepWorktree":true,"provider":"copilot","model":"gpt-5.6-terra","effort":"none","branchUpdate":"merge","updateOnGitHub":false,"screenReader":false}`, &got)
	if rec.Code != http.StatusOK {
		t.Fatalf("put settings: %d %s", rec.Code, rec.Body)
	}
	if got.PollIntervalSeconds != 120 || got.WatchIntervalSeconds != 45 || got.MergeMethod != "rebase" {
		t.Fatalf("answer = %+v, want what was sent", got)
	}

	stored, err := st.Settings(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if stored.PollInterval != 2*time.Minute || stored.WatchInterval != 45*time.Second || stored.WatchMaxInterval != 10*time.Minute || stored.CheckMaxInterval != 20*time.Minute || !stored.KeepWorktree ||
		stored.Provider != "copilot" || stored.Model != "gpt-5.6-terra" || stored.Effort != "none" ||
		stored.BranchUpdate != store.BranchMerge || stored.UpdateOnGitHub || stored.ScreenReader {
		t.Fatalf("stored = %+v, want what was sent", stored)
	}
	if len(applied()) != 1 || applied()[0].WatchInterval != 45*time.Second || applied()[0].WatchMaxInterval != 10*time.Minute || applied()[0].CheckMaxInterval != 20*time.Minute {
		t.Fatalf("the daemon was handed %+v, want the settings that were saved", applied())
	}
}

func TestPutSettingsStoresTheModelAndTheEffortWithTheIDsOfTheManifest(t *testing.T) {
	t.Parallel()
	h, st, _, _ := newTestAPISettings(t)

	rec := call(t, h, http.MethodPut, "/settings", `{"provider":"claude","model":" Opus ","effort":" High "}`, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("put settings: %d %s", rec.Code, rec.Body)
	}
	stored, err := st.Settings(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if stored.Model != "opus" || stored.Effort != "high" {
		t.Fatalf("stored model %q and effort %q, want opus and high", stored.Model, stored.Effort)
	}
}

func TestPutSettingsRejectsWhatTheDaemonCannotRun(t *testing.T) {
	t.Parallel()
	h, _, _, applied := newTestAPISettings(t)
	cases := map[string]string{
		"interval below the floor":  `{"pollIntervalSeconds":1,"watchIntervalSeconds":60,"mergeMethod":"","includeExisting":false,"includeOwn":false,"keepWorktree":false}`,
		"merge method unknown":      `{"pollIntervalSeconds":60,"watchIntervalSeconds":60,"mergeMethod":"fast-forward","includeExisting":false,"includeOwn":false,"keepWorktree":false}`,
		"approvals below zero":      `{"pollIntervalSeconds":60,"watchIntervalSeconds":60,"approvalsRequired":-1,"mergeMethod":"","includeExisting":false,"includeOwn":false,"keepWorktree":false}`,
		"longest below the watch":   `{"watchIntervalSeconds":600,"watchMaxIntervalSeconds":300}`,
		"body that is not JSON":     `not json`,
		"field the daemon has not":  `{"pollIntervalSeconds":60,"colour":"blue"}`,
		"provider of your session":  `{"provider":"self"}`,
		"model of another provider": `{"provider":"claude","model":"auto"}`,
		"effort the model lacks":    `{"provider":"claude","model":"haiku","effort":"high"}`,
		"branch update unknown":     `{"branchUpdate":"squash"}`,
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			if rec := call(t, h, http.MethodPut, "/settings", body, nil); rec.Code != http.StatusBadRequest {
				t.Fatalf("put settings: %d %s, want 400", rec.Code, rec.Body)
			}
		})
	}
	if len(applied()) != 0 {
		t.Fatalf("the daemon was handed %+v, want nothing for a rejected write", applied())
	}
}

func TestStartWatchHandsTheWatchServiceWhatTheBodySaid(t *testing.T) {
	t.Parallel()
	h, _, fw, _ := newTestAPISettings(t)

	if rec := call(t, h, http.MethodPost, "/watches", `{"target":"octo/hello#3","sourceDir":"/src"}`, nil); rec.Code != http.StatusCreated {
		t.Fatalf("start watch: %d %s", rec.Code, rec.Body)
	}
	got := fw.started()
	if len(got) != 1 {
		t.Fatalf("started %d watches, want 1", len(got))
	}
	if got[0].IncludeExisting != nil || got[0].IncludeOwn != nil || got[0].ApprovalsRequired.Set || got[0].MergeMethod != nil ||
		got[0].Provider != "" || got[0].KeepWorktree != nil {
		t.Fatalf("start request = %+v, want every field the body left out unset", got[0])
	}

	rec := call(t, h, http.MethodPost, "/watches",
		`{"target":"octo/hello#4","sourceDir":"/src","includeExisting":false,"includeOwn":true,"mergeMethod":"rebase","approvalsRequired":0,"keepWorktree":true,"effort":"high"}`, nil)
	if rec.Code != http.StatusCreated {
		t.Fatalf("start watch: %d %s", rec.Code, rec.Body)
	}
	got = fw.started()
	if got[1].IncludeExisting == nil || *got[1].IncludeExisting || got[1].IncludeOwn == nil || !*got[1].IncludeOwn ||
		got[1].KeepWorktree == nil || !*got[1].KeepWorktree || got[1].Effort != "high" {
		t.Fatalf("start request = %+v, want the false and the true the body set", got[1])
	}
	if got[1].ApprovalsRequired.Count == nil || *got[1].ApprovalsRequired.Count != 0 || got[1].MergeMethod == nil || *got[1].MergeMethod != "rebase" {
		t.Fatalf("start request = %+v, want what the body asked for", got[1])
	}

	rec = call(t, h, http.MethodPost, "/watches", `{"target":"octo/hello#6","sourceDir":"/src","approvalsRequired":null}`, nil)
	if rec.Code != http.StatusCreated {
		t.Fatalf("start watch asking for the rule of the base branch: %d %s", rec.Code, rec.Body)
	}
	if got = fw.started(); !got[2].ApprovalsRequired.Set || got[2].ApprovalsRequired.Count != nil {
		t.Fatalf("approvals = %v, want the rule of the base branch the body asked for", got[2].ApprovalsRequired)
	}
	if rec := call(t, h, http.MethodPost, "/watches", `{"target":"octo/hello#7","sourceDir":"/src","approvalsRequired":-1}`, nil); rec.Code != http.StatusBadRequest {
		t.Fatalf("start watch with -1 approvals: %d %s, want 400", rec.Code, rec.Body)
	}

	rec = call(t, h, http.MethodPost, "/watches", `{"target":"octo/hello#5","sourceDir":"/src","mergeMethod":""}`, nil)
	if rec.Code != http.StatusCreated {
		t.Fatalf("start watch: %d %s", rec.Code, rec.Body)
	}
	if got = fw.started(); got[3].MergeMethod == nil || *got[3].MergeMethod != "" {
		t.Fatalf("merge method = %v, want the repository default the body asked for", got[3].MergeMethod)
	}
}

func TestStopWatchHandsTheWatchServiceWhatTheBodySaid(t *testing.T) {
	t.Parallel()
	h, st, fw, _ := newTestAPISettings(t)
	seedWatch(t, st)

	if rec := call(t, h, http.MethodPost, "/watches/1/stop", "", nil); rec.Code != http.StatusOK {
		t.Fatalf("stop watch: %d %s", rec.Code, rec.Body)
	}
	if got := fw.stopped(); len(got) != 1 || got[0].KeepWorktree != nil {
		t.Fatalf("stop options = %+v, want the worktree left to the rule of the watch", got)
	}

	if rec := call(t, h, http.MethodPost, "/watches/1/stop", `{"keepWorktree":false}`, nil); rec.Code != http.StatusOK {
		t.Fatalf("stop watch: %d %s", rec.Code, rec.Body)
	}
	got := fw.stopped()
	if len(got) != 2 || got[1].KeepWorktree == nil || *got[1].KeepWorktree {
		t.Fatalf("stop options = %+v, want the worktree removed as the request asked", got)
	}
}

func TestPutSettingsMutesTheNotificationKindsTheBodyNames(t *testing.T) {
	t.Parallel()
	h, st, _, _ := newTestAPISettings(t)

	var got Settings
	rec := call(t, h, http.MethodPut, "/settings",
		`{"pollIntervalSeconds":60,"watchIntervalSeconds":180,"mergeMethod":"","includeExisting":false,"includeOwn":false,"keepWorktree":false,"notificationsEnabled":true,"mutedNotificationKinds":["checks","review"]}`, &got)
	if rec.Code != http.StatusOK {
		t.Fatalf("put settings: %d %s", rec.Code, rec.Body)
	}
	if !reflect.DeepEqual(got.MutedNotificationKinds, []string{"review", "checks"}) {
		t.Fatalf("answer muted %v, want the two kinds in the order of the schema", got.MutedNotificationKinds)
	}

	stored, err := st.Settings(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if stored.ShowsNotification(store.NotificationReview) || !stored.ShowsNotification(store.NotificationMerge) {
		t.Fatalf("stored = %+v, want the review muted and the merge shown", stored)
	}
}

func TestPutSettingsKeepsEveryFieldTheBodyLeavesOut(t *testing.T) {
	t.Parallel()
	h, st, _, _ := newTestAPISettings(t)

	if rec := call(t, h, http.MethodPut, "/settings", `{"watchIntervalSeconds":600}`, nil); rec.Code != http.StatusOK {
		t.Fatalf("put settings: %d %s", rec.Code, rec.Body)
	}

	stored, err := st.Settings(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if stored.WatchInterval != 10*time.Minute {
		t.Fatalf("stored = %+v, want the interval the body set", stored)
	}
	if stored.PollInterval != time.Minute {
		t.Fatalf("stored = %+v, want the interval the body left out kept", stored)
	}
}

func TestPutSettingsKeepsTheNotificationFieldsTheBodyLeavesOut(t *testing.T) {
	t.Parallel()
	h, st, _, _ := newTestAPISettings(t)

	body := `{"pollIntervalSeconds":60,"watchIntervalSeconds":180,"mergeMethod":"","includeExisting":false,"includeOwn":false,"keepWorktree":true}`
	if rec := call(t, h, http.MethodPut, "/settings", body, nil); rec.Code != http.StatusOK {
		t.Fatalf("put settings: %d %s", rec.Code, rec.Body)
	}

	stored, err := st.Settings(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if !stored.NotificationsEnabled || len(stored.SilentNotificationKinds) != 0 {
		t.Fatalf("stored = %+v, want the notifications and the sound left on", stored)
	}
	if !stored.KeepWorktree {
		t.Fatalf("stored = %+v, want the field the body did set", stored)
	}
}

func TestPutSettingsKeepsTheMutedKindsTheBodyLeavesOut(t *testing.T) {
	t.Parallel()
	h, st, _, _ := newTestAPISettings(t)

	muted := `{"pollIntervalSeconds":60,"watchIntervalSeconds":180,"mergeMethod":"","includeExisting":false,"includeOwn":false,"keepWorktree":false,"mutedNotificationKinds":["review"]}`
	if rec := call(t, h, http.MethodPut, "/settings", muted, nil); rec.Code != http.StatusOK {
		t.Fatalf("put settings: %d %s", rec.Code, rec.Body)
	}

	older := `{"pollIntervalSeconds":60,"watchIntervalSeconds":180,"mergeMethod":"","includeExisting":false,"includeOwn":false,"keepWorktree":false}`
	if rec := call(t, h, http.MethodPut, "/settings", older, nil); rec.Code != http.StatusOK {
		t.Fatalf("put settings: %d %s", rec.Code, rec.Body)
	}

	stored, err := st.Settings(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(stored.MutedNotificationKinds, store.NotificationReview) {
		t.Fatalf("stored = %+v, want the review still muted", stored)
	}
}

func TestPutSettingsTurnsTheNotificationsOffWhenTheBodySaysSo(t *testing.T) {
	t.Parallel()
	h, st, _, _ := newTestAPISettings(t)

	body := `{"pollIntervalSeconds":60,"watchIntervalSeconds":180,"mergeMethod":"","includeExisting":false,"includeOwn":false,"keepWorktree":false,"notificationsEnabled":false,"mutedNotificationKinds":[]}`
	if rec := call(t, h, http.MethodPut, "/settings", body, nil); rec.Code != http.StatusOK {
		t.Fatalf("put settings: %d %s", rec.Code, rec.Body)
	}

	stored, err := st.Settings(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if stored.NotificationsEnabled {
		t.Fatalf("stored = %+v, want the notifications off", stored)
	}
}

func TestPutSettingsTakesTheSilentKindsAndTheBackgroundSwitch(t *testing.T) {
	t.Parallel()
	h, st, _, _ := newTestAPISettings(t)

	var got Settings
	rec := call(t, h, http.MethodPut, "/settings", `{"silentNotificationKinds":["merge","agent"],"notificationsBackgroundOnly":true}`, &got)
	if rec.Code != http.StatusOK {
		t.Fatalf("put settings: %d %s", rec.Code, rec.Body)
	}
	if !reflect.DeepEqual(got.SilentNotificationKinds, []string{"agent", "merge"}) || !got.NotificationsBackgroundOnly {
		t.Fatalf("answer = %+v, want the two silent kinds in the order of the schema and the background switch on", got)
	}

	stored, err := st.Settings(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if stored.PlaysSound(store.NotificationMerge) || !stored.PlaysSound(store.NotificationReview) {
		t.Fatalf("stored = %+v, want the merge silent and the review with a sound", stored)
	}
}

func TestPutSettingsKeepsTheSilentKindsTheBodyLeavesOut(t *testing.T) {
	t.Parallel()
	h, st, _, _ := newTestAPISettings(t)

	if rec := call(t, h, http.MethodPut, "/settings", `{"silentNotificationKinds":["checks"],"notificationsBackgroundOnly":true}`, nil); rec.Code != http.StatusOK {
		t.Fatalf("put settings: %d %s", rec.Code, rec.Body)
	}
	if rec := call(t, h, http.MethodPut, "/settings", `{"watchIntervalSeconds":600}`, nil); rec.Code != http.StatusOK {
		t.Fatalf("put settings: %d %s", rec.Code, rec.Body)
	}

	stored, err := st.Settings(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(stored.SilentNotificationKinds, store.NotificationChecks) || !stored.NotificationsBackgroundOnly {
		t.Fatalf("stored = %+v, want the checks still silent and the background switch still on", stored)
	}
}

func TestPutSettingsRejectsAnUnknownSilentNotificationKind(t *testing.T) {
	t.Parallel()
	h, _, _, _ := newTestAPISettings(t)

	if rec := call(t, h, http.MethodPut, "/settings", `{"silentNotificationKinds":["rumour"]}`, nil); rec.Code != http.StatusBadRequest {
		t.Fatalf("put settings: %d %s, want 400", rec.Code, rec.Body)
	}
}

func TestPutSettingsRejectsAnUnknownNotificationKind(t *testing.T) {
	t.Parallel()
	h, _, _, _ := newTestAPISettings(t)

	body := `{"pollIntervalSeconds":60,"watchIntervalSeconds":180,"mergeMethod":"","includeExisting":false,"includeOwn":false,"keepWorktree":false,"mutedNotificationKinds":["rumour"]}`
	if rec := call(t, h, http.MethodPut, "/settings", body, nil); rec.Code != http.StatusBadRequest {
		t.Fatalf("put settings: %d %s, want 400", rec.Code, rec.Body)
	}
}

func TestGetSettingsAnswersAnEmptyListOfMutedKinds(t *testing.T) {
	t.Parallel()
	h, _, _ := newTestAPI(t)

	var got Settings
	if rec := call(t, h, http.MethodGet, "/settings", "", &got); rec.Code != http.StatusOK {
		t.Fatalf("get settings: %d %s", rec.Code, rec.Body)
	}
	if got.MutedNotificationKinds == nil {
		t.Fatal("muted kinds = null, want an empty list so the client never reads null")
	}
	if got.SilentNotificationKinds == nil {
		t.Fatal("silent kinds = null, want an empty list so the client never reads null")
	}
}
