package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/SomeoneWithOptions/n8n-cli/internal/config"
	"github.com/SomeoneWithOptions/n8n-cli/internal/n8n"
)

const (
	roleMappingResource    = "rolemappingrule"
	maxRoleMappingDocument = 1 << 20
)

func newRoleMappingCommand(opts Options) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "role-mapping",
		Short: "Manage identity-provider role-mapping rules",
		Long: "Manage the rules that turn identity-provider claims into n8n roles at login.\n" +
			"Start with 'list' to read the current rules, then create, update, move, or\n" +
			"delete them.\n\n" +
			"Rules are evaluated in order within their own type: instance rules grant a\n" +
			"global role, project rules grant a project role on the projects they name, and\n" +
			"each type has its own sequence starting at 0. Filter 'list' by --type to read a\n" +
			"single evaluation order. Rules only take effect for users signing in through\n" +
			"SAML or OIDC; see 'n8n saml get' and 'n8n oidc get' for the provider itself.",
		Example: "  n8n role-mapping list --type instance\n" +
			"  n8n role-mapping create --input rule.json\n" +
			"  n8n role-mapping update RULE_ID --input rule-update.json\n" +
			"  n8n role-mapping move RULE_ID --target-index 0\n" +
			"  n8n role-mapping delete RULE_ID --yes",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error { return cmd.Help() },
	}
	cmd.AddCommand(
		newRoleMappingListCommand(opts),
		newRoleMappingCreateCommand(opts),
		newRoleMappingMoveCommand(opts),
		newRoleMappingUpdateCommand(opts),
		newRoleMappingDeleteCommand(opts),
	)
	return cmd
}

type roleMappingListFlags struct {
	instance instanceFlags
	limit    int
	cursor   string
	all      bool
	ruleType string
	output   string
}

func newRoleMappingListCommand(opts Options) *cobra.Command {
	var f roleMappingListFlags
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List role-mapping rules",
		Long: "List role-mapping rules one cursor-paginated page at a time. Use --all to\n" +
			"follow every cursor, capped at 10,000 rules.\n\n" +
			"The order column is the rule's position within its own type, so an unfiltered\n" +
			"list contains two independent sequences that both start at 0. Pass\n" +
			"--type instance or --type project to read one evaluation order on its own.\n" +
			"Use a returned ID with update, move, or delete. Requires roleMappingRule:list.",
		Example: "  n8n role-mapping list\n" +
			"  n8n role-mapping list --type project --limit 50\n" +
			"  n8n role-mapping list --all --output json",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runRoleMappingList(cmd.Context(), opts, f)
		},
	}
	f.instance.register(cmd)
	cmd.Flags().IntVar(&f.limit, "limit", 0, "rules per API page, 1 to 250 (default: server default of 100)")
	cmd.Flags().StringVar(&f.cursor, "cursor", "", "pagination cursor returned by a previous list (default: the first page)")
	cmd.Flags().BoolVar(&f.all, "all", false, "follow every page instead of one (maximum 10,000 rules)")
	cmd.Flags().StringVar(&f.ruleType, "type", "", "return one evaluation order: instance or project (default: both)")
	cmd.Flags().StringVar(&f.output, "output", outputText, "output format: text or json (JSON is a page object with data and nextCursor)")
	return cmd
}

