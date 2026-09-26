package apispec

import (
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
