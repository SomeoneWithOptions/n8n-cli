package n8n

import (
	"encoding/json"
	"testing"
)

func TestNullableEncodesOmittedNullAndValue(t *testing.T) {
	type doc struct {
		Field Nullable[string] `json:"field,omitzero"`
	}
	for name, tt := range map[string]struct {
		value Nullable[string]
		json  string
	}{
		"omitted": {json: `{}`},
		"null":    {value: SetNull[string](), json: `{"field":null}`},
		"value":   {value: SetValue("x"), json: `{"field":"x"}`},
		"empty":   {value: SetValue(""), json: `{"field":""}`},
	} {
		t.Run(name, func(t *testing.T) {
			encoded, err := json.Marshal(doc{Field: tt.value})
			if err != nil || string(encoded) != tt.json {
				t.Errorf("Marshal = %s (error: %v), want %s", encoded, err, tt.json)
			}
			var decoded doc
			if err := json.Unmarshal([]byte(tt.json), &decoded); err != nil {
				t.Fatalf("Unmarshal: %v", err)
			}
			if decoded.Field != tt.value {
				t.Errorf("Unmarshal = %+v, want %+v", decoded.Field, tt.value)
			}
		})
	}

	var bad doc
	if err := json.Unmarshal([]byte(`{"field":1}`), &bad); err == nil {
		t.Error("a number decoded into Nullable[string]")
	}
}
