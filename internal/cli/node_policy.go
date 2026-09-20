package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/SomeoneWithOptions/n8n-cli/internal/config"
	"github.com/SomeoneWithOptions/n8n-cli/internal/n8n"
)

const (
	nodePolicyResource    = "nodetypepolicy"
	maxNodePolicyDocument = 1 << 20
)

// nodePolicyUnavailable explains a 404 or 503 from this group. The routes need
// n8n 2.40.0 or later and the licensed type-availability-policies module, so an
// instance without either answers as if the resource does not exist.
const nodePolicyUnavailable = "node type policies need n8n 2.40.0 or later with the licensed type-availability-policies module enabled"

func newNodePolicyCommand(opts Options) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "node-policy",
		Short: "Manage node type availability policies",
		Long: "Read and replace the policies that decide which node types may be used, at\n" +
			"instance scope and per project. A policy is a default action plus an ordered\n" +
			"rule list; the first matching rule wins and the default applies to the rest.\n\n" +
			"Every action is a full replacement, so the workflow is get, edit, set: start\n" +
			"from 'get --output json', change the rules, and pass the whole document back.\n" +
			"The version field carries that round trip and a stale value is rejected with\n" +
			"409, which means someone else changed the policy first.\n\n" +
			"Requires nodeTypePolicy:manage, n8n 2.40.0 or later, and the licensed\n" +
			"type-availability-policies module. Instances without it answer 404 or 503.",
		Example: "  n8n node-policy instance get\n" +
			"  n8n node-policy instance get --output json > policy.json\n" +
			"  n8n node-policy instance set --input policy.json\n" +
			"  n8n node-policy project get L5fcpzBVjoU2PKFP",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error { return cmd.Help() },
	}
	cmd.AddCommand(
		newNodePolicyInstanceCommand(opts),
		newNodePolicyProjectCommand(opts),
	)
	return cmd
}

func newNodePolicyInstanceCommand(opts Options) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "instance",
		Short: "Manage the instance node type policy",
		Long: "Read or fully replace the instance-scope node type policy. Instance scope is the\n" +
			"only scope that accepts the delegate action, which hands a decision down to the\n" +
			"project policy instead of settling it.\n\n" +
			"An instance that was never configured reports no rules, an allow default, and\n" +
			"version 0; writing to it creates the policy document.",
		Example: "  n8n node-policy instance get\n" +
			"  n8n node-policy instance get --output json > policy.json\n" +
			"  n8n node-policy instance set --input policy.json",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error { return cmd.Help() },
	}
	cmd.AddCommand(
		newNodePolicyInstanceGetCommand(opts),
		newNodePolicyInstanceSetCommand(opts),
	)
	return cmd
}

func newNodePolicyProjectCommand(opts Options) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "project",
		Short: "Manage a project's node type policy",
		Long: "Read or fully replace one project's own node type policy. The reported policy is\n" +
			"the project's own document, not the result of combining it with the instance\n" +
			"policy, so a node type can still be blocked by instance scope.\n\n" +
			"Project scope accepts allow and deny only; delegate is instance-scope. A project\n" +
			"that was never configured reports no rules, an allow default, and version 0.",
		Example: "  n8n node-policy project get L5fcpzBVjoU2PKFP\n" +
			"  n8n node-policy project get L5fcpzBVjoU2PKFP --output json > project-policy.json\n" +
			"  n8n node-policy project set L5fcpzBVjoU2PKFP --input project-policy.json",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error { return cmd.Help() },
	}
	cmd.AddCommand(
		newNodePolicyProjectGetCommand(opts),
		newNodePolicyProjectSetCommand(opts),
	)
	return cmd
}

type nodePolicyGetFlags struct {
	instance instanceFlags
	output   string
}

func newNodePolicyInstanceGetCommand(opts Options) *cobra.Command {
	var f nodePolicyGetFlags
	cmd := &cobra.Command{
		Use:   "get",
		Short: "Get the instance node type policy",
		Long: "Get the composed instance node type policy: its default action and the rules of\n" +
			"every attached policy document, in evaluation order. Use --output json as the\n" +
			"starting point for a get-edit-set workflow; the version it reports is what 'set'\n" +
			"must send back.\n\n" +
			"A scope that was never configured reports no rules, defaultAction allow, and\n" +
			"version 0. Requires nodeTypePolicy:manage.",
		Example: "  n8n node-policy instance get\n" +
			"  n8n node-policy instance get --output json\n" +
			"  n8n node-policy instance get --output json > policy.json",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runNodePolicyGet(cmd.Context(), opts, f, "", n8n.NodeTypePolicyScopeInstance)
		},
	}
	f.instance.register(cmd)
	cmd.Flags().StringVar(&f.output, "output", outputText, "output format: text or json (JSON is the full document 'set' expects)")
	return cmd
}

