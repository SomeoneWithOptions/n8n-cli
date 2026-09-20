package cli

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/SomeoneWithOptions/n8n-cli/internal/config"
	"github.com/SomeoneWithOptions/n8n-cli/internal/n8n"
)

const variableResource = "variables"

func newVariableCommand(opts Options) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "variable",
		Short: "Manage instance and project variables",
		Long: "Manage variables available to n8n expressions. Start with 'list' to inspect\n" +
			"visible global and project-scoped variables, then create, update or delete them.\n\n" +
			"List output contains variable values because retrieval is the requested operation.\n" +
			"Create and update never repeat a submitted value in success or error diagnostics.\n" +
			"Keys are unique within their scope. Omitting --project-id on a write selects global\n" +
			"scope; project variables need the relevant project access on the instance.",
		Example: "  n8n variable list\n" +
			"  n8n variable create API_HOST --value https://api.example.com\n" +
			"  n8n variable update VARIABLE_ID --key API_HOST --value https://api.internal\n" +
			"  n8n variable delete VARIABLE_ID --yes",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error { return cmd.Help() },
	}
	cmd.AddCommand(
		newVariableListCommand(opts),
		newVariableCreateCommand(opts),
		newVariableUpdateCommand(opts),
		newVariableDeleteCommand(opts),
	)
	return cmd
}

type variableListFlags struct {
	instance  instanceFlags
	limit     int
	cursor    string
	all       bool
	projectID string
	state     string
	output    string
}

func newVariableListCommand(opts Options) *cobra.Command {
	var f variableListFlags
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List variables and their values",
		Long: "List variables visible to the selected credential, including their values, one\n" +
			"cursor-paginated page at a time. Use --project-id to select one project or\n" +
			"--state empty to find variables with an empty value. Use --all to follow every\n" +
			"cursor, capped at 10,000 variables.\n\n" +
			"Values are requested output: text prints quoted values and JSON preserves exact\n" +
			"strings. Keep output and redirected files private. Requires variable:list.",
		Example: "  n8n variable list\n" +
			"  n8n variable list --project-id PROJECT_ID --limit 50\n" +
			"  n8n variable list --state empty --output json\n" +
			"  n8n variable list --all --output json",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runVariableList(cmd.Context(), opts, f)
		},
	}
	f.instance.register(cmd)
	cmd.Flags().IntVar(&f.limit, "limit", 0, "variables per API page, 1 to 250 (default: server default of 100)")
	cmd.Flags().StringVar(&f.cursor, "cursor", "", "pagination cursor returned by a previous list (default: the first page)")
	cmd.Flags().BoolVar(&f.all, "all", false, "follow every page instead of one (maximum 10,000 variables)")
	cmd.Flags().StringVar(&f.projectID, "project-id", "", "return variables for this project ID (default: all visible scopes)")
	cmd.Flags().StringVar(&f.state, "state", "", "filter by value state: empty (default: all values)")
	cmd.Flags().StringVar(&f.output, "output", outputText, "output format: text or json (JSON is a page object containing exact variable values)")
	return cmd
}

func runVariableList(ctx context.Context, opts Options, f variableListFlags) error {
	if err := validateOutput(f.output); err != nil {
		return err
	}
	listOpts := n8n.ListVariablesOptions{
		ListOptions: n8n.ListOptions{Limit: f.limit, Cursor: f.cursor},
		ProjectID:   f.projectID,
		State:       n8n.VariableState(f.state),
	}
	if err := listOpts.Validate(); err != nil {
		return err
	}
	client, resolution, err := opts.apiClient(f.instance)
	if err != nil {
		return err
	}
	var page n8n.Page[n8n.Variable]
	if f.all {
		fetch := func(ctx context.Context, pagination n8n.ListOptions) (n8n.Page[n8n.Variable], error) {
			pageOpts := listOpts
			pageOpts.ListOptions = pagination
			return client.ListVariables(ctx, pageOpts)
		}
		page.Data, err = n8n.Collect(ctx, fetch, listOpts.ListOptions, n8n.DefaultCollectLimit)
	} else {
		page, err = client.ListVariables(ctx, listOpts)
	}
	if err != nil {
		return variableAPIError(err, resolution, "", "", "list")
	}
	if f.output == outputJSON {
		return writeJSON(opts.Streams.Out, page)
	}
	return writeVariableListText(opts, resolution, page)
}

