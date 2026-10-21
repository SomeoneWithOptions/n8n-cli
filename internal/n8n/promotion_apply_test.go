package n8n

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

// Apply result fixtures follow applyPackageResultDto in the upstream 1.1.1
// contract: one per status of the union.
const promotionAppliedBody = `{
  "connectionId":"conn-1",
  "configId":"cfg-apply",
  "git":{"commitSha":"0123456789abcdef0123456789abcdef01234567","branchName":"main"},
  "status":"applied",
  "counts":{
    "projects":{"created":1,"updated":0,"skipped":0,"deleted":0},
    "folders":{"created":1,"skipped":0,"removed":0},
    "workflows":{"created":1,"updated":0,"skipped":0,"archived":0,"deleted":0,"publishing":{"published":1,"unpublished":0,"unchanged":0,"blocked":0,"failed":0}},
    "credentials":{"matched":1,"stubbed":0},
    "dataTables":{"matched":0,"created":0,"updated":2},
    "variables":{"matched":0,"created":0,"updated":0,"stubbed":0,"missing":0},
    "tags":{"matched":1,"created":0,"renamed":0,"reconciled":0,"skipped":0}
  },
  "warnings":[
    {"kind":"variable","code":"variable-shadowed","name":"API_URL","scope":{"kind":"global"},
     "consumers":[{"project":{"id":"p-1","name":"Sales"},"workflows":[{"id":"wf-1","name":"Sync"}]}]}
  ]
}`

const promotionBlockedBody = `{
  "connectionId":"conn-1",
  "configId":"cfg-apply",
  "git":{"commitSha":"0123456789abcdef0123456789abcdef01234567","branchName":"main"},
  "status":"blocked",
  "preflight":{
    "missingProjects":[{"id":"p-9","name":"Ops","icon":{"type":"emoji","value":"x"},"customTelemetryTags":[{"key":"team","value":"ops"}]}],
    "missingBindings":[
      {"kind":"credential","sourceId":"cred-1","name":"Slack","credentialType":"slackApi",
       "consumers":[{"project":{"id":"p-1","name":"Sales"},"workflows":[{"id":"wf-1","name":"Sync"},{"id":"wf-2","name":"Notify"}]}],
       "ownerProject":{"id":"p-1","name":"Sales"},"expressionData":{"token":"={{ $env.X }}"}},
      {"kind":"variable","name":"API_URL","variableType":"string","scope":{"kind":"project","project":{"id":"p-1","name":"Sales"}},
       "sourceValue":"https://api.example.com","consumers":[{"project":{"id":"p-1","name":"Sales"},"workflows":[{"id":"wf-1","name":"Sync"}]}]}
    ],
    "accessRequirements":[
      {"kind":"credential","sourceId":"cred-2","name":"Postgres","credentialType":"postgres","code":"access-required",
       "consumers":[{"project":{"id":"p-1","name":"Sales"},"workflows":[{"id":"wf-3","name":"Load"}]}]}
    ],
    "conflicts":[
      {"kind":"credential","sourceId":null,"name":"Mail","expectedTypes":["smtp"],"referenceFiles":["workflows/wf-4.json"],"code":"missing-id",
       "consumers":[{"project":{"id":"p-1","name":"Sales"},"workflows":[{"id":"wf-4","name":"Mail"}]}]},
      {"kind":"project","code":"project-deletion","project":{"id":"p-7","name":"Legacy"}},
      {"kind":"data-table","code":"destructive-change","id":"dt-1","name":"Orders",
       "consumers":[{"project":{"id":"p-1","name":"Sales"},"workflows":[{"id":"wf-5","name":"Orders"}]}],
       "changes":[{"kind":"remove-column","column":"total","type":"number","destructive":true},
                  {"kind":"change-column-type","column":"date","from":"string","to":"date","destructive":true},
                  {"kind":"rename-table","from":"orders","to":"Orders","destructive":false}]}
    ],
    "warnings":[]
  }
}`

const promotionSourceChangedBody = `{
  "connectionId":"conn-1",
  "configId":"cfg-apply",
  "git":{"commitSha":"fedcba9876543210fedcba9876543210fedcba98","branchName":"main"},
  "status":"source-changed"
}`