func newNodePolicyProjectGetCommand(opts Options) *cobra.Command {
	var f nodePolicyGetFlags
	cmd := &cobra.Command{
		Use:   "get <project-id>",
		Short: "Get a project's node type policy",
		Long: "Get one project's own composed node type policy: its default action and the rules\n" +
			"of its policy document, in evaluation order. This is not the effective decision\n" +
			"for the project, because the instance policy is applied on top at evaluation.\n\n" +
			"A project that was never configured reports no rules, defaultAction allow, and\n" +
			"version 0. Use --output json as the input for 'set'. Requires\n" +
			"nodeTypePolicy:manage. Find project IDs with 'n8n project list'.",
		Example: "  n8n node-policy project get L5fcpzBVjoU2PKFP\n" +
			"  n8n node-policy project get L5fcpzBVjoU2PKFP --output json > project-policy.json",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runNodePolicyGet(cmd.Context(), opts, f, args[0], n8n.NodeTypePolicyScopeProject)
		},
	}
	f.instance.register(cmd)
	cmd.Flags().StringVar(&f.output, "output", outputText, "output format: text or json (JSON is the full document 'set' expects)")
	return cmd
}

func runNodePolicyGet(ctx context.Context, opts Options, f nodePolicyGetFlags, projectID string, scope n8n.NodeTypePolicyScope) error {
	if err := validateOutput(f.output); err != nil {
		return err
	}
	client, resolution, err := opts.apiClient(f.instance)
	if err != nil {
		return err
	}
	var policy *n8n.NodeTypePolicy
	if scope == n8n.NodeTypePolicyScopeProject {
		policy, err = client.GetProjectNodeTypePolicy(ctx, projectID)
	} else {
		policy, err = client.GetInstanceNodeTypePolicy(ctx)
	}
	if err != nil {
		return nodePolicyAPIError(err, resolution, "read", scope)
	}
	heading := "Instance node type policy"
	if scope == n8n.NodeTypePolicyScopeProject {
		heading = fmt.Sprintf("Node type policy of project %s", projectID)
	}
	return writeNodePolicy(opts, resolution, heading, policy, nil, f.output)
}

type nodePolicySetFlags struct {
	instance instanceFlags
	input    string
	yes      bool
	output   string
}

func newNodePolicyInstanceSetCommand(opts Options) *cobra.Command {
	var f nodePolicySetFlags
	cmd := &cobra.Command{
		Use:   "set",
		Short: "Fully replace the instance node type policy",
		Long: "Replace the instance default action and every rule of its policy document from\n" +
			"strict JSON. This is a PUT, not a patch: rules, defaultAction, and version are\n" +
			"all required, and any rule left out is deleted. Start from\n" +
			"'n8n node-policy instance get --output json'.\n\n" +
			"Actions are allow, deny, or delegate. Selectors are {\"kind\":\"name\"} for one node\n" +
			"type or {\"kind\":\"package\"} for every node type in a package. Rule ids must be\n" +
			"unique and rules are evaluated in list order.\n\n" +
			"version must equal the version last read; a stale value is rejected with 409 and\n" +
			"nothing changes. Blocking a node type stops workflows that use it, so\n" +
			"interactive runs ask for confirmation and non-interactive runs require --yes.\n" +
			"scopeId and warnings from a previous response are accepted and ignored.\n" +
			"Requires nodeTypePolicy:manage.",
		Example: "  n8n node-policy instance get --output json > policy.json\n" +
			"  n8n node-policy instance set --input policy.json\n" +
			"  n8n node-policy instance set --input policy.json --yes --output json\n" +
			"  n8n node-policy instance set --input - --yes < policy.json",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runNodePolicySet(cmd.Context(), opts, f, "", n8n.NodeTypePolicyScopeInstance)
		},
	}
	registerNodePolicySetFlags(cmd, &f)
	return cmd
}

