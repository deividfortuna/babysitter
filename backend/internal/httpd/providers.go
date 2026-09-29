package httpd

import (
	"net/http"
)

type ProviderEffort struct {
	ID    string `json:"id"`
	Label string `json:"label"`
}

type ProviderModel struct {
	ID      string           `json:"id"`
	Label   string           `json:"label"`
	Efforts []ProviderEffort `json:"efforts" description:"The effort levels the model takes; empty when the model takes none"`
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
			model := ProviderModel{ID: m.ID, Label: m.Label, Efforts: make([]ProviderEffort, 0, len(m.Efforts))}
			for _, e := range m.Efforts {
				model.Efforts = append(model.Efforts, ProviderEffort{ID: e.ID, Label: e.Label})
			}
			item.Models = append(item.Models, model)
		}
		out.Providers = append(out.Providers, item)
	}
	writeJSON(w, http.StatusOK, out)
}
