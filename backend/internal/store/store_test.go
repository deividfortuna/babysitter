package store

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/deividfortuna/babysitter/internal/checks"
	"github.com/deividfortuna/babysitter/internal/events"
)

func openTemp(t *testing.T) (*Store, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "sub", "babysitter.db")
	s, err := Open(path)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s, path
}

func TestOpenMigratesAndReopens(t *testing.T) {
	t.Parallel()
	s, path := openTemp(t)

	var version int
	if err := s.db.QueryRow("PRAGMA user_version").Scan(&version); err != nil {
		t.Fatal(err)
	}
	if version != len(migrations) {
		t.Fatalf("user_version = %d, want %d", version, len(migrations))
	}
	var mode string
	if err := s.db.QueryRow("PRAGMA journal_mode").Scan(&mode); err != nil {
		t.Fatal(err)
	}
	if mode != "wal" {
		t.Fatalf("journal_mode = %q, want wal", mode)
	}

	s2, err := Open(path)
	if err != nil {
		t.Fatalf("second Open() error = %v", err)
	}
	s2.Close()
}

func TestOpenKeepsTheDatabaseDirPrivate(t *testing.T) {
	t.Parallel()
	if runtime.GOOS == "windows" {
		t.Skip("Windows has no permission bits")
	}
	_, path := openTemp(t)

	info, err := os.Stat(filepath.Dir(path))
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0o700 {
		t.Fatalf("database dir mode = %o, want 700", got)
	}
}