func runRoleMappingList(ctx context.Context, opts Options, f roleMappingListFlags) error {
	if err := validateOutput(f.output); err != nil {
		return err
	}
	listOpts := n8n.ListRoleMappingRulesOptions{
		ListOptions: n8n.ListOptions{Limit: f.limit, Cursor: f.cursor},
		Type:        n8n.RoleMappingRuleType(f.ruleType),
	}
	if err := listOpts.Validate(); err != nil {
		return err
	}
	client, resolution, err := opts.apiClient(f.instance)
	if err != nil {
		return err
	}
	var page n8n.Page[n8n.RoleMappingRule]
	if f.all {
		fetch := func(ctx context.Context, pagination n8n.ListOptions) (n8n.Page[n8n.RoleMappingRule], error) {
			pageOpts := listOpts
			pageOpts.ListOptions = pagination
			return client.ListRoleMappingRules(ctx, pageOpts)
		}
		page.Data, err = n8n.Collect(ctx, fetch, listOpts.ListOptions, n8n.DefaultCollectLimit)
	} else {
		page, err = client.ListRoleMappingRules(ctx, listOpts)
	}
	if err != nil {
		return roleMappingAPIError(err, resolution, "list")
	}
	if f.output == outputJSON {
		return writeJSON(opts.Streams.Out, page)
	}
	return writeRoleMappingListText(opts, resolution, page)
}

func writeRoleMappingListText(opts Options, resolution config.Resolution, page n8n.Page[n8n.RoleMappingRule]) error {
	tw := tabwriter.NewWriter(opts.Streams.Out, 0, 0, 2, ' ', 0)
	fmt.Fprintf(tw, "Instance:\t%s\n", resolution.URL)
	fmt.Fprintf(tw, "Rules:\t%d\n", len(page.Data))
	if len(page.Data) > 0 {
		fmt.Fprintln(tw, "\nTYPE\tORDER\tID\tROLE\tPROJECTS\tEXPRESSION")
		for _, rule := range page.Data {
			fmt.Fprintf(tw, "%s\t%d\t%s\t%s\t%s\t%s\n",
				rule.Type, rule.Order, rule.ID, rule.Role,
				roleMappingProjects(rule.ProjectIDs), strconv.Quote(rule.Expression))
		}
	}
	if page.NextCursor != "" {
		fmt.Fprintf(tw, "\nNext cursor:\t%s\n", page.NextCursor)
	}
	if err := tw.Flush(); err != nil {
		return err
	}
	if len(page.Data) == 0 {
		fmt.Fprintln(opts.Streams.Err, "No role-mapping rules were returned. Create one with 'n8n role-mapping create --input rule.json'.")
	}
	return nil
}

type roleMappingInputFlags struct {
	instance instanceFlags
	input    string
	output   string
}

func (f *roleMappingInputFlags) register(cmd *cobra.Command) {
	f.instance.register(cmd)
	cmd.Flags().StringVar(&f.input, "input", "", "role-mapping rule JSON file path, or - for stdin (required; maximum 1 MiB)")
	cmd.Flags().StringVar(&f.output, "output", outputText, "output format: text or json (JSON is the rule returned by the API)")
}

func newRoleMappingCreateCommand(opts Options) *cobra.Command {
	var f roleMappingInputFlags
	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a role-mapping rule",
		Long: "Create one rule from strict JSON supplied by --input. Required fields are\n" +
			"expression (the claim expression to match), role (an existing role slug, at\n" +
			"most 128 characters), and type (instance or project).\n\n" +
			"Optional order is the 0-based position within that type's evaluation order;\n" +
			"omitting it appends the rule to the end. Optional projectIds names the projects\n" +
			"a project rule grants its role on and is refused on an instance rule. Unknown\n" +
			"fields and trailing JSON documents are refused before transport. Run\n" +
			"'n8n role list' for valid slugs. Requires roleMappingRule:create.",
		Example: "  n8n role-mapping create --input rule.json\n" +
			"  printf '%s\\n' '{\"expression\":\"groups contains \\\"admins\\\"\",\"role\":\"global:admin\",\"type\":\"instance\"}' | n8n role-mapping create --input -\n" +
			"  printf '%s\\n' '{\"expression\":\"dept == \\\"ops\\\"\",\"role\":\"project:admin\",\"type\":\"project\",\"order\":0,\"projectIds\":[\"PROJECT_ID\"]}' | n8n role-mapping create --input - --output json",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runRoleMappingCreate(cmd.Context(), opts, f)
		},
	}
	f.register(cmd)
	return cmd
}

