package cli

import (
	"net/http"
	"slices"
	"strings"
	"testing"

	"github.com/SomeoneWithOptions/n8n-cli/internal/config"
	"github.com/SomeoneWithOptions/n8n-cli/internal/n8n"

	"github.com/SomeoneWithOptions/n8n-cli/internal/coverage"
)

// TestScopeMappersMatchManifest guards the one way these messages can lie: a
// scope the CLI names that the API does not require. The manifest is the
// committed record of what each operation needs, so every phrase a 403 prints
// has to name every scope the manifest records for that command.
func TestScopeMappersMatchManifest(t *testing.T) {
	manifest, err := coverage.Load()
	if err != nil {
		t.Fatalf("load manifest: %v", err)
	}
	scopes := make(map[string]string, len(manifest.Operations))
	for _, op := range manifest.Operations {
		scopes[op.Command] = op.Scope
	}

	cases := []struct {
		command string
		need    scopeNeed
	}{
		{"n8n tag list", tagScope("list")},
		{"n8n tag get", tagScope("read")},
		{"n8n tag create", tagScope("create")},
		{"n8n tag update", tagScope("update")},
		{"n8n tag delete", tagScope("delete")},

		{"n8n execution list", executionScope("list")},
		{"n8n execution get", executionScope("read")},
		{"n8n execution delete", executionScope("delete")},
		{"n8n execution retry", executionScope("retry")},
		{"n8n execution stop", executionScope("stop")},
		{"n8n execution stop-many", executionScope("stop")},
		{"n8n execution tag list", executionScope("tag read")},
		{"n8n execution tag set", executionScope("tag update")},

		{"n8n variable list", variableActionScope("list")},
		{"n8n variable create", variableActionScope("create")},
		{"n8n variable update", variableActionScope("update")},
		{"n8n variable delete", variableActionScope("delete")},

		{"n8n role list", roleScope("list")},
		{"n8n role get", roleScope("read")},
		{"n8n role create", roleScope("create")},
		{"n8n role update", roleScope("update")},
		{"n8n role delete", roleScope("delete")},

		{"n8n role-mapping list", roleMappingScope("list")},
		{"n8n role-mapping create", roleMappingScope("create")},
		{"n8n role-mapping update", roleMappingScope("update")},
		{"n8n role-mapping move", roleMappingScope("move")},
		{"n8n role-mapping delete", roleMappingScope("delete")},

		{"n8n community-package list", communityPackageScope("list")},
		{"n8n community-package install", communityPackageScope("install")},
		{"n8n community-package update", communityPackageScope("update")},
		{"n8n community-package uninstall", communityPackageScope("uninstall")},

		{"n8n credential list", credentialScope("list")},
		{"n8n credential get", credentialScope("get")},
		{"n8n credential test", credentialScope("test")},
		{"n8n credential create", credentialScope("create")},
		{"n8n credential update", credentialScope("update")},
		{"n8n credential delete", credentialScope("delete")},
		{"n8n credential transfer", credentialScope("transfer")},

		{"n8n data-table list", dataTableScope("list")},
		{"n8n data-table get", dataTableScope("read")},
		{"n8n data-table create", dataTableScope("create")},
		{"n8n data-table update", dataTableScope("update")},
		{"n8n data-table delete", dataTableScope("delete")},
		{"n8n data-table column list", dataTableScope("column list")},
		{"n8n data-table column create", dataTableScope("column create")},
		{"n8n data-table column update", dataTableScope("column update")},
		{"n8n data-table column delete", dataTableScope("column delete")},
		{"n8n data-table row list", dataTableScope("row list")},
		{"n8n data-table row insert", dataTableScope("row insert")},
		{"n8n data-table row update", dataTableScope("row update")},
		{"n8n data-table row upsert", dataTableScope("row upsert")},
		{"n8n data-table row delete", dataTableScope("row delete")},
		{"n8n data-table row clear", dataTableScope("row clear")},

		{"n8n workflow list", workflowScope("list")},
		{"n8n workflow get", workflowScope("read")},
		{"n8n workflow get", workflowScope("get")},
		{"n8n workflow create", workflowScope("create")},
		{"n8n workflow update", workflowScope("update")},
		{"n8n workflow delete", workflowScope("delete")},
		{"n8n workflow archive", workflowScope("archive")},
		{"n8n workflow unarchive", workflowScope("unarchive")},
		{"n8n workflow publish", workflowScope("publish")},
		{"n8n workflow unpublish", workflowScope("unpublish")},
		{"n8n workflow transfer", workflowScope("transfer")},
		{"n8n workflow tag list", workflowScope("tag read")},
		{"n8n workflow tag set", workflowScope("tag update")},

		{"n8n project list", projectScope("list")},
		{"n8n project create", projectScope("create")},
		{"n8n project update", projectScope("update")},
		{"n8n project delete", projectScope("delete")},
		{"n8n project user add", projectScope("member add")},
		{"n8n project user remove", projectScope("member removal")},
		{"n8n project user role set", projectScope("member role change")},

		{"n8n user list", userScope("list")},
		{"n8n user get", userScope("get")},
		{"n8n user create", userScope("create")},
		{"n8n user delete", userScope("delete")},
		{"n8n user role set", userScope("change role")},

		{"n8n evaluation list", evaluationScope("list")},
		{"n8n evaluation get", evaluationScope("read")},
		{"n8n evaluation create", evaluationScope("create")},
		{"n8n evaluation cancel", evaluationScope("cancel")},
		{"n8n evaluation case list", evaluationScope("list cases for")},
	}

	for _, tc := range cases {
		scope, ok := scopes[tc.command]
		if !ok {
			t.Errorf("%s: no manifest operation for this command", tc.command)
			continue
		}
		if !tc.need.known() {
			t.Errorf("%s: no scope mapped, the 403 would stay vague", tc.command)
			continue
		}
		want := strings.Split(scope, ",")
		for i := range want {
			want[i] = strings.TrimSpace(want[i])
		}
		got := slices.Clone(tc.need.scopes)
		slices.Sort(got)
		slices.Sort(want)
		if !slices.Equal(got, want) {
			t.Errorf("%s: scopes %v, want the manifest scopes %v", tc.command, got, want)
		}
	}
}

