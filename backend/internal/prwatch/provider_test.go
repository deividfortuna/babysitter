package prwatch

import (
	"errors"
	"slices"
	"strings"
	"testing"
)

func TestCatalogListsEveryProviderWithADefaultModel(t *testing.T) {
	t.Parallel()
	got := Catalog()
	if len(got) != 2 || got[0].ID != ProviderClaude || got[1].ID != ProviderCopilot {
		t.Fatalf("catalog = %+v", got)
	}
	for _, p := range got {
		if p.Label == "" {
			t.Errorf("provider %q has no label", p.ID)
		}
		if p.Available {
			t.Errorf("provider %q comes back available; the service fills that in", p.ID)
		}
		if len(p.Models) == 0 || p.Models[0].ID != "" {
			t.Errorf("provider %q does not offer its default first: %+v", p.ID, p.Models)
		}
		for _, m := range p.Models {
			if m.Label == "" {
				t.Errorf("model %q of %q has no label", m.ID, p.ID)
			}
		}
	}
}

func TestCatalogCopiesTheModels(t *testing.T) {
	t.Parallel()
	Catalog()[0].Models[0].ID = "tampered"
	if id := Catalog()[0].Models[0].ID; id != "" {
		t.Fatalf("the caller changed the catalog: model = %q", id)
	}
}

func TestNormalizeModel(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name     string
		provider string
		model    string
		want     string
		ok       bool
	}{
		{"empty is the default of the provider", ProviderClaude, "", "", true},
		{"a model of the provider", ProviderClaude, "sonnet", "sonnet", true},
		{"case and space do not matter", ProviderClaude, "  Opus ", "opus", true},
		{"a model of copilot", ProviderCopilot, "gpt-5.3-codex", "gpt-5.3-codex", true},
		{"a model of the other provider", ProviderClaude, "gpt-5.3-codex", "", false},
		{"an unknown model", ProviderCopilot, "no-such-model", "", false},
		{"an unknown provider", "gemini", "", "", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := normalizeModel(tc.provider, tc.model)
			ok := err == nil
			if got != tc.want || ok != tc.ok {
				t.Fatalf("normalizeModel(%q, %q) = %q, %v; want %q, %v", tc.provider, tc.model, got, ok, tc.want, tc.ok)
			}
		})
	}
}

func TestModelErrorNamesTheModelsOfThatProvider(t *testing.T) {
	t.Parallel()
	err := modelError(ProviderClaude, "gpt-5.3-codex")
	if !errors.Is(err, ErrBadModel) {
		t.Fatalf("error = %v", err)
	}
	msg := err.Error()
	for _, want := range []string{`"sonnet"`, `"opus"`} {
		if !strings.Contains(msg, want) {
			t.Errorf("message lacks %s: %s", want, msg)
		}
	}
	if strings.Contains(msg, `"auto"`) {
		t.Errorf("message names a model of the other provider: %s", msg)
	}
}

func TestCatalogCopiesTheEfforts(t *testing.T) {
	t.Parallel()
	Catalog()[0].Models[0].Efforts[0].ID = "tampered"
	if id := Catalog()[0].Models[0].Efforts[0].ID; id != "low" {
		t.Fatalf("the caller changed the catalog: effort = %q", id)
	}
}

func TestNormalizeEffort(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name     string
		provider string
		model    string
		effort   string
		want     string
		ok       bool
	}{
		{"empty is the default of the model", ProviderClaude, "haiku", "", "", true},
		{"a level of the model", ProviderClaude, "opus", "xhigh", "xhigh", true},
		{"a level of the default model", ProviderClaude, "", "max", "max", true},
		{"case and space do not matter", ProviderClaude, " Sonnet", " High ", "high", true},
		{"a level of copilot", ProviderCopilot, "gpt-5.3-codex", "xhigh", "xhigh", true},
		{"no reasoning is a level of its own", ProviderCopilot, "gpt-5.6-terra", "none", "none", true},
		{"a level the model does not take", ProviderCopilot, "gpt-5.3-codex", "max", "", false},
		{"copilot picks the model of auto, so no level fits it", ProviderCopilot, "auto", "low", "", false},
		{"the default model of copilot is unknown, so no level fits it", ProviderCopilot, "", "low", "", false},
		{"a model that takes no effort", ProviderClaude, "haiku", "low", "", false},
		{"an unknown level", ProviderClaude, "opus", "ultra", "", false},
		{"a self watch", ProviderSelf, "", "high", "", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := normalizeEffort(tc.provider, tc.model, tc.effort)
			ok := err == nil
			if got != tc.want || ok != tc.ok {
				t.Fatalf("normalizeEffort(%q, %q, %q) = %q, %v; want %q, %v", tc.provider, tc.model, tc.effort, got, ok, tc.want, tc.ok)
			}
		})
	}
}