func runRoleMappingCreate(ctx context.Context, opts Options, f roleMappingInputFlags) error {
	if err := validateOutput(f.output); err != nil {
		return err
	}
	var request n8n.CreateRoleMappingRuleRequest
	if err := readRoleMappingDocument(opts, f.input, &request); err != nil {
		return err
	}
	if err := request.Validate(); err != nil {
		return err
	}
	client, resolution, err := opts.apiClient(f.instance)
	if err != nil {
		return err
	}
	rule, err := client.CreateRoleMappingRule(ctx, request)
	if err != nil {
		return roleMappingAPIError(err, resolution, "create")
	}
	return writeRoleMappingResult(opts, resolution, "Created", rule, f.output)
}

func newRoleMappingUpdateCommand(opts Options) *cobra.Command {
	var f roleMappingInputFlags
	cmd := &cobra.Command{
		Use:   "update <rule-id>",
		Short: "Update a role-mapping rule",
		Long: "Update one rule from strict JSON supplied by --input. This is a PATCH: only the\n" +
			"fields present are changed, and at least one of expression, role, or projectIds\n" +
			"is required.\n\n" +
			"A rule's type cannot be changed after creation, and its position is changed with\n" +
			"'n8n role-mapping move', so neither type nor order is accepted here. Pass\n" +
			"\"projectIds\": [] to clear a project rule's projects. Run\n" +
			"'n8n role-mapping list --output json' to read the current rule first.\n" +
			"Requires roleMappingRule:update.",
		Example: "  n8n role-mapping update RULE_ID --input rule-update.json\n" +
			"  printf '%s\\n' '{\"role\":\"global:member\"}' | n8n role-mapping update RULE_ID --input -\n" +
			"  printf '%s\\n' '{\"projectIds\":[]}' | n8n role-mapping update RULE_ID --input - --output json",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runRoleMappingUpdate(cmd.Context(), opts, args[0], f)
		},
	}
	f.register(cmd)
	return cmd
}

func runRoleMappingUpdate(ctx context.Context, opts Options, id string, f roleMappingInputFlags) error {
	if err := validateOutput(f.output); err != nil {
		return err
	}
	if err := validateRoleMappingRuleID(id); err != nil {
		return err
	}
	var request n8n.UpdateRoleMappingRuleRequest
	if err := readRoleMappingDocument(opts, f.input, &request); err != nil {
		return err
	}
	if err := request.Validate(); err != nil {
		return err
	}
	client, resolution, err := opts.apiClient(f.instance)
	if err != nil {
		return err
	}
	rule, err := client.UpdateRoleMappingRule(ctx, id, request)
	if err != nil {
		return roleMappingAPIError(err, resolution, "update")
	}
	return writeRoleMappingResult(opts, resolution, "Updated", rule, f.output)
}

type roleMappingMoveFlags struct {
	instance    instanceFlags
	targetIndex int
	output      string
}

func newRoleMappingMoveCommand(opts Options) *cobra.Command {
	var f roleMappingMoveFlags
	cmd := &cobra.Command{
		Use:   "move <rule-id>",
		Short: "Move a rule in its evaluation order",
		Long: "Move one rule to a new position in the evaluation order of its own type.\n" +
			"--target-index is the desired 0-based position among rules of that type; an\n" +
			"index beyond the last position moves the rule to the end.\n\n" +
			"Reordering changes which rule wins for a claim that several rules match, so it\n" +
			"changes the roles users receive at their next sign-in. The rules that shift\n" +
			"around the moved one keep a contiguous sequence. Run\n" +
			"'n8n role-mapping list --type TYPE' before and after to confirm the order.\n" +
			"Requires roleMappingRule:update.",
		Example: "  n8n role-mapping move RULE_ID --target-index 0\n" +
			"  n8n role-mapping move RULE_ID --target-index 99 --output json",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runRoleMappingMove(cmd.Context(), opts, args[0], f, cmd.Flags().Changed("target-index"))
		},
	}
	f.instance.register(cmd)
	cmd.Flags().IntVar(&f.targetIndex, "target-index", 0, "0-based position within the rule's own type (required; a larger index moves the rule to the end)")
	cmd.Flags().StringVar(&f.output, "output", outputText, "output format: text or json (JSON is the moved rule returned by the API)")
	return cmd
}

