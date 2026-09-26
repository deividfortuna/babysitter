package httpd

import (
	"net/http"
)

type ProviderModel struct {
	ID    string `json:"id"`
	Label string `json:"label"`
}

type Provider struct {
	ID        string          `json:"id" enum:"claude,copilot"`
	Label     string          `json:"label"`
	Models    []ProviderModel `json:"models"`
	Available bool            `json:"available"`
}

type ProviderList struct {
	Providers []Provider `json:"providers"`
}

func (a *api) handleListProviders(w http.ResponseWriter, r *http.Request) {
	out := ProviderList{Providers: []Provider{}}
	for _, p := range a.watches.Providers() {
		item := Provider{ID: p.ID, Label: p.Label, Available: p.Available, Models: make([]ProviderModel, 0, len(p.Models))}
		for _, m := range p.Models {
			item.Models = append(item.Models, ProviderModel{ID: m.ID, Label: m.Label})
		}
		out.Providers = append(out.Providers, item)
	}
	writeJSON(w, http.StatusOK, out)
}
