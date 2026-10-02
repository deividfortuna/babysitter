package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/deividfortuna/babysitter/internal/ghclient/ghfake"
	"github.com/deividfortuna/babysitter/internal/remote"
	"github.com/deividfortuna/babysitter/internal/runfile"
)

func TestPairPrintsOnlyTheAddressADaemonListensOn(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	running := fmt.Sprintf(`{"pid":%d,"port":5000,"startedAt":%q,"remotePort":7420,"remoteHost":"192.168.1.20"}`,
		os.Getpid(), time.Now().Add(time.Hour).Format(time.RFC3339))
	if err := os.WriteFile(runfile.Path(dir), []byte(running), 0o600); err != nil {
		t.Fatal(err)
	}

	out, err := runCLI(t, ghfake.New(), filepath.Join(dir, "babysitter.db"), "daemon", "pair", "--data-dir", dir, "-o", "json")
	if err != nil {
		t.Fatal(err)
	}

	var got pairOutput
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("invalid JSON %q: %v", out, err)
	}
	want := remote.PairingLink("192.168.1.20", 7420, got.Token)
	if len(got.Links) != 1 || got.Links[0] != want {
		t.Fatalf("links = %q, want only %q", got.Links, want)
	}
}