func runRoleMappingMove(ctx context.Context, opts Options, id string, f roleMappingMoveFlags, indexSet bool) error {
	if err := validateOutput(f.output); err != nil {
		return err
	}
	if err := validateRoleMappingRuleID(id); err != nil {
		return err
	}
	if !indexSet {
		return fmt.Errorf("--target-index is required: pass --target-index 0 to move the rule to the front of its type")
	}
	request := n8n.MoveRoleMappingRuleRequest{TargetIndex: f.targetIndex}
	if err := request.Validate(); err != nil {
		return err
	}
	client, resolution, err := opts.apiClient(f.instance)
	if err != nil {
		return err
	}
	rule, err := client.MoveRoleMappingRule(ctx, id, request)
	if err != nil {
		return roleMappingAPIError(err, resolution, "move")
	}
	return writeRoleMappingResult(opts, resolution, "Moved", rule, f.output)
}

type roleMappingDeleteFlags struct {
	instance instanceFlags
	yes      bool
	output   string
}

func newRoleMappingDeleteCommand(opts Options) *cobra.Command {
	var f roleMappingDeleteFlags
	cmd := &cobra.Command{
		Use:   "delete <rule-id>",
		Short: "Permanently delete a role-mapping rule",
		Long: "Permanently delete one role-mapping rule. Users already signed in keep the role\n" +
			"they were granted, but the rule stops applying at their next sign-in, so people\n" +
			"who depended on it can lose access. Deletion cannot be undone.\n\n" +
			"The remaining rules of the same type close the gap, so their order values stay\n" +
			"a contiguous sequence starting at 0. Interactive runs ask for confirmation;\n" +
			"non-interactive runs require --yes. Run 'n8n role-mapping list' first to find\n" +
			"the ID. Requires roleMappingRule:delete.",
		Example: "  n8n role-mapping delete RULE_ID\n" +
			"  n8n role-mapping delete RULE_ID --yes --output json",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runRoleMappingDelete(cmd.Context(), opts, args[0], f)
		},
	}
	f.instance.register(cmd)
	cmd.Flags().BoolVar(&f.yes, "yes", false, "confirm permanent deletion without prompting (required when stdin is not interactive)")
	cmd.Flags().StringVar(&f.output, "output", outputText, "output format: text or json (JSON is the deleted rule returned by the API)")
	return cmd
}

func runRoleMappingDelete(ctx context.Context, opts Options, id string, f roleMappingDeleteFlags) error {
	if err := validateOutput(f.output); err != nil {
		return err
	}
	if err := validateRoleMappingRuleID(id); err != nil {
		return err
	}
	client, resolution, err := opts.apiClient(f.instance)
	if err != nil {
		return err
	}
	question := fmt.Sprintf("Permanently delete role-mapping rule %q from %s? Users matching it lose that role at their next sign-in.", id, resolution.URL)
	if err := opts.confirmer(f.yes).Confirm(question); err != nil {
		return err
	}
	rule, err := client.DeleteRoleMappingRule(ctx, id)
	if err != nil {
		return roleMappingAPIError(err, resolution, "delete")
	}
	return writeRoleMappingResult(opts, resolution, "Deleted", rule, f.output)
}