func writeVariableListText(opts Options, resolution config.Resolution, page n8n.Page[n8n.Variable]) error {
	tw := tabwriter.NewWriter(opts.Streams.Out, 0, 0, 2, ' ', 0)
	fmt.Fprintf(tw, "Instance:\t%s\n", resolution.URL)
	fmt.Fprintf(tw, "Variables:\t%d\n", len(page.Data))
	if len(page.Data) > 0 {
		fmt.Fprintln(tw, "\nID\tKEY\tVALUE\tTYPE\tSCOPE")
		for _, variable := range page.Data {
			fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\n",
				variable.ID, variable.Key, strconv.Quote(variable.Value), variable.Type, variableScope(variable.Project))
		}
	}
	if page.NextCursor != "" {
		fmt.Fprintf(tw, "\nNext cursor:\t%s\n", page.NextCursor)
	}
	if err := tw.Flush(); err != nil {
		return err
	}
	if len(page.Data) == 0 {
		fmt.Fprintln(opts.Streams.Err, "No variables were returned. Create one with 'n8n variable create KEY --value VALUE'.")
	}
	return nil
}

func variableScope(project *n8n.VariableProject) string {
	if project == nil {
		return "global"
	}
	if project.Name == "" {
		return project.ID
	}
	if project.ID == "" {
		return project.Name
	}
	return fmt.Sprintf("%s (%s)", project.Name, project.ID)
}

type variableWriteFlags struct {
	instance  instanceFlags
	value     string
	projectID string
	output    string
}

func (f *variableWriteFlags) register(cmd *cobra.Command) {
	f.instance.register(cmd)
	cmd.Flags().StringVar(&f.value, "value", "", "complete variable value; pass an empty string to store an empty value (required, not repeated in diagnostics)")
	cmd.Flags().StringVar(&f.projectID, "project-id", "", "project scope ID (default: global scope; on update this replaces current scope)")
	cmd.Flags().StringVar(&f.output, "output", outputText, "output format: text or json (acknowledgement excludes the submitted value)")
}

func newVariableCreateCommand(opts Options) *cobra.Command {
	var f variableWriteFlags
	cmd := &cobra.Command{
		Use:   "create <key>",
		Short: "Create a variable",
		Long: "Create one variable with a complete key and value. Omit --project-id for a\n" +
			"global variable, or pass a project ID for project scope. A key must be unique\n" +
			"within that scope; a duplicate fails with a conflict.\n\n" +
			"--value is required, though an explicitly empty value is valid. The submitted\n" +
			"value is sent to n8n but never repeated in CLI diagnostics or acknowledgement\n" +
			"output. Requires variable:create.",
		Example: "  n8n variable create API_HOST --value https://api.example.com\n" +
			"  n8n variable create OPTIONAL_VALUE --value \"\"\n" +
			"  n8n variable create API_HOST --value https://api.example.com --project-id PROJECT_ID --output json",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runVariableCreate(cmd.Context(), opts, args[0], f, cmd.Flags().Changed("value"))
		},
	}
	f.register(cmd)
	return cmd
}

func runVariableCreate(ctx context.Context, opts Options, key string, f variableWriteFlags, valueSet bool) error {
	if err := validateVariableWrite(key, f, valueSet); err != nil {
		return err
	}
	client, resolution, err := opts.apiClient(f.instance)
	if err != nil {
		return err
	}
	request := n8n.VariableRequest{Key: key, Value: f.value, ProjectID: f.projectID}
	if err := client.CreateVariable(ctx, request); err != nil {
		return variableAPIError(err, resolution, key, f.projectID, "create")
	}
	return writeVariableMutation(opts, resolution, variableMutation{Action: "created", Key: key, ProjectID: f.projectID}, f.output)
}

