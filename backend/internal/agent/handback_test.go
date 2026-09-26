package agent

import (
	"strings"
	"testing"
)

func TestTheHandbackMessageListsTheWorkOfTheAuthor(t *testing.T) {
	t.Parallel()
	msg, err := HandbackMessage(Handback{
		PR: pr, WorkBranch: "babysitter/fix", Work: "9a1c3e2",
		Commits: []Commit{{SHA: "5d0b7f1", Subject: "Start the webhook sink in the retry test"}, {SHA: "9a1c3e2", Subject: "Wait for the sink"}},
		Files:   []string{" M internal/webhook/deliver_test.go"},
	})
	if err != nil {
		t.Fatalf("HandbackMessage() error = %v", err)
	}
	for _, want := range []string{
		"The author gave the session of PR #3 (fix -> main) back to you",
		"babysitter/fix stands at 9a1c3e2",
		"2 commits that the pull request branch does not have",
		"5d0b7f1 Start the webhook sink in the retry test",
		"9a1c3e2 Wait for the sink",
		"The daemon pushes them with the work of your next turn",
		"1 change that is not committed",
		" M internal/webhook/deliver_test.go",
		"Read what the author did before you go on",
		"PR: https://github.com/octo/hello/pull/3",
	} {
		if !strings.Contains(msg, want) {
			t.Errorf("hand-back message lacks %q:\n%s", want, msg)
		}
	}
}

func TestTheHandbackMessageOfAnAuthorWhoChangedNothing(t *testing.T) {
	t.Parallel()
	msg, err := HandbackMessage(Handback{PR: pr, WorkBranch: "babysitter/fix", Work: "abc"})
	if err != nil {
		t.Fatalf("HandbackMessage() error = %v", err)
	}
	if !strings.Contains(msg, "no commits that the pull request branch does not have") || strings.Contains(msg, "not committed") {
		t.Fatalf("hand-back message:\n%s", msg)
	}
}
