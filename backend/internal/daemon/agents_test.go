package daemon

import (
	"bytes"
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/deividfortuna/babysitter/internal/agent/copilot"
	"github.com/deividfortuna/babysitter/internal/prwatch"
	"github.com/deividfortuna/babysitter/internal/testutil"
)

func TestBuildAgentsRunsCopilotFromTheConfiguredCommand(t *testing.T) {
	t.Parallel()
	bin := filepath.Join(t.TempDir(), "copilot-elsewhere")
	if err := os.WriteFile(bin, []byte("#!/bin/sh\necho 1.0.0\n"), 0o750); err != nil {
		t.Fatal(err)
	}
	cfg := Config{AgentBin: agentOff, CopilotBin: bin, CopilotModel: "gpt-5"}
	agents := buildAgents(context.Background(), cfg, testutil.Logger(t))
	r, ok := agents[prwatch.ProviderCopilot].(*copilot.Runner)
	if !ok {
		t.Fatalf("copilot runner = %T, want *copilot.Runner", agents[prwatch.ProviderCopilot])
	}
	if r.Bin != bin || r.Model != "gpt-5" {
		t.Fatalf("copilot runner = %q %q, want %q %q", r.Bin, r.Model, bin, "gpt-5")
	}
	if _, ok := agents[prwatch.ProviderClaude]; ok {
		t.Fatal("claude is on, but the config turned it off")
	}
}

func TestBuildAgentsLogsAFailedVersionCheckWithoutTheToken(t *testing.T) {
	t.Parallel()
	token := "ghp_" + strings.Repeat("a", 36)
	bin := filepath.Join(t.TempDir(), "copilot-broken")
	script := "#!/bin/sh\necho \"login failed, token " + token + " is not valid\"\nexit 1\n"
	if err := os.WriteFile(bin, []byte(script), 0o750); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	log := slog.New(slog.NewTextHandler(&out, nil))

	agents := buildAgents(context.Background(), Config{AgentBin: agentOff, CopilotBin: bin}, log)

	if _, ok := agents[prwatch.ProviderCopilot]; ok {
		t.Fatal("a command whose version check fails was taken for an agent")
	}
	if !strings.Contains(out.String(), "agent unavailable") {
		t.Fatalf("the daemon said nothing about the provider: %s", out.String())
	}
	if strings.Contains(out.String(), token) {
		t.Fatalf("the log carries what the command printed: %s", out.String())
	}
}
