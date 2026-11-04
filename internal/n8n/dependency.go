package n8n

import (
	"context"
	"fmt"
)

// Dependency types the API reports. The type names the relation to the
// requested resource, not only the kind of the listed one.
const (
	// DependencyAgent is an agent that uses the requested resource.
	DependencyAgent = "agent"
	// DependencyCredential is a credential the requested workflow uses.
	DependencyCredential = "credential"
	// DependencyDataTable is a data table the requested workflow uses.
	DependencyDataTable = "dataTable"
	// DependencyErrorWorkflow is the error workflow the requested workflow uses.
	DependencyErrorWorkflow = "errorWorkflow"
	// DependencyErrorWorkflowParent is a workflow that uses the requested
	// workflow as its error workflow.
	DependencyErrorWorkflowParent = "errorWorkflowParent"
	// DependencyParentWorkflow is a workflow that uses the requested resource.
	DependencyParentWorkflow = "parentWorkflow"
	// DependencySubWorkflow is a sub-workflow the requested workflow calls.
	DependencySubWorkflow = "subWorkflow"
)

// MaxDependencyLimit is the largest page size the dependency endpoints accept.
const MaxDependencyLimit = 250

// Dependency is one entry of a dependency list: a resource related to the
// requested workflow, credential or data table.
type Dependency struct {
	Type string `json:"type"`
	ID   string `json:"id"`
	Name string `json:"name"`
	// ProjectID is the owning project, when the server knows it.
	ProjectID string `json:"projectId,omitempty"`
}

// DependencyPage is one cursor-paginated dependency list. The three
// dependency endpoints share this shape.
type DependencyPage struct {
	Data       []Dependency `json:"data"`
	NextCursor string       `json:"nextCursor"`
	// InaccessibleCount is how many dependencies exist that the caller may not
	// see; they are not listed. It counts the whole list, not this page.
	InaccessibleCount int `json:"inaccessibleCount"`
}

// HasMore reports whether another page is available.
func (p DependencyPage) HasMore() bool { return p.NextCursor != "" }

// Page returns the entries and cursor as a generic page, for the shared
// pagination helpers.
func (p DependencyPage) Page() Page[Dependency] {
	return Page[Dependency]{Data: p.Data, NextCursor: p.NextCursor}
}

// DependencyFunc fetches one dependency page of the resource with the given ID.
type DependencyFunc func(ctx context.Context, id string, opts ListOptions) (DependencyPage, error)

// listDependencies fetches one dependency page from path. Callers validate the
// resource ID; options are checked here, before any request.
func (c *Client) listDependencies(ctx context.Context, path string, opts ListOptions) (DependencyPage, error) {
	if err := ValidateDependencyOptions(opts); err != nil {
		return DependencyPage{}, err
	}
	var page DependencyPage
	if _, err := c.Do(ctx, Request{Path: path, Query: opts.Apply(nil)}, &page); err != nil {
		return DependencyPage{}, err
	}
	if page.Data == nil {
		page.Data = []Dependency{}
	}
	return page, nil
}

// ValidateDependencyOptions rejects pagination the dependency endpoints
// refuse: a negative limit or one above [MaxDependencyLimit].
func ValidateDependencyOptions(opts ListOptions) error {
	if err := opts.Validate(); err != nil {
		return err
	}
	if opts.Limit > MaxDependencyLimit {
		return fmt.Errorf("limit must not exceed %d, got %d", MaxDependencyLimit, opts.Limit)
	}
	return nil
}
