package n8n

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

const agentSummaryJSON = `{"id":"ag/1","name":"Support","projectId":"p1","availableInMCP":true,"isArchived":false,"versionId":"v2","activeVersionId":"v1","createdAt":"2026-11-01T00:00:00.000Z","updatedAt":"2026-11-02T00:00:00.000Z","future":true}`

const agentDetailJSON = `{"id":"ag/1","name":"Support","projectId":"p1","project":{"id":"p1","name":"Ops","type":"team"},"availableInMCP":false,"isArchived":false,"versionId":"v2","activeVersionId":null,"createdAt":"2026-11-01T00:00:00.000Z","updatedAt":"2026-11-02T00:00:00.000Z","config":{"name":"Support","model":"anthropic/claude-sonnet-4-5","instructions":"Help."},"configHash":"h1","tools":{"lookup":{"code":"return 1","descriptor":{"name":"lookup","extra":1}}},"activeVersion":{"versionId":"v1","integrations":[{"type":"slack","credentialId":"c1","settings":{"channel":"x"}}]}}`

func TestListAgents(t *testing.T) {
	server := newCommunityPackageServer(t, packageJSONHandler(http.StatusOK, `{"data":[`+agentSummaryJSON+`],"nextCursor":"next page"}`))
	page, err := server.client(t).ListAgents(context.Background(), AgentListOptions{ListOptions: ListOptions{Limit: 25, Cursor: "c 1"}, ProjectID: "p1"})
	if err != nil {
		t.Fatalf("ListAgents: %v", err)
	}
	req, body := server.last(t)
	if req.Method != http.MethodGet || req.URL.Path != BasePath+AgentsPath || body != "" {
		t.Errorf("request = %s %s body %q", req.Method, req.URL.Path, body)
	}
	q := req.URL.Query()
	if q.Get("limit") != "25" || q.Get("cursor") != "c 1" || q.Get("projectId") != "p1" {
		t.Errorf("query = %q", req.URL.RawQuery)
	}
	if len(page.Data) != 1 || page.NextCursor != "next page" {
		t.Fatalf("page = %+v", page)
	}
	agent := page.Data[0]
	if agent.ID != "ag/1" || !agent.AvailableInMCP || !agent.Published() || *agent.VersionID != "v2" {
		t.Errorf("agent = %+v", agent)
	}
	out, err := json.Marshal(agent)
	if err != nil || !strings.Contains(string(out), `"future":true`) {
		t.Errorf("marshal = %s, %v; want the verbatim document", out, err)
	}
}

func TestListAgentsValidatesBeforeTransport(t *testing.T) {
	server := newCommunityPackageServer(t, packageJSONHandler(http.StatusOK, `{}`))
	client := server.client(t)
	for _, opts := range []AgentListOptions{
		{ListOptions: ListOptions{Limit: 251}},
		{ListOptions: ListOptions{Limit: -1}},
		{ProjectID: " p1"},
	} {
		if _, err := client.ListAgents(context.Background(), opts); err == nil {
			t.Errorf("%+v accepted", opts)
		}
	}
	if len(server.requests) != 0 {
		t.Errorf("validation sent %d requests", len(server.requests))
	}
}

func TestGetAndDeleteAgent(t *testing.T) {
	server := newCommunityPackageServer(t, packageJSONHandler(http.StatusOK, agentDetailJSON))
	client := server.client(t)
	agent, err := client.GetAgent(context.Background(), "ag/1")
	if err != nil {
		t.Fatalf("GetAgent: %v", err)
	}
	req, _ := server.last(t)
	if req.Method != http.MethodGet || req.URL.EscapedPath() != BasePath+"/agents/ag%2F1" {
		t.Errorf("request = %s %s", req.Method, req.URL.EscapedPath())
	}
	if agent.Project == nil || agent.Project.Name != "Ops" || agent.Published() || agent.ActiveVersion == nil ||
		agent.ActiveVersion.Integrations[0].Type != "slack" || *agent.ConfigHash != "h1" {
		t.Errorf("agent = %+v", agent)
	}
	out, _ := json.Marshal(agent)
	if !strings.Contains(string(out), `"extra":1`) {
		t.Errorf("marshal = %s; want tools kept verbatim", out)
	}

	if _, err := client.DeleteAgent(context.Background(), "ag/1"); err != nil {
		t.Fatalf("DeleteAgent: %v", err)
	}
	req, _ = server.last(t)
	if req.Method != http.MethodDelete || req.URL.EscapedPath() != BasePath+"/agents/ag%2F1" {
		t.Errorf("request = %s %s", req.Method, req.URL.EscapedPath())
	}
	for _, id := range []string{"", " x"} {
		if _, err := client.GetAgent(context.Background(), id); err == nil {
			t.Errorf("GetAgent(%q) accepted", id)
		}
		if _, err := client.DeleteAgent(context.Background(), id); err == nil {
			t.Errorf("DeleteAgent(%q) accepted", id)
		}
	}
}

func TestCreateAgent(t *testing.T) {
	server := newCommunityPackageServer(t, packageJSONHandler(http.StatusCreated, agentDetailJSON))
	client := server.client(t)
	mcp := false
	request := CreateAgentRequest{
		Name:           "Support",
		ProjectID:      "p1",
		Config:         json.RawMessage(`{"model":""}`),
		Tools:          json.RawMessage(`{"lookup":{"code":"return 1","descriptor":{"name":"lookup"}}}`),
		AvailableInMCP: &mcp,
	}
	if _, err := client.CreateAgent(context.Background(), request, IdempotencyKey("agent-key")); err != nil {
		t.Fatalf("CreateAgent: %v", err)
	}
	req, body := server.last(t)
	want := `{"name":"Support","projectId":"p1","config":{"model":""},"tools":{"lookup":{"code":"return 1","descriptor":{"name":"lookup"}}},"availableInMCP":false}`
	if req.Method != http.MethodPost || req.URL.Path != BasePath+AgentsPath || body != want {
		t.Errorf("request = %s %s body %s", req.Method, req.URL.Path, body)
	}
	if req.Header.Get(IdempotencyKeyHeader) != "agent-key" {
		t.Errorf("idempotency header = %q", req.Header.Get(IdempotencyKeyHeader))
	}

	if _, err := client.CreateAgent(context.Background(), CreateAgentRequest{Name: "Draft"}); err != nil {
		t.Fatalf("CreateAgent draft: %v", err)
	}
	if _, body := server.last(t); body != `{"name":"Draft"}` {
		t.Errorf("draft body = %s, want only the name", body)
	}
}

func TestCreateAgentValidatesBeforeTransport(t *testing.T) {
	server := newCommunityPackageServer(t, packageJSONHandler(http.StatusCreated, agentDetailJSON))
	client := server.client(t)
	for _, request := range []CreateAgentRequest{
		{},
		{Name: " "},
		{Name: "a", ProjectID: " p"},
		{Name: "a", Config: json.RawMessage(`[]`)},
		{Name: "a", Config: json.RawMessage(`null`)},
		{Name: "a", Tools: json.RawMessage(`[]`)},
		{Name: "a", Tools: json.RawMessage(`{"t":{"descriptor":{"name":"t"}}}`)},
		{Name: "a", Tools: json.RawMessage(`{"t":{"code":"x","descriptor":{}}}`)},
		{Name: "a", Tools: json.RawMessage(`{"t":"code"}`)},
	} {
		if _, err := client.CreateAgent(context.Background(), request); err == nil {
			t.Errorf("%+v accepted", request)
		}
	}
	if len(server.requests) != 0 {
		t.Errorf("validation sent %d requests", len(server.requests))
	}
}
