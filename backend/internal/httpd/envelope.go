package httpd

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
)

type APIError struct {
	Error ErrorBody `json:"error"`
}

type ErrorBody struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, APIError{Error: ErrorBody{Code: code, Message: message}})
}

func readJSON(w http.ResponseWriter, r *http.Request, v any) error {
	dec := json.NewDecoder(http.MaxBytesReader(serverWriter(w), r.Body, 1<<20))
	dec.DisallowUnknownFields()
	return dec.Decode(v)
}

func readOptionalJSON(w http.ResponseWriter, r *http.Request, v any) error {
	if err := readJSON(w, r, v); err != nil && !errors.Is(err, io.EOF) {
		return err
	}
	return nil
}

func serverWriter(w http.ResponseWriter) http.ResponseWriter {
	for {
		wrapper, ok := w.(interface{ Unwrap() http.ResponseWriter })
		if !ok {
			return w
		}
		w = wrapper.Unwrap()
	}
}
