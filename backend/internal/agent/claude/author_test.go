package claude

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func TestTheCommandOfTheAuthorContinuesTheConversationWithNoRules(t *testing.T) {
	t.Parallel()
	c := New("", "sonnet")
	c.ConfigPath = filepath.Join(t.TempDir(), ".claude.json")
	l := launch(t)
	l.Resume = true
	l.Model = "opus"

	argv, err := c.AuthorCommand(l)
	if err != nil {
		t.Fatalf("AuthorCommand() error = %v", err)
	}
	want := []string{"claude", "--resume", l.SessionID, "--permission-mode", "manual", "--model", "opus"}
	if !slices.Equal(argv, want) {
		t.Fatalf("argv = %v, want %v", argv, want)
	}
	data, _ := os.ReadFile(c.ConfigPath)
	var config struct {
		Projects map[string]struct {
			HasTrustDialogAccepted bool `json:"hasTrustDialogAccepted"`
		} `json:"projects"`
	}
	if err := json.Unmarshal(data, &config); err != nil || !config.Projects[l.WorktreeDir].HasTrustDialogAccepted {
		t.Fatalf("the worktree is not trusted: %s, %v", data, err)
	}
}

func TestTheCommandOfTheAuthorStartsANewConversation(t *testing.T) {
	t.Parallel()
	c := New("/opt/claude", "")
	c.ConfigPath = filepath.Join(t.TempDir(), ".claude.json")
	l := launch(t)

	argv, err := c.AuthorCommand(l)
	if err != nil {
		t.Fatalf("AuthorCommand() error = %v", err)
	}
	want := []string{"/opt/claude", "--session-id", l.SessionID, "--permission-mode", "manual"}
	if !slices.Equal(argv, want) {
		t.Fatalf("argv = %v, want %v", argv, want)
	}
}

func TestTheCommandOfTheAuthorNeedsAWorktreeAndASession(t *testing.T) {
	t.Parallel()
	c := New("", "")
	c.ConfigPath = filepath.Join(t.TempDir(), ".claude.json")
	if _, err := c.AuthorCommand(launch(t)); err != nil {
		t.Fatalf("AuthorCommand() error = %v", err)
	}
	l := launch(t)
	l.SessionID = ""
	if _, err := c.AuthorCommand(l); err == nil {
		t.Fatal("AuthorCommand() took no session id")
	}
}