func newNodePolicyProjectSetCommand(opts Options) *cobra.Command {
	var f nodePolicySetFlags
	cmd := &cobra.Command{
		Use:   "set <project-id>",
		Short: "Fully replace a project's node type policy",
		Long: "Replace one project's default action and every rule of its policy document from\n" +
			"strict JSON. This is a PUT, not a patch: rules, defaultAction, and version are\n" +
			"all required, and any rule left out is deleted. Start from\n" +
			"'n8n node-policy project get <project-id> --output json'.\n\n" +
			"Project scope accepts allow and deny only; delegate belongs to the instance\n" +
			"policy and is rejected before the request is sent. Selectors are\n" +
			"{\"kind\":\"name\"} or {\"kind\":\"package\"}, rule ids must be unique, and rules are\n" +
			"evaluated in list order.\n\n" +
			"version must equal the version last read; a stale value, or a policy document\n" +
			"shared with another scope, is rejected with 409 and nothing changes. Blocking a\n" +
			"node type stops workflows in the project that use it, so interactive runs ask\n" +
			"for confirmation and non-interactive runs require --yes. Requires\n" +
			"nodeTypePolicy:manage.",
		Example: "  n8n node-policy project get L5fcpzBVjoU2PKFP --output json > project-policy.json\n" +
			"  n8n node-policy project set L5fcpzBVjoU2PKFP --input project-policy.json\n" +
			"  n8n node-policy project set L5fcpzBVjoU2PKFP --input - --yes < project-policy.json",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runNodePolicySet(cmd.Context(), opts, f, args[0], n8n.NodeTypePolicyScopeProject)
		},
	}
	registerNodePolicySetFlags(cmd, &f)
	return cmd
}

func registerNodePolicySetFlags(cmd *cobra.Command, f *nodePolicySetFlags) {
	f.instance.register(cmd)
	cmd.Flags().StringVar(&f.input, "input", "", "full node type policy JSON file path, or - for stdin (required; maximum 1 MiB)")
	cmd.Flags().BoolVar(&f.yes, "yes", false, "confirm the full replacement without prompting (required when stdin is not interactive)")
	cmd.Flags().StringVar(&f.output, "output", outputText, "output format: text or json (JSON is the stored policy plus shadowed-rule warnings)")
}

func runNodePolicySet(ctx context.Context, opts Options, f nodePolicySetFlags, projectID string, scope n8n.NodeTypePolicyScope) error {
	if err := validateOutput(f.output); err != nil {
		return err
	}
	if f.input == "-" && !f.yes {
		return fmt.Errorf("--input - consumes stdin and cannot answer the replacement confirmation: pass --yes after reviewing the full policy")
	}
	request, err := readNodePolicyDocument(opts, f.input)
	if err != nil {
		return err
	}
	if err := request.Validate(scope); err != nil {
		return err
	}
	client, resolution, err := opts.apiClient(f.instance)
	if err != nil {
		return err
	}
	target := "the instance"
	if scope == n8n.NodeTypePolicyScopeProject {
		target = "project " + projectID
	}
	question := fmt.Sprintf("Replace the node type policy of %s on %s with %s and default action %s? Rules not in the document are deleted, and blocked node types stop the workflows that use them.",
		target, resolution.URL, nodePolicyRuleCount(len(request.Rules)), *request.DefaultAction)
	if err := opts.confirmer(f.yes).Confirm(question); err != nil {
		return err
	}
	var result *n8n.NodeTypePolicyWriteResult
	if scope == n8n.NodeTypePolicyScopeProject {
		result, err = client.ReplaceProjectNodeTypePolicy(ctx, projectID, request)
	} else {
		result, err = client.ReplaceInstanceNodeTypePolicy(ctx, request)
	}
	if err != nil {
		return nodePolicyAPIError(err, resolution, "replacement", scope)
	}
	heading := "Replaced instance node type policy"
	if scope == n8n.NodeTypePolicyScopeProject {
		heading = fmt.Sprintf("Replaced node type policy of project %s", projectID)
	}
	if f.output == outputJSON {
		return writeJSON(opts.Streams.Out, result)
	}
	return writeNodePolicy(opts, resolution, heading, &result.NodeTypePolicy, result.Warnings, f.output)
}

// nodePolicyDocument is the input shape of 'set'. It accepts the document 'get'
// and a previous 'set' print, so scopeId and warnings round trip without being
// sent: both are server-owned.
type nodePolicyDocument struct {
	ScopeID       *string                     `json:"scopeId"`
	Rules         []n8n.NodeTypePolicyRule    `json:"rules"`
	DefaultAction *n8n.NodeTypePolicyAction   `json:"defaultAction"`
	Version       *int                        `json:"version"`
	Warnings      []n8n.NodeTypePolicyWarning `json:"warnings"`
}

