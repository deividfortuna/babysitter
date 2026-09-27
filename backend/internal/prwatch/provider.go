package prwatch

import (
	"fmt"
	"slices"
	"strconv"
	"strings"
)

var ErrBadProvider = fmt.Errorf("invalid provider")

var ErrBadModel = fmt.Errorf("invalid model")

const (
	ProviderClaude  = "claude"
	ProviderCopilot = "copilot"
	ProviderSelf    = "self"
)

func hostedProvider(p string) bool {
	return p != ProviderSelf
}

type Model struct {
	ID    string
	Label string
}

type Provider struct {
	ID        string
	Label     string
	Models    []Model
	Available bool
}

var catalog = []Provider{
	{ID: ProviderClaude, Label: "Claude", Models: []Model{
		{ID: "", Label: "Provider default"},
		{ID: "fable", Label: "Fable"},
		{ID: "opus", Label: "Opus"},
		{ID: "sonnet", Label: "Sonnet"},
		{ID: "haiku", Label: "Haiku"},
	}},
	{ID: ProviderCopilot, Label: "Copilot", Models: []Model{
		{ID: "", Label: "Provider default"},
		{ID: "auto", Label: "Auto"},
		{ID: "gpt-5.6-terra", Label: "GPT-5.6 Terra"},
		{ID: "gpt-5.3-codex", Label: "GPT-5.3 Codex"},
		{ID: "claude-haiku-4.5", Label: "Claude Haiku 4.5"},
	}},
}

func Catalog() []Provider {
	out := slices.Clone(catalog)
	for i := range out {
		out[i].Models = slices.Clone(catalog[i].Models)
	}
	return out
}

func normalized(s string) string {
	return strings.ToLower(strings.TrimSpace(s))
}

func modelsOf(provider string) ([]Model, bool) {
	i := slices.IndexFunc(catalog, func(p Provider) bool { return p.ID == provider })
	if i < 0 {
		return nil, false
	}
	return catalog[i].Models, true
}

func normalizeProvider(s string) (string, bool) {
	switch normalized(s) {
	case "", ProviderClaude:
		return ProviderClaude, true
	case ProviderCopilot:
		return ProviderCopilot, true
	case ProviderSelf:
		return ProviderSelf, true
	default:
		return "", false
	}
}

var hostedProviders = []string{ProviderClaude, ProviderCopilot}

func CheckHostedAgent(provider, model string) error {
	if !slices.Contains(hostedProviders, provider) {
		return fmt.Errorf("%w: provider must be %q or %q, got %q", ErrBadProvider, ProviderClaude, ProviderCopilot, provider)
	}
	if _, ok := normalizeModel(provider, model); !ok {
		return modelError(provider, model)
	}
	return nil
}

func providerError(s string) error {
	return fmt.Errorf("%w: provider must be %q, %q or %q, got %q", ErrBadProvider, ProviderClaude, ProviderCopilot, ProviderSelf, s)
}

func normalizeModel(provider, model string) (string, bool) {
	m := normalized(model)
	if !hostedProvider(provider) {
		return "", m == ""
	}
	models, ok := modelsOf(provider)
	if !ok {
		return "", false
	}
	if !slices.ContainsFunc(models, func(known Model) bool { return known.ID == m }) {
		return "", false
	}
	return m, true
}

func modelError(provider, model string) error {
	if !hostedProvider(provider) {
		return fmt.Errorf("%w: a watch of provider %s takes no model, got %q", ErrBadModel, provider, model)
	}
	models, _ := modelsOf(provider)
	var names []string
	for _, m := range models {
		if m.ID != "" {
			names = append(names, strconv.Quote(m.ID))
		}
	}
	return fmt.Errorf("%w: model of %s must be empty or one of %s, got %q", ErrBadModel, provider, strings.Join(names, ", "), model)
}
