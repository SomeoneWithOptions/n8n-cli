package n8n

import "encoding/json"

// ProgressSaving is a workflow's saveExecutionProgress setting. n8n writes
// per-node run data during a run only when this is on; otherwise the data
// appears once, when the run ends.
type ProgressSaving int

const (
	// ProgressSavingUnknown means the setting is absent or "DEFAULT", so the
	// workflow inherits EXECUTIONS_DATA_SAVE_ON_PROGRESS from the instance,
	// which the public API does not expose.
	ProgressSavingUnknown ProgressSaving = iota
	// ProgressSavingOn means the workflow saves progress node by node.
	ProgressSavingOn
	// ProgressSavingOff means the workflow explicitly opted out.
	ProgressSavingOff
)

// String is the value the CLI prints: on, off or unknown.
func (p ProgressSaving) String() string {
	switch p {
	case ProgressSavingOn:
		return "on"
	case ProgressSavingOff:
		return "off"
	default:
		return "unknown"
	}
}

// SaveExecutionProgress decodes settings.saveExecutionProgress, which the API
// types as boolean | "DEFAULT" and may omit. Any unexpected shape is Unknown,
// never an error: an unrecognised settings object must not break a command
// that only needs the setting to choose its wording.
func (w *Workflow) SaveExecutionProgress() ProgressSaving {
	if w == nil || len(w.Settings) == 0 {
		return ProgressSavingUnknown
	}
	var settings struct {
		SaveExecutionProgress json.RawMessage `json:"saveExecutionProgress"`
	}
	if err := json.Unmarshal(w.Settings, &settings); err != nil {
		return ProgressSavingUnknown
	}
	var value bool
	if err := json.Unmarshal(settings.SaveExecutionProgress, &value); err != nil {
		return ProgressSavingUnknown
	}
	if value {
		return ProgressSavingOn
	}
	return ProgressSavingOff
}
