package daemon

import (
	"context"
	"log/slog"

	"github.com/deividfortuna/babysitter/internal/agent"
	"github.com/deividfortuna/babysitter/internal/agent/claude"
	"github.com/deividfortuna/babysitter/internal/agent/copilot"
	"github.com/deividfortuna/babysitter/internal/prwatch"
	"github.com/deividfortuna/babysitter/internal/redact"
)

const agentOff = "none"

func buildAgents(ctx context.Context, cfg Config, log *slog.Logger) map[string]agent.Runner {
	agents := map[string]agent.Runner{}
	add := func(provider string, r agent.Runner) {
		if err := r.Doctor(ctx); err != nil {
			log.Warn("agent unavailable, watches cannot use it", "provider", provider, "err", redact.Err(err))
			return
		}
		agents[provider] = r
	}
	if cfg.AgentBin != agentOff {
		add(prwatch.ProviderClaude, claude.New(cfg.AgentBin, cfg.AgentModel))
	}
	add(prwatch.ProviderCopilot, copilot.New(cfg.CopilotBin, cfg.CopilotModel))
	return agents
}