func TestEffortErrorNamesTheLevelsOfThatModel(t *testing.T) {
	t.Parallel()
	err := effortError(ProviderCopilot, "gpt-5.3-codex", "max")
	if !errors.Is(err, ErrBadEffort) {
		t.Fatalf("error = %v", err)
	}
	if msg := err.Error(); !strings.Contains(msg, `one of "low", "medium", "high", "xhigh", got "max"`) {
		t.Errorf("message does not name the levels of gpt-5.3-codex: %s", msg)
	}
	if msg := effortError(ProviderClaude, "haiku", "low").Error(); !strings.Contains(msg, "takes no effort") {
		t.Errorf("message of a model without effort = %s", msg)
	}
}

func TestNormalizeHostedAgent(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name       string
		provider   string
		model      string
		effort     string
		wantModel  string
		wantEffort string
		wantErr    error
	}{
		{name: "the IDs of the manifest", provider: ProviderClaude, model: " Opus ", effort: " High ", wantModel: "opus", wantEffort: "high"},
		{name: "an effort the model does not take", provider: ProviderClaude, model: "haiku", effort: "high", wantErr: ErrBadEffort},
		{name: "a self watch", provider: ProviderSelf, wantErr: ErrBadProvider},
		{name: "no provider", model: "opus", wantErr: ErrBadProvider},
		{name: "an unknown provider", provider: "gemini", wantErr: ErrBadProvider},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			model, effort, err := NormalizeHostedAgent(tc.provider, tc.model, tc.effort)
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("NormalizeHostedAgent(%q, %q, %q) error = %v, want %v", tc.provider, tc.model, tc.effort, err, tc.wantErr)
			}
			got, want := [2]string{model, effort}, [2]string{tc.wantModel, tc.wantEffort}
			if got != want {
				t.Fatalf("NormalizeHostedAgent(%q, %q, %q) = %q, want %q", tc.provider, tc.model, tc.effort, got, want)
			}
		})
	}
}

func TestAManifestThatDoesNotHoldTogetherIsRefused(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		raw  string
		want string
	}{
		{
			"a model listed twice",
			`{"effortSets":{},"providers":[{"id":"copilot","label":"Copilot","models":[{"id":"auto","label":"Auto"},{"id":"auto","label":"Auto"}]}]}`,
			`"auto" of copilot is listed twice`,
		},
		{
			"an unknown effort set",
			`{"effortSets":{},"providers":[{"id":"claude","label":"Claude","models":[{"id":"","label":"Default","efforts":"missing"}]}]}`,
			`"missing"`,
		},
		{
			"an unknown effort level",
			`{"effortLabels":{"low":"Low"},"effortSets":{"some":["low","ultra"]},"providers":[]}`,
			`"ultra"`,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, err := parseManifest([]byte(tc.raw))
			if err == nil {
				t.Fatal("parseManifest() error = nil, want a refusal")
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("parseManifest() error = %v, want it to name %s", err, tc.want)
			}
		})
	}
}

func TestCopilotOffersTheEffortsOfEachModel(t *testing.T) {
	t.Parallel()
	cases := []struct {
		model   string
		efforts []string
	}{
		{"claude-opus-5.5", []string{"low", "medium", "high", "xhigh", "max"}},
		{"gpt-6-sol", []string{"none", "low", "medium", "high", "xhigh", "max"}},
		{"gemini-3.5-flash", []string{"minimal", "low", "medium", "high"}},
		{"kimi-k3", []string{"low", "high", "max"}},
		{"claude-opus-4.7", []string{"medium"}},
		{"kimi-k2.7-code", nil},
	}
	for _, tc := range cases {
		m, ok := modelOf(ProviderCopilot, tc.model)
		if !ok {
			t.Errorf("copilot does not offer %s", tc.model)
			continue
		}
		var got []string
		for _, level := range m.Efforts {
			got = append(got, level.ID)
		}
		if !slices.Equal(got, tc.efforts) {
			t.Errorf("efforts of %s = %v, want %v", tc.model, got, tc.efforts)
		}
	}
}
