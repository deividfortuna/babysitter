package copilot

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func TestTheCommandOfTheAuthorContinuesTheConversationWithNoRules(t *testing.T) {
	t.Parallel()
	c := New("", "gpt-5")
	l := launch(t)
	l.Resume = true

	argv, err := c.AuthorCommand(l)
	if err != nil {
		t.Fatalf("AuthorCommand() error = %v", err)
	}
	want := []string{"copilot", "--resume", l.SessionID, "--model", "gpt-5"}
	if !slices.Equal(argv, want) {
		t.Fatalf("argv = %v, want %v", argv, want)
	}
	if _, err := os.Stat(filepath.Join(l.WorktreeDir, ".github", "hooks")); !os.IsNotExist(err) {
		t.Fatalf("the command of the author installed hooks: %v", err)
	}
}

func TestTheCommandOfTheAuthorStartsANewConversation(t *testing.T) {
	t.Parallel()
	c := New("", "")
	l := launch(t)

	argv, err := c.AuthorCommand(l)
	if err != nil {
		t.Fatalf("AuthorCommand() error = %v", err)
	}
	want := []string{"copilot", "--session-id", l.SessionID}
	if !slices.Equal(argv, want) {
		t.Fatalf("argv = %v, want %v", argv, want)
	}
	l.WorktreeDir = ""
	if _, err := c.AuthorCommand(l); err == nil {
		t.Fatal("AuthorCommand() took no worktree")
	}
}
