package n8n

import (
	"encoding/json"
	"testing"
)

func TestAppendExtra(t *testing.T) {
	for name, tt := range map[string]struct {
		base  string
		extra map[string]json.RawMessage
		want  string
	}{
		"no extra":         {base: `{"a":1}`, want: `{"a":1}`},
		"empty base":       {base: `{}`, extra: map[string]json.RawMessage{"b": json.RawMessage(`2`)}, want: `{"b":2}`},
		"sorted after":     {base: `{"a":1}`, extra: map[string]json.RawMessage{"c": json.RawMessage(`3`), "b": json.RawMessage(`2`)}, want: `{"a":1,"b":2,"c":3}`},
		"named field wins": {base: `{"a":1}`, extra: map[string]json.RawMessage{"a": json.RawMessage(`9`)}, want: `{"a":1}`},
	} {
		t.Run(name, func(t *testing.T) {
			got, err := appendExtra([]byte(tt.base), tt.extra)
			if err != nil || string(got) != tt.want {
				t.Errorf("appendExtra = %s (error: %v), want %s", got, err, tt.want)
			}
			if !json.Valid(got) {
				t.Errorf("appendExtra produced invalid JSON: %s", got)
			}
		})
	}
}

func TestUnknownFields(t *testing.T) {
	rest, err := unknownFields([]byte(`{"a":1,"b":2}`), []string{"a"})
	if err != nil || len(rest) != 1 || string(rest["b"]) != "2" {
		t.Errorf("unknownFields = %v (error: %v)", rest, err)
	}
	if rest, err := unknownFields([]byte(`{"a":1}`), []string{"a"}); err != nil || rest != nil {
		t.Errorf("unknownFields = %v (error: %v), want nil", rest, err)
	}
}