// TestScopeMappersFallBackOnUnknownActions keeps an unmapped action vague
// instead of naming a scope the API never asked for.
func TestScopeMappersFallBackOnUnknownActions(t *testing.T) {
	mappers := map[string]func(string) scopeNeed{
		"tag":              tagScope,
		"execution":        executionScope,
		"variable":         variableActionScope,
		"role":             roleScope,
		"role-mapping":     roleMappingScope,
		"communityPackage": communityPackageScope,
		"credential":       credentialScope,
		"data-table":       dataTableScope,
		"workflow":         workflowScope,
		"project":          projectScope,
		"user":             userScope,
	}
	for name, mapper := range mappers {
		if got := mapper("no-such-action"); got.known() {
			t.Errorf("%sScope(unknown) = %v, want no scope claim", name, got.scopes)
		}
	}
}

// TestForbiddenScopeErrorKeepsServerDetail checks the two things the 403 report
// promises beyond the scope: the server's own words are quoted, and the API
// error stays reachable for callers that inspect it instead of printing it.
func TestForbiddenScopeErrorKeepsServerDetail(t *testing.T) {
	apiErr := &n8n.APIError{
		StatusCode: http.StatusForbidden,
		Status:     "403 Forbidden",
		Method:     http.MethodGet,
		Path:       "/workflows",
		Message:    "missing scope",
		Code:       "FORBIDDEN",
		RequestID:  "req-42",
	}
	err := forbiddenScopeError(apiErr, config.Resolution{URL: "https://n8n.example.com"},
		"workflow list", allOf("workflow:list"), "a 403 can also mean no access to the project.", "workflow")

	got := err.Error()
	for _, want := range []string{
		"https://n8n.example.com denied workflow list (403)",
		"missing scope:\n    workflow:list",
		"server said: GET /workflows: 403 Forbidden [FORBIDDEN]: missing scope (request id req-42)",
		"run 'n8n discover --resource workflow' to inspect access",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("error missing %q:\n%s", want, got)
		}
	}
	if !n8n.IsForbidden(err) {
		t.Error("the API error no longer survives wrapping")
	}
}
