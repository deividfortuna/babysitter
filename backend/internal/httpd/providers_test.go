package httpd

import (
	"net/http"
	"slices"
	"testing"
)

func TestListProvidersNamesTheEffortsOfEachModel(t *testing.T) {
	t.Parallel()
	h, _, _ := newTestAPI(t)

	var got ProviderList
	if rec := call(t, h, http.MethodGet, "/providers", "", &got); rec.Code != http.StatusOK {
		t.Fatalf("list providers: %d %s", rec.Code, rec.Body)
	}
	efforts := map[string][]string{}
	for _, p := range got.Providers {
		for _, m := range p.Models {
			var ids []string
			for _, e := range m.Efforts {
				ids = append(ids, e.ID)
			}
			efforts[p.ID+"/"+m.ID] = ids
		}
	}
	if want := []string{"low", "medium", "high", "xhigh", "max"}; !slices.Equal(efforts["claude/opus"], want) {
		t.Fatalf("efforts of opus = %v, want %v", efforts["claude/opus"], want)
	}
	if levels, listed := efforts["claude/haiku"]; !listed || len(levels) != 0 {
		t.Fatalf("efforts of haiku = %v, listed %v; want the model with no effort", levels, listed)
	}
}
