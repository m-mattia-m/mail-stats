package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"text/tabwriter"

	"gopkg.in/yaml.v3"
)

const (
	outputTable = "table"
	outputJSON  = "json"
	outputYAML  = "yaml"
)

func validateOutput(format string) error {
	switch format {
	case outputTable, outputJSON, outputYAML:
		return nil
	default:
		return fmt.Errorf("unknown output format %q (want table, json or yaml)", format)
	}
}

// printStructured writes v as JSON or YAML.
func printStructured(w io.Writer, format string, v any) error {
	if format == outputJSON {
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		return enc.Encode(v)
	}
	enc := yaml.NewEncoder(w)
	enc.SetIndent(2)
	if err := enc.Encode(v); err != nil {
		return err
	}
	return enc.Close()
}

func newTable(w io.Writer) *tabwriter.Writer {
	return tabwriter.NewWriter(w, 0, 0, 3, ' ', 0)
}
