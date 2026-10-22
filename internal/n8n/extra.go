package n8n

import (
	"bytes"
	"encoding/json"
	"maps"
	"slices"
)

// unknownFields returns the top-level fields of a JSON object that are not in
// known, or nil when there are none. Read models keep them in an Extra map so
// `--output json` does not lose what a newer instance sends.
func unknownFields(data []byte, known []string) (map[string]json.RawMessage, error) {
	var rest map[string]json.RawMessage
	if err := json.Unmarshal(data, &rest); err != nil {
		return nil, err
	}
	for _, name := range known {
		delete(rest, name)
	}
	if len(rest) == 0 {
		return nil, nil
	}
	return rest, nil
}

// appendExtra writes extra into the encoded object base, after its own
// fields and in sorted key order, so output is deterministic. Keys base
// already holds are skipped: the named field wins.
func appendExtra(base []byte, extra map[string]json.RawMessage) ([]byte, error) {
	if len(extra) == 0 {
		return base, nil
	}
	var named map[string]json.RawMessage
	if err := json.Unmarshal(base, &named); err != nil {
		return nil, err
	}
	var b bytes.Buffer
	b.Grow(len(base) + len(extra)*32)
	b.Write(bytes.TrimSuffix(bytes.TrimSpace(base), []byte("}")))
	first := len(named) == 0
	for _, name := range slices.Sorted(maps.Keys(extra)) {
		if _, ok := named[name]; ok {
			continue
		}
		if !first {
			b.WriteByte(',')
		}
		first = false
		key, err := json.Marshal(name)
		if err != nil {
			return nil, err
		}
		b.Write(key)
		b.WriteByte(':')
		b.Write(extra[name])
	}
	b.WriteByte('}')
	return b.Bytes(), nil
}
