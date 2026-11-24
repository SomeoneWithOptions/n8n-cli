package n8n

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

// AgentsPath is the collection endpoint for Agents (n8n 2.49 and later).
const AgentsPath = "/agents"

// maxAgentListLimit is the largest page size the agent list accepts.
const maxAgentListLimit = 250

// AgentProject names the project that owns an Agent.
type AgentProject struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Type string `json:"type"`
}

// AgentIntegration is one channel integration of a published Agent version.
// Settings and approval are platform specific and stay raw.
type AgentIntegration struct {
	Type         string          `json:"type"`
	CredentialID string          `json:"credentialId"`
	Settings     json.RawMessage `json:"settings,omitempty"`
	Approval     json.RawMessage `json:"approval,omitempty"`
}

// AgentActiveVersion is the published version of an Agent and its channels.
type AgentActiveVersion struct {
	VersionID    string             `json:"versionId"`
	Integrations []AgentIntegration `json:"integrations"`
}

// Agent is one Agent as the API returns it. List and delete return the
// summary fields only; get and create add Project, IsArchived (list too),
// Config, ConfigHash, Tools, and ActiveVersion.
//
// Config and Tools are free-form documents the server owns, so they stay raw.
// Raw keeps the response verbatim so --output json never drops a field this
// type does not name; MarshalJSON re-emits it when present.
type Agent struct {
	ID              string              `json:"id"`
	Name            string              `json:"name"`
	ProjectID       string              `json:"projectId"`
	Project         *AgentProject       `json:"project,omitempty"`
	AvailableInMCP  bool                `json:"availableInMCP"`
	IsArchived      bool                `json:"isArchived"`
	VersionID       *string             `json:"versionId"`
	ActiveVersionID *string             `json:"activeVersionId"`
	CreatedAt       string              `json:"createdAt"`
	UpdatedAt       string              `json:"updatedAt"`
	Config          json.RawMessage     `json:"config,omitempty"`
	ConfigHash      *string             `json:"configHash,omitempty"`
	Tools           json.RawMessage     `json:"tools,omitempty"`
	ActiveVersion   *AgentActiveVersion `json:"activeVersion,omitempty"`
	Raw             json.RawMessage     `json:"-"`
}

// UnmarshalJSON decodes the named fields and keeps the verbatim document.
func (a *Agent) UnmarshalJSON(data []byte) error {
	type plain Agent
	var decoded plain
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	*a = Agent(decoded)
	a.Raw = append(json.RawMessage(nil), data...)
	return nil
}

// MarshalJSON re-emits the verbatim document when present.
func (a Agent) MarshalJSON() ([]byte, error) {
	if len(a.Raw) > 0 {
		return a.Raw, nil
	}
	type plain Agent
	return json.Marshal(plain(a))
}

// Published reports whether the Agent has a published version.
func (a Agent) Published() bool { return a.ActiveVersionID != nil && *a.ActiveVersionID != "" }

// AgentListOptions pages the agent list and narrows it to one project.
type AgentListOptions struct {
	ListOptions
	ProjectID string
}

// Validate rejects page sizes and project filters the API would reject.
func (o AgentListOptions) Validate() error {
	if err := o.ListOptions.Validate(); err != nil {
		return err
	}
	if o.Limit > maxAgentListLimit {
		return fmt.Errorf("limit must not exceed %d, got %d", maxAgentListLimit, o.Limit)
	}
	if o.ProjectID != strings.TrimSpace(o.ProjectID) {
		return fmt.Errorf("project ID must not start or end with whitespace")
	}
	return nil
}

// Apply encodes the page and project filter.
func (o AgentListOptions) Apply(q url.Values) url.Values {
	q = o.ListOptions.Apply(q)
	if o.ProjectID != "" {
		q.Set("projectId", o.ProjectID)
	}
	return q
}

// ListAgents returns one cursor page of the Agents in the projects where the
// caller holds agent:list.
func (c *Client) ListAgents(ctx context.Context, opts AgentListOptions) (Page[Agent], error) {
	page := Page[Agent]{Data: []Agent{}}
	if err := opts.Validate(); err != nil {
		return page, err
	}
	_, err := c.Do(ctx, Request{Path: AgentsPath, Query: opts.Apply(nil)}, &page)
	if page.Data == nil {
		page.Data = []Agent{}
	}
	return page, err
}