func readNodePolicyDocument(opts Options, path string) (n8n.ReplaceNodeTypePolicyRequest, error) {
	var request n8n.ReplaceNodeTypePolicyRequest
	if strings.TrimSpace(path) == "" {
		return request, fmt.Errorf("--input is required: start from 'n8n node-policy ... get --output json' and pass the edited file, or --input - for stdin")
	}
	reader := opts.Streams.In
	if path != "-" {
		file, err := os.Open(path)
		if err != nil {
			return request, fmt.Errorf("open node type policy input %q: %w", path, err)
		}
		defer file.Close()
		reader = file
	}
	raw, err := io.ReadAll(io.LimitReader(reader, maxNodePolicyDocument+1))
	if err != nil {
		return request, fmt.Errorf("read node type policy JSON: %w", err)
	}
	if len(raw) > maxNodePolicyDocument {
		return request, fmt.Errorf("node type policy JSON exceeds %d bytes", maxNodePolicyDocument)
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	var document nodePolicyDocument
	if err := decoder.Decode(&document); err != nil {
		return request, fmt.Errorf("decode node type policy JSON: %w", err)
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return request, fmt.Errorf("decode node type policy JSON: multiple documents are not allowed")
		}
		return request, fmt.Errorf("decode node type policy JSON: %w", err)
	}
	return n8n.ReplaceNodeTypePolicyRequest{
		Rules:         document.Rules,
		DefaultAction: document.DefaultAction,
		Version:       document.Version,
	}, nil
}

func writeNodePolicy(opts Options, resolution config.Resolution, heading string, policy *n8n.NodeTypePolicy, warnings []n8n.NodeTypePolicyWarning, output string) error {
	if output == outputJSON {
		return writeJSON(opts.Streams.Out, policy)
	}
	tw := tabwriter.NewWriter(opts.Streams.Out, 0, 0, 2, ' ', 0)
	fmt.Fprintf(tw, "%s:\t%s\n", heading, resolution.URL)
	if policy.Configured() {
		fmt.Fprintf(tw, "Scope document:\t%s\n", *policy.ScopeID)
	} else {
		fmt.Fprintf(tw, "Scope document:\tnone (never configured; every node type is allowed)\n")
	}
	fmt.Fprintf(tw, "Default action:\t%s\n", policy.DefaultAction)
	fmt.Fprintf(tw, "Version:\t%d\n", policy.Version)
	fmt.Fprintf(tw, "Rules:\t%d\n", len(policy.Rules))
	if len(policy.Rules) > 0 {
		fmt.Fprintln(tw, "\nORDER\tID\tACTION\tSELECTOR")
		for i, rule := range policy.Rules {
			fmt.Fprintf(tw, "%d\t%s\t%s\t%s=%s\n", i+1, rule.ID, rule.Action, rule.Selector.Kind, rule.Selector.Value)
		}
	}
	for _, warning := range warnings {
		fmt.Fprintf(tw, "\nWarning: rule %s never matches; rule %s already decides those node types.\n", warning.RuleID, warning.ShadowedByRuleID)
	}
	return tw.Flush()
}

func nodePolicyRuleCount(rules int) string {
	if rules == 1 {
		return "1 rule"
	}
	return fmt.Sprintf("%d rules", rules)
}

func nodePolicyAPIError(err error, resolution config.Resolution, action string, scope n8n.NodeTypePolicyScope) error {
	switch {
	case n8n.IsNotFound(err), n8n.IsStatus(err, http.StatusServiceUnavailable):
		return fmt.Errorf("%s does not serve node type policies (%d): %s; run %s to see what this instance offers",
			resolution.URL, n8n.StatusCodeOf(err), nodePolicyUnavailable, discoverHint(nodePolicyResource))
	case n8n.IsForbidden(err):
		return forbiddenScopeError(err, resolution, "node type policy "+action, allOf("nodeTypePolicy:manage"), "", nodePolicyResource)
	case n8n.IsConflict(err) && scope == n8n.NodeTypePolicyScopeProject:
		return fmt.Errorf("%s refused the node type policy replacement (409): the version was changed by someone else since it was read, or the policy document is shared with another scope, so nothing changed; run 'n8n node-policy project get' again and re-apply the edit",
			resolution.URL)
	case n8n.IsConflict(err):
		return fmt.Errorf("%s refused the node type policy replacement (409): the version was changed by someone else since it was read, so nothing changed; run 'n8n node-policy instance get' again and re-apply the edit",
			resolution.URL)
	case n8n.IsStatus(err, http.StatusBadRequest):
		return fmt.Errorf("%s rejected the node type policy replacement (400): send rules, defaultAction, and version together, use unique rule ids, and select node types with {\"kind\":\"name\"} or {\"kind\":\"package\"}",
			resolution.URL)
	case n8n.IsStatus(err, http.StatusUnsupportedMediaType):
		return fmt.Errorf("%s rejected the node type policy content type (415): the document must be JSON", resolution.URL)
	default:
		return apiError(err, resolution, nodePolicyResource)
	}
}
