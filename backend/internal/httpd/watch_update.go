package httpd

import (
	"encoding/json"
	"maps"
	"net/http"
	"slices"
	"strings"

	"github.com/deividfortuna/babysitter/internal/prwatch"
)

var changeableWatchFields = []string{"approvalsRequired", "mergeMethod", "mergeWhenReady"}

func (a *api) handleUpdateWatch(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	var fields map[string]json.RawMessage
	if err := readJSON(w, r, &fields); err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", "body must be a JSON object")
		return
	}
	if name, found := fixedField(fields); found {
		writeError(w, http.StatusBadRequest, "field_not_changeable",
			name+" of a watch cannot change; only "+strings.Join(changeableWatchFields, ", ")+" can")
		return
	}
	var req UpdateWatchRequest
	if err := decodeFields(fields, &req); err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", "approvalsRequired must be a number or null, mergeMethod a string and mergeWhenReady a boolean")
		return
	}
	wt, err := a.watches.SetMergeRules(r.Context(), id, prwatch.MergeRulesChange{
		ApprovalsRequired: approvalsIn(req.ApprovalsRequired),
		MergeMethod:       req.MergeMethod,
		MergeWhenReady:    req.MergeWhenReady,
	})
	if updateWatchErrors.write(w, err) {
		return
	}
	writeJSON(w, http.StatusOK, a.watchOut(r.Context(), wt))
}

func fixedField(fields map[string]json.RawMessage) (string, bool) {
	for _, name := range slices.Sorted(maps.Keys(fields)) {
		if !slices.Contains(changeableWatchFields, name) {
			return name, true
		}
	}
	return "", false
}

func decodeFields(fields map[string]json.RawMessage, v any) error {
	b, err := json.Marshal(fields)
	if err != nil {
		return err
	}
	return json.Unmarshal(b, v)
}
