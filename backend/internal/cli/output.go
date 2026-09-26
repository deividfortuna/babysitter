package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"slices"
)

const (
	outputText = "text"
	outputJSON = "json"
)

var outputFormats = []string{outputText, outputJSON}

func validateOutput(format string) error {
	if slices.Contains(outputFormats, format) {
		return nil
	}
	return fmt.Errorf("unknown output format %q, want one of %v", format, outputFormats)
}

type printable interface {
	writeText(w io.Writer) error
}

func (o *options) print(w io.Writer, v printable) error {
	if o.output == outputJSON {
		return writeJSON(w, v)
	}
	return v.writeText(w)
}

func writeJSON(w io.Writer, v any) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}
