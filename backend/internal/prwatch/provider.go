package prwatch

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"slices"
	"strconv"
	"strings"
)

var ErrBadProvider = fmt.Errorf("invalid provider")

var ErrBadModel = fmt.Errorf("invalid model")

var ErrBadEffort = fmt.Errorf("invalid effort")

const (
	ProviderClaude  = "claude"
	ProviderCopilot = "copilot"
	ProviderSelf    = "self"
)

func hostedProvider(p string) bool {
	return p != ProviderSelf
}

type Effort struct {
	ID    string
	Label string
}

type Model struct {
	ID      string
	Label   string
	Efforts []Effort
}

type Provider struct {
	ID        string
	Label     string
	Models    []Model
	Available bool
}

//go:embed model-manifest.json
var manifestJSON []byte

type manifest struct {
	EffortLabels map[string]string   `json:"effortLabels"`
	EffortSets   map[string][]string `json:"effortSets"`
	Providers    []struct {
		ID     string `json:"id"`
		Label  string `json:"label"`
		Models []struct {
			ID      string `json:"id"`
			Label   string `json:"label"`
			Efforts string `json:"efforts"`
		} `json:"models"`
	} `json:"providers"`
}

var catalog = mustParseManifest(manifestJSON)

func mustParseManifest(raw []byte) []Provider {
	providers, err := parseManifest(raw)
	if err != nil {
		panic(err)
	}
	return providers
}

func parseManifest(raw []byte) ([]Provider, error) {
	var m manifest
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil, fmt.Errorf("model manifest: %w", err)
	}
	sets, err := m.effortSets()
	if err != nil {
		return nil, err
	}
	out := make([]Provider, 0, len(m.Providers))
	for _, p := range m.Providers {
		provider := Provider{ID: p.ID, Label: p.Label}
		for _, model := range p.Models {
			if _, found := modelIn(provider.Models, model.ID); found {
				return nil, fmt.Errorf("model manifest: model %q of %s is listed twice", model.ID, p.ID)
			}
			efforts, known := sets[model.Efforts]
			if model.Efforts != "" && !known {
				return nil, fmt.Errorf("model manifest: model %q of %s names the unknown effort set %q", model.ID, p.ID, model.Efforts)
			}
			provider.Models = append(provider.Models, Model{ID: model.ID, Label: model.Label, Efforts: efforts})
		}
		out = append(out, provider)
	}
	return out, nil
}

func (m manifest) effortSets() (map[string][]Effort, error) {
	sets := make(map[string][]Effort, len(m.EffortSets))
	for name, ids := range m.EffortSets {
		for _, id := range ids {
			label, known := m.EffortLabels[id]
			if !known {
				return nil, fmt.Errorf("model manifest: effort set %q names the unknown level %q", name, id)
			}
			sets[name] = append(sets[name], Effort{ID: id, Label: label})
		}
	}
	return sets, nil
}

func Catalog() []Provider {
	out := slices.Clone(catalog)
	for i := range out {
		out[i].Models = slices.Clone(catalog[i].Models)
		for j := range out[i].Models {
			out[i].Models[j].Efforts = slices.Clone(catalog[i].Models[j].Efforts)
		}
	}
	return out
}

func normalized(s string) string {
	return strings.ToLower(strings.TrimSpace(s))
}

func modelsOf(provider string) []Model {
	i := slices.IndexFunc(catalog, func(p Provider) bool { return p.ID == provider })
	if i < 0 {
		return nil
	}
	return catalog[i].Models
}

func modelOf(provider, model string) (Model, bool) {
	return modelIn(modelsOf(provider), normalized(model))
}

func modelIn(models []Model, id string) (Model, bool) {
	i := slices.IndexFunc(models, func(known Model) bool { return known.ID == id })
	if i < 0 {
		return Model{}, false
	}
	return models[i], true
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

func NormalizeHostedAgent(provider, model, effort string) (normalModel, normalEffort string, err error) {
	if !slices.Contains(hostedProviders, provider) {
		return "", "", fmt.Errorf("%w: provider must be %q or %q, got %q", ErrBadProvider, ProviderClaude, ProviderCopilot, provider)
	}
	chosen, err := normalizeAgent(provider, model, effort)
	return chosen.model, chosen.effort, err
}

func normalizeAgent(provider, model, effort string) (agentChoice, error) {
	p, ok := normalizeProvider(provider)
	if !ok {
		return agentChoice{}, providerError(provider)
	}
	m, err := normalizeModel(p, model)
	if err != nil {
		return agentChoice{}, err
	}
	e, err := normalizeEffort(p, m, effort)
	if err != nil {
		return agentChoice{}, err
	}
	return agentChoice{provider: p, model: m, effort: e}, nil
}

func providerError(s string) error {
	return fmt.Errorf("%w: provider must be %q, %q or %q, got %q", ErrBadProvider, ProviderClaude, ProviderCopilot, ProviderSelf, s)
}

func normalizeModel(provider, model string) (string, error) {
	m := normalized(model)
	_, offered := modelOf(provider, m)
	accepted := offered || (!hostedProvider(provider) && m == "")
	if !accepted {
		return "", modelError(provider, model)
	}
	return m, nil
}

func normalizeEffort(provider, model, effort string) (string, error) {
	e := normalized(effort)
	if e == "" {
		return "", nil
	}
	known, _ := modelOf(provider, model)
	if !slices.ContainsFunc(known.Efforts, func(level Effort) bool { return level.ID == e }) {
		return "", effortError(provider, model, effort)
	}
	return e, nil
}

func modelError(provider, model string) error {
	if !hostedProvider(provider) {
		return fmt.Errorf("%w: a watch of provider %s takes no model, got %q", ErrBadModel, provider, model)
	}
	names := quotedIDs(modelsOf(provider), func(m Model) string { return m.ID })
	return fmt.Errorf("%w: model of %s must be empty or one of %s, got %q", ErrBadModel, provider, names, model)
}

func effortError(provider, model, effort string) error {
	if !hostedProvider(provider) {
		return fmt.Errorf("%w: a watch of provider %s takes no effort, got %q", ErrBadEffort, provider, effort)
	}
	subject := provider + " with its default model"
	if m := normalized(model); m != "" {
		subject = provider + " model " + m
	}
	known, _ := modelOf(provider, model)
	if len(known.Efforts) == 0 {
		return fmt.Errorf("%w: %s takes no effort, got %q", ErrBadEffort, subject, effort)
	}
	names := quotedIDs(known.Efforts, func(level Effort) string { return level.ID })
	return fmt.Errorf("%w: effort of %s must be empty or one of %s, got %q", ErrBadEffort, subject, names, effort)
}

func quotedIDs[T any](items []T, id func(T) string) string {
	var names []string
	for _, item := range items {
		if v := id(item); v != "" {
			names = append(names, strconv.Quote(v))
		}
	}
	return strings.Join(names, ", ")
}
