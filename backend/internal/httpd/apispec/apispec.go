package apispec

import (
	_ "embed"
	"net/http"
)

//go:generate go run ../../../cmd/genspec -out openapi.yaml

//go:embed openapi.yaml
var openapiYAML []byte

func ServeYAML(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/yaml; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(openapiYAML)
}
