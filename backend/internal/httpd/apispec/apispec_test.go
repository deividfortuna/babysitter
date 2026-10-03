package apispec

import (
	"maps"
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestStopWatchParamsKeepsTheOptionOptional(t *testing.T) {
	t.Parallel()
	var doc struct {
		Components struct {
			Schemas map[string]struct {
				Required   []string       `yaml:"required"`
				Properties map[string]any `yaml:"properties"`
			} `yaml:"schemas"`
		} `yaml:"components"`
	}
	rec := httptest.NewRecorder()
	ServeYAML(rec, httptest.NewRequest(http.MethodGet, "/openapi.yaml", nil))
	if err := yaml.Unmarshal(rec.Body.Bytes(), &doc); err != nil {
		t.Fatal(err)
	}
	params, ok := doc.Components.Schemas["StopWatchParams"]
	if !ok {
		t.Fatal("StopWatchParams is not in the document")
	}
	if _, ok := params.Properties["keepWorktree"]; !ok {
		t.Fatalf("keepWorktree is not a property of StopWatchParams: %v", params.Properties)
	}
	if slices.Contains(params.Required, "keepWorktree") {
		t.Fatalf("keepWorktree is required, but the stop takes an empty body: %v", params.Required)
	}
}

func TestResizeParamsCarriesTheLimitsTheHandlerChecks(t *testing.T) {
	t.Parallel()
	type bounds struct {
		Minimum int `yaml:"minimum"`
		Maximum int `yaml:"maximum"`
	}
	var doc struct {
		Components struct {
			Schemas map[string]struct {
				Properties map[string]bounds `yaml:"properties"`
			} `yaml:"schemas"`
		} `yaml:"components"`
	}
	rec := httptest.NewRecorder()
	ServeYAML(rec, httptest.NewRequest(http.MethodGet, "/openapi.yaml", nil))
	if err := yaml.Unmarshal(rec.Body.Bytes(), &doc); err != nil {
		t.Fatal(err)
	}
	params, ok := doc.Components.Schemas["ResizeParams"]
	if !ok {
		t.Fatal("ResizeParams is not in the document")
	}
	want := map[string]bounds{"rows": {Minimum: 1, Maximum: 500}, "cols": {Minimum: 1, Maximum: 1000}}
	for field, limits := range want {
		if got := params.Properties[field]; got != limits {
			t.Errorf("%s has %+v, want %+v", field, got, limits)
		}
	}
}

func TestStartSignInDeclaresTheCancelledSignIn(t *testing.T) {
	t.Parallel()
	var doc struct {
		Paths map[string]map[string]struct {
			Responses map[string]any `yaml:"responses"`
		} `yaml:"paths"`
	}
	rec := httptest.NewRecorder()
	ServeYAML(rec, httptest.NewRequest(http.MethodGet, "/openapi.yaml", nil))
	if err := yaml.Unmarshal(rec.Body.Bytes(), &doc); err != nil {
		t.Fatal(err)
	}
	responses := doc.Paths["/api/v1/auth/signin"]["post"].Responses
	if _, ok := responses["409"]; !ok {
		t.Fatalf("POST /auth/signin declares %v, want 409 for signin_cancelled", slices.Sorted(maps.Keys(responses)))
	}
}