type variableUpdateFlags struct {
	write variableWriteFlags
	key   string
}

func newVariableUpdateCommand(opts Options) *cobra.Command {
	var f variableUpdateFlags
	cmd := &cobra.Command{
		Use:   "update <variable-id>",
		Short: "Fully replace a variable",
		Long: "Fully replace one variable by ID. This is a PUT, so --key and --value are both\n" +
			"required even when one is unchanged. Omitting --project-id replaces the current\n" +
			"scope with global scope; pass its project ID to keep or select project scope.\n\n" +
			"A key already used in the resulting scope fails with a conflict. An explicitly\n" +
			"empty value is valid. Submitted values never appear in diagnostics or success\n" +
			"acknowledgements. Requires variable:update.",
		Example: "  n8n variable update VARIABLE_ID --key API_HOST --value https://api.internal\n" +
			"  n8n variable update VARIABLE_ID --key API_HOST --value \"\" --project-id PROJECT_ID\n" +
			"  n8n variable update VARIABLE_ID --key API_HOST --value VALUE --output json",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runVariableUpdate(cmd.Context(), opts, args[0], f,
				cmd.Flags().Changed("key"), cmd.Flags().Changed("value"))
		},
	}
	f.write.register(cmd)
	cmd.Flags().StringVar(&f.key, "key", "", "complete replacement key (required, even when unchanged)")
	return cmd
}

func runVariableUpdate(ctx context.Context, opts Options, id string, f variableUpdateFlags, keySet, valueSet bool) error {
	if err := validateVariableID(id); err != nil {
		return err
	}
	if !keySet {
		return fmt.Errorf("--key is required: update fully replaces the variable")
	}
	if err := validateVariableWrite(f.key, f.write, valueSet); err != nil {
		return err
	}
	client, resolution, err := opts.apiClient(f.write.instance)
	if err != nil {
		return err
	}
	request := n8n.VariableRequest{Key: f.key, Value: f.write.value, ProjectID: f.write.projectID}
	if err := client.UpdateVariable(ctx, id, request); err != nil {
		return variableAPIError(err, resolution, f.key, f.write.projectID, "update")
	}
	return writeVariableMutation(opts, resolution, variableMutation{
		Action: "updated", ID: id, Key: f.key, ProjectID: f.write.projectID,
	}, f.write.output)
}

type variableDeleteFlags struct {
	instance instanceFlags
	yes      bool
	output   string
}

func newVariableDeleteCommand(opts Options) *cobra.Command {
	var f variableDeleteFlags
	cmd := &cobra.Command{
		Use:   "delete <variable-id>",
		Short: "Permanently delete a variable",
		Long: "Permanently delete one variable by ID. Workflows are not deleted, but expressions\n" +
			"that rely on the variable may stop resolving or behave differently. The deletion\n" +
			"cannot be undone.\n\n" +
			"Interactive runs ask for confirmation. Non-interactive runs require --yes. Use\n" +
			"'n8n variable list' first to find the ID. Requires variable:delete.",
		Example: "  n8n variable delete VARIABLE_ID\n" +
			"  n8n variable delete VARIABLE_ID --yes --output json",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runVariableDelete(cmd.Context(), opts, args[0], f)
		},
	}
	f.instance.register(cmd)
	cmd.Flags().BoolVar(&f.yes, "yes", false, "confirm permanent deletion without prompting (required when stdin is not interactive)")
	cmd.Flags().StringVar(&f.output, "output", outputText, "output format: text or json (acknowledgement contains only the deleted variable ID)")
	return cmd
}

func runVariableDelete(ctx context.Context, opts Options, id string, f variableDeleteFlags) error {
	if err := validateOutput(f.output); err != nil {
		return err
	}
	if err := validateVariableID(id); err != nil {
		return err
	}
	client, resolution, err := opts.apiClient(f.instance)
	if err != nil {
		return err
	}
	question := fmt.Sprintf("Permanently delete variable %q from %s? Workflows that use it may stop working.", id, resolution.URL)
	if err := opts.confirmer(f.yes).Confirm(question); err != nil {
		return err
	}
	if err := client.DeleteVariable(ctx, id); err != nil {
		return variableAPIError(err, resolution, id, "", "delete")
	}
	return writeVariableMutation(opts, resolution, variableMutation{Action: "deleted", ID: id}, f.output)
}

