package remote

import (
	"os"
	"strings"
	"testing"

	"github.com/deividfortuna/babysitter/internal/testutil"
)

func TestThePairingLinkNamesTheHostTheResponderServes(t *testing.T) {
	osHostname = func() (string, error) { return "studio.example.com", nil }
	t.Cleanup(func() { osHostname = os.Hostname })

	an, err := newAnnouncer(nil, nil, Announcement{Instance: "studio", Host: Hostname(), Port: 7420}, testutil.Logger(t))
	if err != nil {
		t.Fatal(err)
	}

	want := PairingLink(strings.TrimSuffix(an.host.String(), "."), 7420, "abc")
	if got := PairingLinks("", 7420, "abc")[0]; got != want {
		t.Fatalf("pairing link %q, want the host the responder serves: %q", got, want)
	}
}
