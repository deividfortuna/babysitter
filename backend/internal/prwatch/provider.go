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
	ID    string `json:"id"`
	Label string `json:"label"`
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
	EffortSets map[string][]Effort `json:"effortSets"`
	Providers  []struct {
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
	out := make([]Provider, 0, len(m.Providers))
	for _, p := range m.Providers {
		provider := Provider{ID: p.ID, Label: p.Label}
		for _, model := range p.Models {
			if _, found := modelIn(provider.Models, model.ID); found {
				return nil, fmt.Errorf("model manifest: model %q of %s is listed twice", model.ID, p.ID)
			}
			efforts, known := m.EffortSets[model.Efforts]
			if model.Efforts != "" && !known {
				return nil, fmt.Errorf("model manifest: model %q of %s names the unknown effort set %q", model.ID, p.ID, model.Efforts)
			}
			provider.Models = append(provider.Models, Model{ID: model.ID, Label: model.Label, Efforts: slices.Clone(efforts)})
		}
		out = append(out, provider)
	}
	return out, nil
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

func modelsOf(provider string) ([]Model, bool) {
	i := slices.IndexFunc(catalog, func(p Provider) bool { return p.ID == provider })
	if i < 0 {
		return nil, false
	}
	return catalog[i].Models, true
}

func modelOf(provider, model string) (Model, bool) {
	models, _ := modelsOf(provider)
	return modelIn(models, normalized(model))
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

func CheckHostedAgent(provider, model, effort string) error {
	if !slices.Contains(hostedProviders, provider) {
		return fmt.Errorf("%w: provider must be %q or %q, got %q", ErrBadProvider, ProviderClaude, ProviderCopilot, provider)
	}
	if _, ok := normalizeModel(provider, model); !ok {
		return modelError(provider, model)
	}
	if _, ok := normalizeEffort(provider, model, effort); !ok {
		return effortError(provider, model, effort)
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
	if _, ok := modelOf(provider, m); !ok {
		return "", false
	}
	return m, true
}

func normalizeEffort(provider, model, effort string) (string, bool) {
	e := normalized(effort)
	if e == "" {
		return "", true
	}
	known, _ := modelOf(provider, model)
	if !slices.ContainsFunc(known.Efforts, func(level Effort) bool { return level.ID == e }) {
		return "", false
	}
	return e, true
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
	names := make([]string, 0, len(known.Efforts))
	for _, level := range known.Efforts {
		names = append(names, strconv.Quote(level.ID))
	}
	return fmt.Errorf("%w: effort of %s must be empty or one of %s, got %q", ErrBadEffort, subject, strings.Join(names, ", "), effort)
}