func readRoleMappingDocument(opts Options, path string, dst any) error {
	if strings.TrimSpace(path) == "" {
		return fmt.Errorf("--input is required: pass a JSON file path or --input - for stdin")
	}
	reader := opts.Streams.In
	if path != "-" {
		file, err := os.Open(path)
		if err != nil {
			return fmt.Errorf("open role-mapping rule input %q: %w", path, err)
		}
		defer file.Close()
		reader = file
	}
	raw, err := io.ReadAll(io.LimitReader(reader, maxRoleMappingDocument+1))
	if err != nil {
		return fmt.Errorf("read role-mapping rule JSON: %w", err)
	}
	if len(raw) > maxRoleMappingDocument {
		return fmt.Errorf("role-mapping rule JSON exceeds %d bytes", maxRoleMappingDocument)
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(dst); err != nil {
		return fmt.Errorf("decode role-mapping rule JSON: %w", err)
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return fmt.Errorf("decode role-mapping rule JSON: multiple documents are not allowed")
		}
		return fmt.Errorf("decode role-mapping rule JSON: %w", err)
	}
	return nil
}

func writeRoleMappingResult(opts Options, resolution config.Resolution, action string, rule *n8n.RoleMappingRule, output string) error {
	if output == outputJSON {
		return writeJSON(opts.Streams.Out, rule)
	}
	tw := tabwriter.NewWriter(opts.Streams.Out, 0, 0, 2, ' ', 0)
	fmt.Fprintf(tw, "%s:\t%s\n", action, rule.ID)
	fmt.Fprintf(tw, "Type:\t%s\n", rule.Type)
	fmt.Fprintf(tw, "Order:\t%d\n", rule.Order)
	fmt.Fprintf(tw, "Role:\t%s\n", rule.Role)
	fmt.Fprintf(tw, "Expression:\t%s\n", strconv.Quote(rule.Expression))
	fmt.Fprintf(tw, "Projects:\t%s\n", roleMappingProjects(rule.ProjectIDs))
	fmt.Fprintf(tw, "Instance:\t%s\n", resolution.URL)
	if rule.CreatedAt != "" {
		fmt.Fprintf(tw, "Created:\t%s\n", rule.CreatedAt)
	}
	if rule.UpdatedAt != "" {
		fmt.Fprintf(tw, "Updated:\t%s\n", rule.UpdatedAt)
	}
	return tw.Flush()
}

func roleMappingProjects(ids []string) string {
	if len(ids) == 0 {
		return "-"
	}
	return strings.Join(ids, ",")
}

func validateRoleMappingRuleID(id string) error {
	if strings.TrimSpace(id) == "" {
		return fmt.Errorf("role-mapping rule ID is required")
	}
	if strings.TrimSpace(id) != id {
		return fmt.Errorf("role-mapping rule ID must not start or end with whitespace")
	}
	return nil
}

// roleMappingAPIError adds the remediation the API's own message cannot give:
// a 400 on these endpoints is usually an unknown role slug, an unknown project,
// or an expression the server refuses to parse.
func roleMappingAPIError(err error, resolution config.Resolution, action string) error {
	if n8n.StatusCodeOf(err) == 400 {
		return fmt.Errorf("%w; check the role slug with 'n8n role list', the project IDs with 'n8n project list', and the claim expression syntax", err)
	}
	if n8n.IsForbidden(err) {
		need := roleMappingScope(action)
		return forbiddenScopeError(err, resolution, "role-mapping "+action, need,
			"a 403 can also mean the instance is not licensed for role mapping.", roleMappingResource)
	}
	return apiError(err, resolution, roleMappingResource)
}

// roleMappingScope maps a command action to the scope the API requires for it.
func roleMappingScope(action string) scopeNeed {
	switch action {
	case "list":
		return allOf("roleMappingRule:list")
	case "create":
		return allOf("roleMappingRule:create")
	case "update", "move":
		return allOf("roleMappingRule:update")
	case "delete":
		return allOf("roleMappingRule:delete")
	}
	return scopeNeed{}
}