// GetAgent returns one Agent with its configuration and tools.
func (c *Client) GetAgent(ctx context.Context, id string) (*Agent, error) {
	if err := validateAgentID(id); err != nil {
		return nil, err
	}
	var agent Agent
	if _, err := c.Do(ctx, Request{Path: PathJoin("agents", id)}, &agent); err != nil {
		return nil, err
	}
	return &agent, nil
}

// DeleteAgent deletes an Agent right away, published or not, together with its
// channels, and returns its summary as the server last saw it.
func (c *Client) DeleteAgent(ctx context.Context, id string) (*Agent, error) {
	if err := validateAgentID(id); err != nil {
		return nil, err
	}
	var deleted Agent
	if _, err := c.Do(ctx, Request{Method: http.MethodDelete, Path: PathJoin("agents", id)}, &deleted); err != nil {
		return nil, err
	}
	return &deleted, nil
}

// CreateAgentRequest is the create document. The schema rejects unknown
// properties, so it carries the writable fields only. ProjectID is optional:
// the server falls back to the caller's personal project. Config holds name,
// model, and instructions; an empty model creates an unconfigured draft.
// Tools maps a tool name to its code and a descriptor that has a name.
type CreateAgentRequest struct {
	Name           string          `json:"name"`
	ProjectID      string          `json:"projectId,omitempty"`
	Config         json.RawMessage `json:"config,omitempty"`
	Tools          json.RawMessage `json:"tools,omitempty"`
	AvailableInMCP *bool           `json:"availableInMCP,omitempty"`
}

// AgentReadOnlyFields are the fields 'agent get' returns that create does not
// accept. A create document built from a get response must drop them.
var AgentReadOnlyFields = []string{
	"id", "project", "isArchived", "versionId", "activeVersionId", "createdAt",
	"updatedAt", "configHash", "activeVersion",
}

// Validate rejects a create document the API would reject.
func (r CreateAgentRequest) Validate() error {
	if strings.TrimSpace(r.Name) == "" {
		return fmt.Errorf("agent name is required: pass --name or set name in --input")
	}
	if r.ProjectID != strings.TrimSpace(r.ProjectID) {
		return fmt.Errorf("agent project ID must not start or end with whitespace")
	}
	if len(r.Config) > 0 {
		var config map[string]json.RawMessage
		if err := json.Unmarshal(r.Config, &config); err != nil || config == nil {
			return fmt.Errorf("agent config must be a JSON object such as {\"name\":\"Support\",\"model\":\"anthropic/claude-sonnet-4-5\",\"instructions\":\"...\"}; omit it to create an unconfigured draft")
		}
	}
	if len(r.Tools) > 0 {
		if err := validateAgentTools(r.Tools); err != nil {
			return err
		}
	}
	return nil
}

// agentTool is the shape create requires of each tools entry; other
// descriptor fields pass through untouched.
type agentTool struct {
	Code       *string `json:"code"`
	Descriptor *struct {
		Name *string `json:"name"`
	} `json:"descriptor"`
}

func validateAgentTools(raw json.RawMessage) error {
	var tools map[string]json.RawMessage
	if err := json.Unmarshal(raw, &tools); err != nil || tools == nil {
		return fmt.Errorf("agent tools must be a JSON object mapping each tool name to {\"code\": ..., \"descriptor\": {\"name\": ...}}")
	}
	for name, entry := range tools {
		var tool agentTool
		if err := json.Unmarshal(entry, &tool); err != nil {
			return fmt.Errorf("agent tool %q must be an object with code and descriptor: %w", name, err)
		}
		if tool.Code == nil {
			return fmt.Errorf("agent tool %q needs code (a string)", name)
		}
		if tool.Descriptor == nil || tool.Descriptor.Name == nil {
			return fmt.Errorf("agent tool %q needs descriptor.name (a string)", name)
		}
	}
	return nil
}

// CreateAgent creates an Agent. opts accepts [IdempotencyKey].
func (c *Client) CreateAgent(ctx context.Context, request CreateAgentRequest, opts ...CallOption) (*Agent, error) {
	if err := request.Validate(); err != nil {
		return nil, err
	}
	var created Agent
	req := Request{Method: http.MethodPost, Path: AgentsPath, Body: request}
	applyCallOptions(&req, opts)
	if _, err := c.Do(ctx, req, &created); err != nil {
		return nil, err
	}
	return &created, nil
}

func validateAgentID(id string) error {
	if strings.TrimSpace(id) == "" {
		return fmt.Errorf("agent ID is required: find it with 'n8n agent list'")
	}
	if strings.TrimSpace(id) != id {
		return fmt.Errorf("agent ID must not start or end with whitespace")
	}
	return nil
}
