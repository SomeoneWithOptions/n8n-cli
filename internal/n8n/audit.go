package n8n

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"net/http"
	"slices"
	"strings"
)

// AuditPath generates a security audit of the instance. It is a POST because it
// takes options, not because it changes anything: the report is derived from
// existing workflows, credentials and settings.
const AuditPath = "/audit"

// Audit risk categories accepted by POST /audit.
const (
	AuditCategoryCredentials = "credentials"
	AuditCategoryDatabase    = "database"
	AuditCategoryNodes       = "nodes"
	AuditCategoryFilesystem  = "filesystem"
	AuditCategoryInstance    = "instance"
)

// AuditCategories lists every category the API documents, in specification
// order. An omitted category filter audits all of them.
var AuditCategories = []string{
	AuditCategoryCredentials,
	AuditCategoryDatabase,
	AuditCategoryNodes,
	AuditCategoryFilesystem,
	AuditCategoryInstance,
}

// AuditOptions are the optional request body of POST /audit. The zero value
// sends no body at all, which audits every category with the instance's own
// abandoned-workflow threshold.
type AuditOptions struct {
	// DaysAbandonedWorkflow is how long a workflow may go unexecuted before it
	// counts as abandoned. Zero leaves the instance default in place.
	DaysAbandonedWorkflow int
	// Categories narrows the audit to these risk categories. Empty audits all.
	Categories []string
}

// Validate rejects options the API would reject, so a typo fails before a
// request instead of costing a round trip.
func (o AuditOptions) Validate() error {
	if o.DaysAbandonedWorkflow < 0 {
		return fmt.Errorf("daysAbandonedWorkflow %d is negative", o.DaysAbandonedWorkflow)
	}
	for _, category := range o.Categories {
		if !validAuditCategory(category) {
			return fmt.Errorf("unknown audit category %q: the API defines %s", category, strings.Join(AuditCategories, ", "))
		}
	}
	return nil
}

func validAuditCategory(category string) bool {
	return slices.Contains(AuditCategories, category)
}

// empty reports whether the options would produce an empty request body.
func (o AuditOptions) empty() bool {
	return o.DaysAbandonedWorkflow == 0 && len(o.Categories) == 0
}

// auditRequest is the body of POST /audit.
type auditRequest struct {
	AdditionalOptions auditAdditionalOptions `json:"additionalOptions"`
}

type auditAdditionalOptions struct {
	DaysAbandonedWorkflow int      `json:"daysAbandonedWorkflow,omitempty"`
	Categories            []string `json:"categories,omitempty"`
}

// body returns the request body for these options, or nil when there is
// nothing to send. The body is optional in the specification, so an unfiltered
// audit sends no body rather than an empty object.
func (o AuditOptions) body() any {
	if o.empty() {
		return nil
	}
	// The wire struct has the same shape as the options, so a field added to
	// one without the other stops compiling rather than going unsent.
	return auditRequest{AdditionalOptions: auditAdditionalOptions(o)}
}

// Audit is the security report POST /audit returns: one risk report per
// category that produced findings. Categories with nothing to report are
// absent, and an audit that finds nothing at all comes back empty.
type Audit struct {
	// Reports is keyed by the report name the API uses, e.g. "Nodes Risk
	// Report". Look a category up with [Audit.Report] instead of building that
	// name yourself.
	Reports map[string]RiskReport
	// Raw is the response body exactly as the instance sent it. Report sections
	// are untyped in the specification and their fields differ per risk, so
	// [RiskSection] keeps only the fields every report shares and JSON output
	// re-emits Raw rather than a lossy re-encoding.
	Raw json.RawMessage
}

// RiskReport is one category's findings.
type RiskReport struct {
	// Risk is the category the report covers, e.g. "nodes".
	Risk string `json:"risk"`
	// Sections are the individual findings.
	Sections []RiskSection `json:"sections"`
}

// RiskSection is one finding: what was found, why it matters and what to do
// about it. Reports carry extra per-risk fields beyond these; see [Audit.Raw].
type RiskSection struct {
	Title          string         `json:"title"`
	Description    string         `json:"description"`
	Recommendation string         `json:"recommendation"`
	Location       []RiskLocation `json:"location,omitempty"`
}

// RiskLocation points at the credential, node or package a finding is about.
// Which fields are set depends on Kind.
type RiskLocation struct {
	Kind         string `json:"kind,omitempty"`
	ID           string `json:"id,omitempty"`
	Name         string `json:"name,omitempty"`
	WorkflowID   string `json:"workflowId,omitempty"`
	WorkflowName string `json:"workflowName,omitempty"`
	NodeID       string `json:"nodeId,omitempty"`
	NodeName     string `json:"nodeName,omitempty"`
	NodeType     string `json:"nodeType,omitempty"`
	PackageURL   string `json:"packageUrl,omitempty"`
}

// UnmarshalJSON decodes the report map, tolerating the empty JSON array the API
// sends instead of an empty object when nothing was found.
func (a *Audit) UnmarshalJSON(data []byte) error {
	a.Raw = append(a.Raw[:0:0], data...)
	a.Reports = nil

	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
		return nil
	}
	if trimmed[0] == '[' {
		var findings []json.RawMessage
		if err := json.Unmarshal(trimmed, &findings); err != nil {
			return err
		}
		if len(findings) > 0 {
			return fmt.Errorf("audit report is a %d-element array, want an object keyed by report name", len(findings))
		}
		return nil
	}
	return json.Unmarshal(trimmed, &a.Reports)
}

// MarshalJSON re-emits the instance's own document, so nothing the CLI does not
// model is lost on the way to stdout.
func (a Audit) MarshalJSON() ([]byte, error) {
	if len(bytes.TrimSpace(a.Raw)) == 0 {
		return json.Marshal(a.Reports)
	}
	return a.Raw, nil
}

// ReportNames returns the report keys in sorted order, so output and tests do
// not depend on Go's map iteration order.
func (a *Audit) ReportNames() []string {
	return slices.Sorted(maps.Keys(a.Reports))
}

// Report looks a report up by category, e.g. Report("nodes"). It matches the
// report's own risk field first and its name second, because the two disagree
// on some instances.
func (a *Audit) Report(category string) (RiskReport, bool) {
	names := a.ReportNames()
	for _, name := range names {
		if strings.EqualFold(a.Reports[name].Risk, category) {
			return a.Reports[name], true
		}
	}
	folded := strings.ToLower(category)
	for _, name := range names {
		if strings.HasPrefix(strings.ToLower(name), folded) {
			return a.Reports[name], true
		}
	}
	return RiskReport{}, false
}

// Findings counts the sections across every report, which is the one number
// that answers "did this audit find anything".
func (a *Audit) Findings() int {
	var total int
	for _, report := range a.Reports {
		total += len(report.Sections)
	}
	return total
}

// Locations counts the locations across every section of a report.
func (r RiskReport) Locations() int {
	var total int
	for _, section := range r.Sections {
		total += len(section.Location)
	}
	return total
}

// GenerateAudit runs a security audit of the instance. It requires the
// securityAudit:generate scope and changes nothing: the report is derived from
// data that already exists.
func (c *Client) GenerateAudit(ctx context.Context, opts AuditOptions) (*Audit, error) {
	if err := opts.Validate(); err != nil {
		return nil, err
	}
	var audit Audit
	if _, err := c.Do(ctx, Request{
		Method: http.MethodPost,
		Path:   AuditPath,
		Body:   opts.body(),
	}, &audit); err != nil {
		return nil, err
	}
	return &audit, nil
}
