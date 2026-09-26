package prwatch

import (
	"errors"
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
			got, ok := normalizeModel(tc.provider, tc.model)
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
