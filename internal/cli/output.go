package cli

import (
	"encoding/json"
	"fmt"
	"io"
)

// Output formats supported by commands that have both a machine-readable and a
// human-readable rendering.
const (
	outputText = "text"
	outputJSON = "json"
)

// validateOutput rejects an unknown --output value before any work is done.
func validateOutput(format string) error {
	switch format {
	case outputText, outputJSON:
		return nil
	default:
		return fmt.Errorf("unknown output format %q: use %s or %s", format, outputText, outputJSON)
	}
}

// writeJSON writes indented JSON with a trailing newline, in field order, so
// tests can compare it and shell pipelines can read it.
func writeJSON(w io.Writer, v any) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}