// promotionUnknownStatusBody is a status and a field this client does not
// know; neither may fail the decode or be dropped from JSON output.
const promotionUnknownStatusBody = `{
  "connectionId":"conn-1",
  "configId":"cfg-apply",
  "git":{"commitSha":"fedcba9876543210fedcba9876543210fedcba98","branchName":"main"},
  "status":"queued",
  "queue":{"position":3}
}`

func decodeApplyResult(t *testing.T, body string) *ApplyPromotionResult {
	t.Helper()
	server := newCommunityPackageServer(t, packageJSONHandler(http.StatusOK, body))
	result, err := server.client(t).ApplyPackage(context.Background(), "conn-1")
	if err != nil {
		t.Fatalf("ApplyPackage: %v", err)
	}
	return result
}

func assertLosslessJSON(t *testing.T, v any, body string) {
	t.Helper()
	encoded, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var want, got bytes.Buffer
	if err := json.Compact(&want, []byte(body)); err != nil {
		t.Fatalf("compact fixture: %v", err)
	}
	if err := json.Compact(&got, encoded); err != nil {
		t.Fatalf("compact output: %v", err)
	}
	if got.String() != want.String() {
		t.Errorf("JSON round trip lost data:\n got %s\nwant %s", got.String(), want.String())
	}
}

func TestApplyPackageApplied(t *testing.T) {
	result := decodeApplyResult(t, promotionAppliedBody)
	if result.Status != PromotionApplyStatusApplied || !result.Applied() {
		t.Errorf("status = %q, applied = %v, want applied", result.Status, result.Applied())
	}
	if result.Counts == nil || result.Counts.DataTables.Updated != 2 || result.Counts.Projects.Created != 1 {
		t.Errorf("counts = %+v, want the decoded counts with dataTables.updated", result.Counts)
	}
	if len(result.Warnings) != 1 || result.Warnings[0].Code != "variable-shadowed" || result.Warnings[0].Scope == nil || result.Warnings[0].Scope.Kind != "global" {
		t.Errorf("warnings = %+v, want the shadowed variable", result.Warnings)
	}
	if result.Preflight != nil || result.Extra != nil {
		t.Errorf("preflight = %+v, extra = %v, want neither", result.Preflight, result.Extra)
	}
	assertLosslessJSON(t, result, promotionAppliedBody)
}

func TestApplyPackageBlocked(t *testing.T) {
	result := decodeApplyResult(t, promotionBlockedBody)
	if result.Status != PromotionApplyStatusBlocked || result.Applied() {
		t.Errorf("status = %q, applied = %v, want blocked and not applied", result.Status, result.Applied())
	}
	if result.Counts != nil {
		t.Errorf("counts = %+v, want none on a blocked apply", result.Counts)
	}
	p := result.Preflight
	if p == nil || p.Empty() {
		t.Fatalf("preflight = %+v, want items", p)
	}
	if len(p.MissingProjects) != 1 || p.MissingProjects[0].ID != "p-9" || len(p.MissingProjects[0].Icon) == 0 {
		t.Errorf("missing projects = %+v", p.MissingProjects)
	}
	if len(p.MissingBindings) != 2 {
		t.Fatalf("missing bindings = %+v, want credential and variable", p.MissingBindings)
	}
	credential, variable := p.MissingBindings[0], p.MissingBindings[1]
	if credential.Kind != "credential" || credential.CredentialType != "slackApi" || credential.SourceID == nil || *credential.SourceID != "cred-1" ||
		credential.OwnerProject == nil || len(credential.Consumers) != 1 || len(credential.Consumers[0].Workflows) != 2 {
		t.Errorf("credential binding = %+v", credential)
	}
	if variable.Kind != "variable" || variable.Scope == nil || variable.Scope.Project == nil || variable.Scope.Project.Name != "Sales" {
		t.Errorf("variable binding = %+v", variable)
	}
	if len(p.AccessRequirements) != 1 || p.AccessRequirements[0].Code != "access-required" {
		t.Errorf("access requirements = %+v", p.AccessRequirements)
	}
	if len(p.Conflicts) != 3 {
		t.Fatalf("conflicts = %+v, want three", p.Conflicts)
	}
	if missing := p.Conflicts[0]; missing.SourceID != nil || len(missing.ExpectedTypes) != 1 || len(missing.ReferenceFiles) != 1 {
		t.Errorf("missing-id conflict = %+v, want a null source ID kept as nil", missing)
	}
	if deletion := p.Conflicts[1]; deletion.Code != PromotionConflictProjectDeletion || deletion.Project == nil || deletion.Project.ID != "p-7" {
		t.Errorf("project-deletion conflict = %+v", deletion)
	}
	if change := p.Conflicts[2]; change.Code != PromotionConflictDestructiveChange || len(change.Changes) != 3 || !change.Changes[0].Destructive || change.Changes[2].Destructive {
		t.Errorf("destructive-change conflict = %+v", change)
	}
	if got := result.ExpectedSource(); got != (PromotionExpectedSource{ConfigID: "cfg-apply", BranchName: "main", CommitSHA: "0123456789abcdef0123456789abcdef01234567"}) {
		t.Errorf("expected source = %+v", got)
	} else if err := got.Validate(); err != nil {
		t.Errorf("expected source from a blocked result: %v", err)
	}
	assertLosslessJSON(t, result, promotionBlockedBody)
}