type variableMutation struct {
	Action    string `json:"action"`
	ID        string `json:"id,omitempty"`
	Key       string `json:"key,omitempty"`
	Scope     string `json:"scope,omitempty"`
	ProjectID string `json:"projectId,omitempty"`
}

func writeVariableMutation(opts Options, resolution config.Resolution, result variableMutation, output string) error {
	if result.Key != "" {
		result.Scope = "global"
		if result.ProjectID != "" {
			result.Scope = "project"
		}
	}
	if output == outputJSON {
		return writeJSON(opts.Streams.Out, result)
	}
	tw := tabwriter.NewWriter(opts.Streams.Out, 0, 0, 2, ' ', 0)
	fmt.Fprintf(tw, "%s:\t%s\n", capitalizeAction(result.Action), firstVariableResult(result.Key, result.ID))
	if result.ID != "" && result.Key != "" {
		fmt.Fprintf(tw, "ID:\t%s\n", result.ID)
	}
	if result.Scope != "" {
		fmt.Fprintf(tw, "Scope:\t%s\n", result.Scope)
	}
	if result.ProjectID != "" {
		fmt.Fprintf(tw, "Project:\t%s\n", result.ProjectID)
	}
	fmt.Fprintf(tw, "Instance:\t%s\n", resolution.URL)
	return tw.Flush()
}

// capitalizeAction uppercases the first letter of a mutation verb. An empty
// action yields an empty string rather than panicking on a zero-length slice.
func capitalizeAction(action string) string {
	if action == "" {
		return ""
	}
	return strings.ToUpper(action[:1]) + action[1:]
}

func firstVariableResult(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

func validateVariableWrite(key string, f variableWriteFlags, valueSet bool) error {
	if err := validateOutput(f.output); err != nil {
		return err
	}
	if err := validateVariableKey(key); err != nil {
		return err
	}
	if !valueSet {
		return fmt.Errorf("--value is required: pass --value \"\" to store an empty value")
	}
	return validateVariableProjectID(f.projectID)
}

func validateVariableID(id string) error { return validateVariableArgument("ID", id, true) }

func validateVariableKey(key string) error { return validateVariableArgument("key", key, true) }

func validateVariableProjectID(id string) error {
	return validateVariableArgument("project ID", id, false)
}

func validateVariableArgument(field, value string, required bool) error {
	if required && strings.TrimSpace(value) == "" {
		return fmt.Errorf("variable %s is required", field)
	}
	if strings.TrimSpace(value) != value {
		return fmt.Errorf("variable %s must not start or end with whitespace", field)
	}
	return nil
}

// variableAPIError adds duplicate-key remediation without ever including the
// submitted value. Other API errors were already stripped of server-controlled
// details by the client write methods.
func variableAPIError(err error, resolution config.Resolution, key, projectID, action string) error {
	if n8n.IsConflict(err) {
		scope := "global scope"
		if projectID != "" {
			scope = fmt.Sprintf("project %q", projectID)
		}
		return fmt.Errorf("%s rejected variable key %q in %s (409): keys must be unique within their scope; choose another key or run 'n8n variable list' to find the existing variable", resolution.URL, key, scope)
	}
	if n8n.IsForbidden(err) {
		need := variableActionScope(action)
		return forbiddenScopeError(err, resolution, "variable "+action, need,
			"a 403 can also mean the instance is not licensed for variables.", variableResource)
	}
	return apiError(err, resolution, variableResource)
}

// variableActionScope maps a command action to the scope the API requires for it.
func variableActionScope(action string) scopeNeed {
	switch action {
	case "list":
		return allOf("variable:list")
	case "create":
		return allOf("variable:create")
	case "update":
		return allOf("variable:update")
	case "delete":
		return allOf("variable:delete")
	}
	return scopeNeed{}
}
