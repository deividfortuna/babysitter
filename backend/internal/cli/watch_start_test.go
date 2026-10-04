package cli

import (
	"maps"
	"reflect"
	"slices"
	"testing"
)

func TestWatchStartSendsTheFlagsThatWereTyped(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name  string
		flags []string
		want  map[string]any
	}{
		{"the rule of the base branch", []string{"--approvals", "branch"}, map[string]any{"approvalsRequired": nil}},
		{"a number of approvals", []string{"--approvals", "2"}, map[string]any{"approvalsRequired": float64(2)}},
		{"the worktree kept on copilot", []string{"--keep-worktree", "--provider", "copilot"}, map[string]any{"keepWorktree": true, "provider": "copilot"}},
		{"opus at max effort", []string{"--model", "opus", "--effort", "max"}, map[string]any{"model": "opus", "effort": "max"}},
		{"merge when ready", []string{"--merge-when-ready"}, map[string]any{"mergeWhenReady": true}},
		{"the approval mode", []string{"--approval-mode", "manual", "--auto-approve-rebase"}, map[string]any{"approvalMode": "manual", "autoApproveRebase": true}},
		{"the merge method", []string{"--merge-method", "rebase"}, map[string]any{"mergeMethod": "rebase"}},
		{"the items that already exist", []string{"--include-existing"}, map[string]any{"includeExisting": true}},
		{"my own comments", []string{"--include-own"}, map[string]any{"includeOwn": true}},
		{"the branch update", []string{"--branch-update", "merge", "--update-on-github=false"}, map[string]any{"branchUpdate": "merge", "updateOnGitHub": false}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			d := newFakeDaemon()

			if _, err := runWatch(t, d, append([]string{"start", "octo/hello#3"}, tc.flags...)...); err != nil {
				t.Fatalf("watch start error = %v", err)
			}
			body := d.starts[0]
			for key, want := range tc.want {
				sent, named := body[key]
				if !named {
					t.Fatalf("body = %v, want the %s field", body, key)
				}
				if !reflect.DeepEqual(sent, want) {
					t.Fatalf("%s = %v, want %v", key, sent, want)
				}
			}
		})
	}
}

func TestWatchStartLeavesEveryOptionalFieldOutWhenNobodyTypedIt(t *testing.T) {
	t.Parallel()
	d := newFakeDaemon()

	if _, err := runWatch(t, d, "start", "octo/hello#3"); err != nil {
		t.Fatalf("watch start error = %v", err)
	}
	fields := slices.Sorted(maps.Keys(d.starts[0]))
	if want := []string{"repo", "sourceDir", "target"}; !slices.Equal(fields, want) {
		t.Fatalf("fields = %v, want only %v so the repository, then the daemon, decides the rest", fields, want)
	}
}