func TestApplyPackageSourceChanged(t *testing.T) {
	result := decodeApplyResult(t, promotionSourceChangedBody)
	if result.Status != PromotionApplyStatusSourceChanged || result.Applied() || result.Counts != nil || result.Preflight != nil {
		t.Errorf("result = %+v, want source-changed with neither counts nor preflight", result)
	}
	assertLosslessJSON(t, result, promotionSourceChangedBody)
}

func TestApplyPackageKeepsUnknownStatus(t *testing.T) {
	result := decodeApplyResult(t, promotionUnknownStatusBody)
	if result.Status != "queued" || result.Applied() {
		t.Errorf("status = %q, applied = %v, want the unknown status kept and not applied", result.Status, result.Applied())
	}
	if string(result.Extra["queue"]) != `{"position":3}` {
		t.Errorf("extra = %v, want the unknown queue field", result.Extra)
	}
	assertLosslessJSON(t, result, promotionUnknownStatusBody)
}

func TestApplyPackageLegacyResultIsApplied(t *testing.T) {
	// promotionApplyResultBody predates the status union: counts, no status.
	result := decodeApplyResult(t, promotionApplyResultBody)
	if result.Status != "" || !result.Applied() {
		t.Errorf("status = %q, applied = %v, want a status-less result with counts treated as applied", result.Status, result.Applied())
	}
}

func TestApplyPromotionResultMarshalsWithoutRaw(t *testing.T) {
	result := ApplyPromotionResult{
		ConnectionID: "conn-1", ConfigID: "cfg-apply", Status: PromotionApplyStatusSourceChanged,
		Git:   PromotionGit{CommitSHA: "abc", BranchName: "main"},
		Extra: map[string]json.RawMessage{"zeta": json.RawMessage(`1`), "alpha": json.RawMessage(`"a"`)},
	}
	encoded, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	want := `{"connectionId":"conn-1","configId":"cfg-apply","status":"source-changed","git":{"commitSha":"abc","branchName":"main"},"alpha":"a","zeta":1}`
	if string(encoded) != want {
		t.Errorf("marshal = %s\nwant %s", encoded, want)
	}
}

func TestPromotionExpectedSourceValidate(t *testing.T) {
	valid := PromotionExpectedSource{ConfigID: "cfg-1", BranchName: "main", CommitSHA: strings.Repeat("a", 40)}
	if err := valid.Validate(); err != nil {
		t.Errorf("SHA-1 source: %v", err)
	}
	sha256 := valid
	sha256.CommitSHA = strings.Repeat("b", 64)
	if err := sha256.Validate(); err != nil {
		t.Errorf("SHA-256 source: %v", err)
	}
	for name, bad := range map[string]PromotionExpectedSource{
		"no config":   {BranchName: "main", CommitSHA: valid.CommitSHA},
		"no branch":   {ConfigID: "cfg-1", BranchName: " ", CommitSHA: valid.CommitSHA},
		"short sha":   {ConfigID: "cfg-1", BranchName: "main", CommitSHA: "deadbeef"},
		"upper sha":   {ConfigID: "cfg-1", BranchName: "main", CommitSHA: strings.Repeat("A", 40)},
		"52-char sha": {ConfigID: "cfg-1", BranchName: "main", CommitSHA: strings.Repeat("a", 52)},
	} {
		if err := bad.Validate(); err == nil {
			t.Errorf("%s: want a validation error", name)
		}
	}
}

