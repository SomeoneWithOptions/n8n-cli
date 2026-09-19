package n8n

import (
	"encoding/json"
	"testing"
)

func TestSaveExecutionProgress(t *testing.T) {
	for name, tc := range map[string]struct {
		settings string
		want     ProgressSaving
	}{
		"true":            {`{"saveExecutionProgress":true}`, ProgressSavingOn},
		"false":           {`{"saveExecutionProgress":false}`, ProgressSavingOff},
		"DEFAULT":         {`{"saveExecutionProgress":"DEFAULT"}`, ProgressSavingUnknown},
		"absent":          {`{"executionOrder":"v1"}`, ProgressSavingUnknown},
		"no settings":     {``, ProgressSavingUnknown},
		"null":            {`null`, ProgressSavingUnknown},
		"unexpected type": {`{"saveExecutionProgress":1}`, ProgressSavingUnknown},
		"not an object":   {`[]`, ProgressSavingUnknown},
	} {
		t.Run(name, func(t *testing.T) {
			w := &Workflow{Settings: json.RawMessage(tc.settings)}
			if got := w.SaveExecutionProgress(); got != tc.want {
				t.Errorf("SaveExecutionProgress() = %s, want %s", got, tc.want)
			}
		})
	}
	if got := (*Workflow)(nil).SaveExecutionProgress(); got != ProgressSavingUnknown {
		t.Errorf("nil workflow = %s, want unknown", got)
	}
	for p, want := range map[ProgressSaving]string{ProgressSavingOn: "on", ProgressSavingOff: "off", ProgressSavingUnknown: "unknown", ProgressSaving(9): "unknown"} {
		if p.String() != want {
			t.Errorf("String(%d) = %q, want %q", p, p.String(), want)
		}
	}
}