func TestRepos(t *testing.T) {
	t.Parallel()
	s, _ := openTemp(t)
	ctx := context.Background()

	r, err := s.AddRepo(ctx, "octo", "hello")
	if err != nil {
		t.Fatalf("AddRepo() error = %v", err)
	}
	if r.ID == 0 || r.FullName() != "octo/hello" {
		t.Fatalf("unexpected repo %+v", r)
	}
	if _, err := s.AddRepo(ctx, "octo", "hello"); !errors.Is(err, ErrRepoExists) {
		t.Fatalf("duplicate AddRepo() error = %v, want ErrRepoExists", err)
	}
	if _, err := s.AddRepo(ctx, "abc", "zzz"); err != nil {
		t.Fatal(err)
	}

	repos, err := s.ListRepos(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(repos) != 2 || repos[0].FullName() != "abc/zzz" || repos[1].FullName() != "octo/hello" {
		t.Fatalf("ListRepos() = %+v", repos)
	}
	if repos[0].LastSyncedAt != nil {
		t.Fatalf("LastSyncedAt = %v, want nil", repos[0].LastSyncedAt)
	}

	at := time.Date(2026, 9, 7, 10, 0, 0, 0, time.UTC)
	if err := s.SetRepoSync(ctx, r.ID, at, errors.New("boom")); err != nil {
		t.Fatal(err)
	}
	got, err := s.GetRepo(ctx, "octo", "hello")
	if err != nil {
		t.Fatal(err)
	}
	if got.LastSyncedAt == nil || !got.LastSyncedAt.Equal(at) || got.LastError != "boom" {
		t.Fatalf("GetRepo() = %+v", got)
	}
	if err := s.SetRepoSync(ctx, r.ID, at, nil); err != nil {
		t.Fatal(err)
	}
	got, _ = s.GetRepo(ctx, "octo", "hello")
	if got.LastError != "" {
		t.Fatalf("LastError = %q, want empty", got.LastError)
	}

	if err := s.RemoveRepo(ctx, "octo", "hello"); err != nil {
		t.Fatal(err)
	}
	if err := s.RemoveRepo(ctx, "octo", "hello"); !errors.Is(err, ErrRepoNotFound) {
		t.Fatalf("RemoveRepo() error = %v, want ErrRepoNotFound", err)
	}
	if _, err := s.GetRepo(ctx, "octo", "hello"); !errors.Is(err, ErrRepoNotFound) {
		t.Fatalf("GetRepo() error = %v, want ErrRepoNotFound", err)
	}
}

func samplePR(repoID int64, number int) PullRequest {
	now := time.Date(2026, 9, 7, 12, 0, 0, 0, time.UTC)
	return PullRequest{
		RepoID:             repoID,
		Number:             number,
		GitHubID:           int64(1000 + number),
		Title:              "Add thing",
		Author:             "alice",
		AuthorAvatarURL:    "https://avatars.githubusercontent.com/u/1",
		State:              StateOpen,
		BaseRef:            "main",
		HeadRef:            "feature",
		HeadSHA:            "abc",
		HTMLURL:            "https://github.com/octo/hello/pull/1",
		CreatedAt:          now.Add(-time.Hour),
		UpdatedAt:          now,
		MergeableState:     "clean",
		ReviewDecision:     ReviewRequired,
		RequestedReviewers: []string{"bob"},
		Labels:             []string{"bug", "p1"},
		Additions:          10,
		Deletions:          2,
		CIStatus:           checks.CIPending,
		SyncedAt:           now,
	}
}

func TestPullRequests(t *testing.T) {
	t.Parallel()
	s, _ := openTemp(t)
	ctx := context.Background()

	r1, _ := s.AddRepo(ctx, "octo", "hello")
	r2, _ := s.AddRepo(ctx, "octo", "world")

	pr := samplePR(r1.ID, 1)
	if err := s.UpsertPR(ctx, pr); err != nil {
		t.Fatalf("UpsertPR() error = %v", err)
	}
	if err := s.UpsertPR(ctx, samplePR(r2.ID, 7)); err != nil {
		t.Fatal(err)
	}

	open, err := s.OpenPRs(ctx, r1.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(open) != 1 {
		t.Fatalf("OpenPRs() len = %d, want 1", len(open))
	}
	got := open[0]
	if got.RepoFullName != "octo/hello" || got.Title != "Add thing" || got.AuthorAvatarURL != pr.AuthorAvatarURL || got.MergeableState != "clean" ||
		got.ReviewDecision != ReviewRequired || got.CIStatus != checks.CIPending || got.Additions != 10 ||
		!got.UpdatedAt.Equal(pr.UpdatedAt) || got.MergedAt != nil {
		t.Fatalf("unexpected PR %+v", got)
	}
	if len(got.RequestedReviewers) != 1 || got.RequestedReviewers[0] != "bob" {
		t.Fatalf("RequestedReviewers = %v", got.RequestedReviewers)
	}
	if len(got.Labels) != 2 || got.Labels[1] != "p1" {
		t.Fatalf("Labels = %v", got.Labels)
	}

	merged := pr.UpdatedAt.Add(time.Minute)
	pr.State = StateMerged
	pr.MergedAt = &merged
	pr.CIStatus = checks.CISuccess
	pr.RequestedReviewers = nil
	if err := s.UpsertPR(ctx, pr); err != nil {
		t.Fatal(err)
	}
	open, _ = s.OpenPRs(ctx, r1.ID)
	if len(open) != 0 {
		t.Fatalf("OpenPRs() after merge len = %d, want 0", len(open))
	}

	all, err := s.ListPRs(ctx, ListPRsOptions{State: "all"})
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 2 {
		t.Fatalf("ListPRs(all) len = %d, want 2", len(all))
	}
	if all[0].State != StateMerged || all[0].MergedAt == nil || !all[0].MergedAt.Equal(merged) || all[0].CIStatus != checks.CISuccess {
		t.Fatalf("merged PR = %+v", all[0])
	}
	if all[0].RequestedReviewers == nil || len(all[0].RequestedReviewers) != 0 {
		t.Fatalf("RequestedReviewers = %#v, want empty slice", all[0].RequestedReviewers)
	}

	openOnly, _ := s.ListPRs(ctx, ListPRsOptions{})
	if len(openOnly) != 1 || openOnly[0].RepoFullName != "octo/world" {
		t.Fatalf("ListPRs(open) = %+v", openOnly)
	}
	byRepo, _ := s.ListPRs(ctx, ListPRsOptions{Owner: "octo", Name: "hello", State: "all"})
	if len(byRepo) != 1 || byRepo[0].Number != 1 {
		t.Fatalf("ListPRs(octo/hello) = %+v", byRepo)
	}

	if err := s.RemoveRepo(ctx, "octo", "hello"); err != nil {
		t.Fatal(err)
	}
	all, _ = s.ListPRs(ctx, ListPRsOptions{State: "all"})
	if len(all) != 1 || all[0].RepoFullName != "octo/world" {
		t.Fatalf("after cascade ListPRs = %+v", all)
	}
}

func TestParseFullName(t *testing.T) {
	t.Parallel()
	cases := map[string][2]string{
		"a/b":                                  {"a", "b"},
		" a/b ":                                {"a", "b"},
		"https://github.com/a/b":               {"a", "b"},
		"https://github.com/a/b.git":           {"a", "b"},
		"github.com/a/b/":                      {"a", "b"},
		"git@github.com:a/b.git":               {"a", "b"},
		"ssh://git@github.com/a/b":             {"a", "b"},
		"ssh://git@github.com:22/a/b.git":      {"a", "b"},
		"ssh://git@ssh.github.com:443/a/b.git": {"a", "b"},
		"github.com:a/b.git":                   {"a", "b"},
		"https://x-access-token:ghs_abc123@github.com/a/b.git": {"a", "b"},
		"https://GitHub.com/a/b":                               {"a", "b"},
	}
	for in, want := range cases {
		owner, name, err := ParseFullName(in)
		if err != nil {
			t.Errorf("ParseFullName(%q) error = %v", in, err)
			continue
		}
		if owner != want[0] || name != want[1] {
			t.Errorf("ParseFullName(%q) = %q/%q, want %q/%q", in, owner, name, want[0], want[1])
		}
	}
	for _, in := range []string{
		"", "a", "/b", "a/", "a/b/c",
		"https://github.com/a",
		"git@gitlab.com:a/b.git",
		"git@github-work:a/b.git",
		"https://gitlab.com/a/b",
		"ssh://git@ghe.example.com/a/b",
	} {
		if _, _, err := ParseFullName(in); err == nil {
			t.Errorf("ParseFullName(%q) expected error", in)
		}
	}
	for _, in := range []string{"https://x-access-token:ghs_secret@github.com/a", "https://x:ghs_secret@gitlab.com/a/b", "https://x:ghs_se cret@github.com/a/b"} {
		_, _, err := ParseFullName(in)
		if err == nil {
			t.Errorf("ParseFullName(%q) expected error", in)
			continue
		}
		if strings.Contains(err.Error(), "ghs_se") {
			t.Errorf("ParseFullName(%q) error echoes the credentials: %v", in, err)
		}
	}
}

func TestRemoveRepoByID(t *testing.T) {
	t.Parallel()
	s, _ := openTemp(t)
	ctx := context.Background()
	pub := &recordingPublisher{}
	s.SetPublisher(pub)

	hello, err := s.AddRepo(ctx, "octo", "hello")
	if err != nil {
		t.Fatal(err)
	}
	world, err := s.AddRepo(ctx, "octo", "world")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.UpsertPR(ctx, samplePR(hello.ID, 1)); err != nil {
		t.Fatal(err)
	}
	if err := s.UpsertPR(ctx, samplePR(world.ID, 2)); err != nil {
		t.Fatal(err)
	}

	if err := s.RemoveRepoByID(ctx, hello.ID+world.ID+1); !errors.Is(err, ErrRepoNotFound) {
		t.Fatalf("RemoveRepoByID(unknown) error = %v, want ErrRepoNotFound", err)
	}
	if err := s.RemoveRepoByID(ctx, hello.ID); err != nil {
		t.Fatalf("RemoveRepoByID() error = %v", err)
	}
	if err := s.RemoveRepoByID(ctx, hello.ID); !errors.Is(err, ErrRepoNotFound) {
		t.Fatalf("second RemoveRepoByID() error = %v, want ErrRepoNotFound", err)
	}
	if _, err := s.GetRepo(ctx, "octo", "hello"); !errors.Is(err, ErrRepoNotFound) {
		t.Fatalf("GetRepo() error = %v, want ErrRepoNotFound", err)
	}

	all, err := s.ListPRs(ctx, ListPRsOptions{State: "all"})
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 1 || all[0].RepoFullName != "octo/world" {
		t.Fatalf("after cascade ListPRs = %+v", all)
	}

	want := []events.Type{events.RepoAdded, events.RepoAdded, events.PullUpdated, events.PullUpdated, events.RepoRemoved}
	if !slices.Equal(pub.types, want) {
		t.Fatalf("events = %v, want %v", pub.types, want)
	}
	if got := pub.last(); got.Repo != "octo/hello" || got.Number != 0 {
		t.Fatalf("removal event = %+v, want octo/hello", got)
	}
}