func TestCreateGitLabPromotionProvider(t *testing.T) {
	const gitlabBody = `{"provider":{"id":"prov-2","name":"gitlab","type":"gitlab","authType":"token",
	  "config":{"schemaVersion":1,"baseUrl":"https://gitlab.example.com"},
	  "createdAt":"2026-09-17T00:00:00.000Z","updatedAt":"2026-09-17T00:00:00.000Z"},"publicKey":null}`
	server := newCommunityPackageServer(t, packageJSONHandler(http.StatusCreated, gitlabBody))

	created, err := server.client(t).CreatePromotionProvider(context.Background(), CreatePromotionProviderRequest{
		Name: "gitlab", Type: PromotionProviderTypeGitLab,
		Auth:   PromotionProviderAuth{AuthType: PromotionAuthTypeToken, Password: "glpat-secret"},
		Config: &PromotionProviderHostConfig{SchemaVersion: 1, BaseURL: "https://gitlab.example.com"},
	})
	if err != nil {
		t.Fatalf("CreatePromotionProvider without username: %v", err)
	}
	_, raw := server.last(t)
	var sent struct {
		Type   string                     `json:"type"`
		Auth   map[string]json.RawMessage `json:"auth"`
		Config map[string]any             `json:"config"`
	}
	if err := json.Unmarshal([]byte(raw), &sent); err != nil {
		t.Fatalf("request body: %v\n%s", err, raw)
	}
	if sent.Type != "gitlab" || sent.Config["baseUrl"] != "https://gitlab.example.com" || sent.Config["schemaVersion"] != float64(1) {
		t.Errorf("request = %s, want gitlab with config", raw)
	}
	if _, ok := sent.Auth["username"]; ok {
		t.Errorf("auth = %s, want no username", raw)
	}
	if created.Provider.Config == nil || created.Provider.Config.BaseURL == nil || *created.Provider.Config.BaseURL != "https://gitlab.example.com" {
		t.Errorf("provider config = %+v, want the base URL", created.Provider.Config)
	}
}

func TestPromotionProviderHostValidation(t *testing.T) {
	token := PromotionProviderAuth{AuthType: PromotionAuthTypeToken, Password: "secret"}
	config := &PromotionProviderHostConfig{SchemaVersion: 1, BaseURL: "https://gitlab.example.com"}
	for name, request := range map[string]CreatePromotionProviderRequest{
		"git token without username": {Name: "g", Type: PromotionProviderTypeGit, Auth: token},
		"config on git":              {Name: "g", Type: PromotionProviderTypeGit, Auth: PromotionProviderAuth{AuthType: PromotionAuthTypeSSHKey}, Config: config},
		"relative base URL":          {Name: "g", Type: PromotionProviderTypeGitLab, Auth: token, Config: &PromotionProviderHostConfig{SchemaVersion: 1, BaseURL: "gitlab.example.com"}},
		"blank base URL":             {Name: "g", Type: PromotionProviderTypeGitLab, Auth: token, Config: &PromotionProviderHostConfig{SchemaVersion: 1, BaseURL: " "}},
		"config schema 2":            {Name: "g", Type: PromotionProviderTypeGitLab, Auth: token, Config: &PromotionProviderHostConfig{SchemaVersion: 2, BaseURL: "https://gitlab.example.com"}},
		"blank username":             {Name: "g", Type: PromotionProviderTypeGitLab, Auth: PromotionProviderAuth{AuthType: PromotionAuthTypeToken, Username: " ", Password: "secret"}},
		"unknown type":               {Name: "g", Type: "github", Auth: token},
	} {
		if err := request.Validate(); err == nil {
			t.Errorf("%s: want a validation error", name)
		}
	}
	if err := (CreatePromotionProviderRequest{Name: "g", Type: PromotionProviderTypeGitLab, Auth: token}).Validate(); err != nil {
		t.Errorf("gitlab token without config or username: %v", err)
	}
	if err := (UpdatePromotionProviderRequest{Config: config}).Validate(); err != nil {
		t.Errorf("config-only update: %v", err)
	}
	if err := (UpdatePromotionProviderRequest{Auth: &token}).Validate(); err != nil {
		t.Errorf("token update without username: %v", err)
	}
}

