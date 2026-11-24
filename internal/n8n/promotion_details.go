package n8n

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

// MaxPromotionChangeDetailKeys is how many resources one change-details
// request may read.
const MaxPromotionChangeDetailKeys = 50

// PromotionChangeDetailsRequest reads the content of changed resources of a
// change preview (n8n 2.49 and later). CommitSHA is the preview's commitSha
// and is always sent: null when the preview had none. Keys are resource keys
// from the preview.
type PromotionChangeDetailsRequest struct {
	CommitSHA *string  `json:"commitSha"`
	Keys      []string `json:"keys"`
}

// Validate rejects a request the API would reject before transport.
func (r PromotionChangeDetailsRequest) Validate() error {
	if r.CommitSHA != nil && !promotionCommitSHA.MatchString(*r.CommitSHA) {
		return fmt.Errorf("commit SHA %q must be the full lowercase 40- or 64-digit hex commitSha of the preview", *r.CommitSHA)
	}
	if len(r.Keys) == 0 {
		return fmt.Errorf("at least one resource key is required: copy the key column of 'n8n promotion project changes'")
	}
	if len(r.Keys) > MaxPromotionChangeDetailKeys {
		return fmt.Errorf("%d resource keys requested, at most %d fit one request: split them", len(r.Keys), MaxPromotionChangeDetailKeys)
	}
	seen := make(map[string]bool, len(r.Keys))
	for _, key := range r.Keys {
		if key == "" {
			return fmt.Errorf("resource key must not be empty")
		}
		if seen[key] {
			return fmt.Errorf("duplicate resource key %q: request each key once", key)
		}
		seen[key] = true
	}
	return nil
}

// PromotionResourceDetail is the content of one resource before and after
// the change; a side the resource is absent from is null. Before and After
// stay raw: their shape depends on Type (project, folder, workflow,
// credential, dataTable, variable, or tag). Credential content holds
// expressions only. ValueHidden reports a variable whose values were left
// out because the API key or the user lacks variable:list.
type PromotionResourceDetail struct {
	Key              string                     `json:"key"`
	Type             string                     `json:"type"`
	Before           json.RawMessage            `json:"before"`
	After            json.RawMessage            `json:"after"`
	Revision         PromotionResourceRevision  `json:"revision"`
	ValueHidden      bool                       `json:"valueHidden,omitempty"`
	DataTableChanges []PromotionDataTableChange `json:"dataTableChanges,omitempty"`
}

// Name returns the resource name on the after side, else the before side.
func (d PromotionResourceDetail) Name() string {
	for _, side := range []json.RawMessage{d.After, d.Before} {
		var named struct {
			Name string `json:"name"`
		}
		if len(side) > 0 && json.Unmarshal(side, &named) == nil && named.Name != "" {
			return named.Name
		}
	}
	return ""
}

// Change describes the detail as added, deleted, or modified from which
// sides hold the resource.
func (d PromotionResourceDetail) Change() string {
	before, after := present(d.Before), present(d.After)
	switch {
	case before && after:
		return PromotionResourceModified
	case after:
		return PromotionResourceAdded
	case before:
		return PromotionResourceDeleted
	}
	return "-"
}

func present(raw json.RawMessage) bool {
	trimmed := strings.TrimSpace(string(raw))
	return trimmed != "" && trimmed != "null"
}

// PromotionChangeDetails is the change-details response. MissingKeys lists
// requested keys neither side holds anymore. The response is preserved
// verbatim for JSON output.
type PromotionChangeDetails struct {
	Details     []PromotionResourceDetail `json:"details"`
	MissingKeys []string                  `json:"missingKeys"`
	raw         json.RawMessage
}

// UnmarshalJSON decodes the typed view and retains the response.
func (r *PromotionChangeDetails) UnmarshalJSON(data []byte) error {
	type plain PromotionChangeDetails
	var p plain
	if err := json.Unmarshal(data, &p); err != nil {
		return err
	}
	*r = PromotionChangeDetails(p)
	r.raw = append(json.RawMessage(nil), data...)
	return nil
}

// MarshalJSON re-emits the server response when present.
func (r PromotionChangeDetails) MarshalJSON() ([]byte, error) {
	if len(r.raw) > 0 {
		return r.raw, nil
	}
	type plain PromotionChangeDetails
	if r.Details == nil {
		r.Details = []PromotionResourceDetail{}
	}
	if r.MissingKeys == nil {
		r.MissingKeys = []string{}
	}
	return json.Marshal(plain(r))
}

// GetPromotionChangeDetails reads the content before and after the change of
// up to 50 preview resources. The branch side is read at request.CommitSHA,
// the instance side now. It writes nothing.
func (c *Client) GetPromotionChangeDetails(ctx context.Context, projectID, direction string, request PromotionChangeDetailsRequest) (*PromotionChangeDetails, error) {
	if err := validatePromotionID(projectID, "project ID"); err != nil {
		return nil, err
	}
	if err := validatePromotionDirection(direction); err != nil {
		return nil, err
	}
	if err := request.Validate(); err != nil {
		return nil, err
	}
	var result PromotionChangeDetails
	req := Request{Method: http.MethodPost, Path: PathJoin("promotions", "projects", projectID, "changes", direction, "details"), Body: request}
	if _, err := c.Do(ctx, req, &result); err != nil {
		return nil, err
	}
	return &result, nil
}
