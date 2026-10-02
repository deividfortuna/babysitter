package ghclient

import (
	"context"
	"testing"

	"github.com/deividfortuna/babysitter/internal/ghclient/ghfake"
)

func TestUserInstallationsNameTheKindOfEachAccount(t *testing.T) {
	t.Parallel()
	g := ghfake.New()
	g.Install("alice")
	g.Install("acme", "api")

	got, err := UserInstallations(context.Background(), g.Client(t))
	if err != nil {
		t.Fatalf("UserInstallations() error = %v", err)
	}

	want := []Installation{
		{Account: "alice", AvatarURL: "https://avatars.githubusercontent.com/alice", Organization: false, AllRepo: true},
		{Account: "acme", AvatarURL: "https://avatars.githubusercontent.com/acme", Organization: true, AllRepo: false},
	}
	if len(got) != len(want) {
		t.Fatalf("UserInstallations() = %+v, want %+v", got, want)
	}
	for i := range want {
		got[i].ID = 0
		if got[i] != want[i] {
			t.Errorf("installation %d = %+v, want %+v", i, got[i], want[i])
		}
	}
}
