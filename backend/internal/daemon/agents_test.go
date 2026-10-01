package daemon

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/deividfortuna/babysitter/internal/agent/copilot"
	"github.com/deividfortuna/babysitter/internal/prwatch"
	"github.com/deividfortuna/babysitter/internal/testutil"
)

// fakeTool writes a command that prints the lines and exits with the
// code: a shell script on Unix, a batch file on Windows.
func fakeTool(t *testing.T, name string, exit int, lines ...string) string {
	t.Helper()
	var script strings.Builder
	bin := filepath.Join(t.TempDir(), name)
	if runtime.GOOS == "windows" {
		bin += ".cmd"
		script.WriteString("@echo off\r\n")
		for _, line := range lines {
			fmt.Fprintf(&script, "echo %s\r\n", line)
		}
		fmt.Fprintf(&script, "exit /b %d\r\n", exit)
	} else {
		script.WriteString("#!/bin/sh\n")
		for _, line := range lines {
			fmt.Fprintf(&script, "echo %q\n", line)
		}
		fmt.Fprintf(&script, "exit %d\n", exit)
	}
	if err := os.WriteFile(bin, []byte(script.String()), 0o750); err != nil {
		t.Fatal(err)
	}
	return bin
}

func TestBuildAgentsRunsCopilotFromTheConfiguredCommand(t *testing.T) {
	t.Parallel()
	bin := fakeTool(t, "copilot-elsewhere", 0, "1.0.0")
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
	bin := fakeTool(t, "copilot-broken", 1, "login failed, token "+token+" is not valid")
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