func TestPromotionConnectionReadFields(t *testing.T) {
	const body = `{
	  "id":"conn-2","name":"gl","scope":"instance",
	  "target":{"schemaVersion":1,"remoteUrl":"https://gitlab.example.com/org/repo.git","repositoryPath":"org/repo"},
	  "provider":{"id":"prov-2","name":"gitlab","type":"gitlab","authType":"token","createdAt":"t","updatedAt":"t"},
	  "configs":{"apply":{"id":"cfg-a","name":"Apply","createdAt":"t","updatedAt":"t","checkout":{"hasCheckout":true,"matchesConfig":false},"settings":{"schemaVersion":1,"branchName":"main"}}},
	  "createdAt":"t","updatedAt":"t"}`
	server := newCommunityPackageServer(t, packageJSONHandler(http.StatusOK, body))

	connection, err := server.client(t).GetPromotionConnection(context.Background(), "conn-2")
	if err != nil {
		t.Fatalf("GetPromotionConnection: %v", err)
	}
	if connection.Target.RepositoryPath != "org/repo" {
		t.Errorf("repository path = %q, want org/repo", connection.Target.RepositoryPath)
	}
	if checkout := connection.Configs.Apply.Checkout; checkout == nil || !checkout.HasCheckout || checkout.MatchesConfig {
		t.Errorf("apply checkout = %+v, want cloned and not matching", checkout)
	}
	assertLosslessJSON(t, connection, body)

	// The read-only path never travels back on a write.
	if _, err := server.client(t).UpdatePromotionConnection(context.Background(), "conn-2", UpdatePromotionConnectionRequest{
		Target: new(connection.Target.Document()),
	}); err != nil {
		t.Fatalf("UpdatePromotionConnection: %v", err)
	}
	_, raw := server.last(t)
	if strings.Contains(raw, "repositoryPath") || !strings.Contains(raw, "remoteUrl") {
		t.Errorf("update body = %s, want remoteUrl without repositoryPath", raw)
	}
}

func TestPromotionTargetDocumentRejectsReadOnlyFields(t *testing.T) {
	var doc PromotionTargetDocument
	err := json.Unmarshal([]byte(`{"schemaVersion":1,"remoteUrl":"x","repositoryPath":"org/repo"}`), &doc)
	if err == nil || !strings.Contains(err.Error(), "read-only") {
		t.Errorf("repositoryPath error = %v, want the read-only explanation", err)
	}
	if err := json.Unmarshal([]byte(`{"schemaVersion":1,"remoteUrl":"x","other":1}`), &doc); err == nil {
		t.Error("unknown target field: want a strict decode error")
	}
	if err := json.Unmarshal([]byte(`{"schemaVersion":1,"remoteUrl":"x"}`), &doc); err != nil || doc.RemoteURL != "x" {
		t.Errorf("valid target = %+v (error %v)", doc, err)
	}
}

func TestUpsertPromotionConfigReportsCheckout(t *testing.T) {
	const body = `{"id":"cfg-p","name":"Promote","createdAt":"t","updatedAt":"t",
	  "checkout":{"hasCheckout":false,"matchesConfig":false},
	  "settings":{"schemaVersion":1,"baseBranchName":"main","createBranchOnPromotion":true}}`
	server := newCommunityPackageServer(t, packageJSONHandler(http.StatusOK, body))
	config, err := server.client(t).UpsertPromotionPromoteConfig(context.Background(), "conn-1", UpsertPromotionPromoteConfigRequest{
		Settings: PromotionPromoteSettings{SchemaVersion: 1, BaseBranchName: "main", CreateBranchOnPromote: true},
	})
	if err != nil {
		t.Fatalf("UpsertPromotionPromoteConfig: %v", err)
	}
	if config.Checkout == nil || config.Checkout.HasCheckout {
		t.Errorf("checkout = %+v, want a reported, not-cloned checkout", config.Checkout)
	}
	assertLosslessJSON(t, config, body)
}
