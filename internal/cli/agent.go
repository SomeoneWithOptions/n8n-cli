package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"slices"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/SomeoneWithOptions/n8n-cli/internal/config"
	"github.com/SomeoneWithOptions/n8n-cli/internal/n8n"
)

const (
	agentResource    = "agent"
	maxAgentDocument = 1 << 20
)

func newAgentCommand(opts Options) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "agent",
		Short: "Manage Agents",
		Long: "Manage the Agents of an n8n instance (n8n 2.49 and later).\n\n" +
			"An Agent lives in a project and holds a configuration (name, model,\n" +
			"instructions), custom tools, and once published, channel integrations.\n" +
			"Start with 'list' to find the agent ID, then get, create, or delete.\n" +
			"Instances before n8n 2.49 answer 404. Moving Agents between instances\n" +
			"belongs to 'n8n package export' and 'n8n package import'.",
		Example: "  n8n agent list --project-id PROJECT_ID\n" +
			"  n8n agent get AGENT_ID --output json > agent.json\n" +
			"  n8n agent create --name Support --project-id PROJECT_ID\n" +
			"  n8n agent delete AGENT_ID --yes",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error { return cmd.Help() },
	}
	cmd.AddCommand(
		newAgentListCommand(opts),
		newAgentGetCommand(opts),
		newAgentCreateCommand(opts),
		newAgentDeleteCommand(opts),
	)
	return cmd
}

type agentListFlags struct {
	instance  instanceFlags
	limit     int
	cursor    string
	all       bool
	projectID string
	view      listViewFlags
	output    string
}

func newAgentListCommand(opts Options) *cobra.Command {
	var f agentListFlags
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List Agents",
		Long: "List the Agents in the projects where you hold agent:list, one cursor page at\n" +
			"a time. --project-id narrows the list to one project ('n8n project list'\n" +
			"finds it). Text shows ID, name, project, whether the Agent is published\n" +
			"and archived, and the update time. Requires agent:list and n8n 2.49 or\n" +
			"later.\n\n" +
			scriptViewHelp("agent", "ID and name") + "\n\n" +
			cursorCollectionHelp,
		Example: "  n8n agent list\n" +
			"  n8n agent list --project-id PROJECT_ID --all\n" +
			"  n8n agent list --quiet\n" +
			"  n8n agent list --limit 50 --output json",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runAgentList(cmd.Context(), opts, f)
		},
	}
	f.instance.register(cmd)
	cmd.Flags().StringVar(&f.projectID, "project-id", "", "only list Agents of this project (default: every project you can list)")
	cmd.Flags().IntVar(&f.limit, "limit", 0, "Agents per API page, 1 to 250 (default: server default of 100)")
	cmd.Flags().StringVar(&f.cursor, "cursor", "", "pagination cursor returned by a previous list (default: the first page)")
	cmd.Flags().BoolVar(&f.all, "all", false, "follow every page instead of one (maximum 10,000 Agents; hitting it warns and sets JSON collection.truncated)")
	f.view.register(cmd, "agent", "ID and name")
	cmd.Flags().StringVar(&f.output, "output", outputText, "output format: text or json (JSON is a page object with data and nextCursor, plus collection with --all)")
	return cmd
}

func runAgentList(ctx context.Context, opts Options, f agentListFlags) error {
	if err := f.view.validate(f.output); err != nil {
		return err
	}
	list := n8n.AgentListOptions{ListOptions: n8n.ListOptions{Limit: f.limit, Cursor: f.cursor}, ProjectID: f.projectID}
	if err := list.Validate(); err != nil {
		return err
	}
	client, resolution, err := opts.apiClient(f.instance)
	if err != nil {
		return err
	}
	fetch := func(ctx context.Context, page n8n.ListOptions) (n8n.Page[n8n.Agent], error) {
		list.ListOptions = page
		return client.ListAgents(ctx, list)
	}
	var page n8n.Page[n8n.Agent]
	var collection *listCollection
	if f.all {
		page, collection, err = collectPage(ctx, fetch, list.ListOptions)
	} else {
		page, err = fetch(ctx, list.ListOptions)
	}
	if err != nil {
		return agentAPIError(err, resolution, "list", "")
	}
	switch {
	case f.output == outputJSON:
		err = writePageJSON(opts.Streams.Out, page, collection)
	case f.view.enabled():
		err = writeScriptPage(opts, f.view, "agent", page, collection, agentListEmptyHint, func(agent n8n.Agent) scriptListRow {
			return scriptListRow{ID: agent.ID, Fields: []string{agent.Name}}
		})
	default:
		err = writeAgentListText(opts, resolution, page)
	}
	return finishCollection(opts.Streams.Err, collection, err)
}

const agentListEmptyHint = "No Agents were returned. Check --project-id, or create one with 'n8n agent create --name NAME'."

func writeAgentListText(opts Options, resolution config.Resolution, page n8n.Page[n8n.Agent]) error {
	tw := tabwriter.NewWriter(opts.Streams.Out, 0, 0, 2, ' ', 0)
	fmt.Fprintf(tw, "Instance:\t%s\n", resolution.URL)
	fmt.Fprintf(tw, "Agents:\t%d\n", len(page.Data))
	if len(page.Data) > 0 {
		fmt.Fprintln(tw, "\nID\tNAME\tPROJECT\tPUBLISHED\tARCHIVED\tUPDATED")
		for _, agent := range page.Data {
			fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\n", agent.ID, agent.Name, agent.ProjectID, yesNo(agent.Published()), yesNo(agent.IsArchived), agent.UpdatedAt)
		}
	}
	if page.NextCursor != "" {
		fmt.Fprintf(tw, "\nNext cursor:\t%s\n", page.NextCursor)
	}
	if err := tw.Flush(); err != nil {
		return err
	}
	if len(page.Data) == 0 {
		fmt.Fprintln(opts.Streams.Err, agentListEmptyHint)
	}
	return nil
}

type agentReadFlags struct {
	instance instanceFlags
	output   string
}

func (f *agentReadFlags) register(cmd *cobra.Command, json string) {
	f.instance.register(cmd)
	cmd.Flags().StringVar(&f.output, "output", outputText, "output format: text or json ("+json+")")
}

func newAgentGetCommand(opts Options) *cobra.Command {
	var f agentReadFlags
	cmd := &cobra.Command{
		Use:   "get <agent-id>",
		Short: "Get one Agent with its configuration",
		Long: "Get one Agent by ID, as listed by 'n8n agent list'. Text shows the project,\n" +
			"publication, configuration hash, custom tool names, and channel\n" +
			"integrations; JSON is the full Agent, including config and tools. The JSON\n" +
			"can seed 'n8n agent create --input': read-only fields are dropped there.\n" +
			"Read-only; requires agent:read and n8n 2.49 or later.",
		Example: "  n8n agent get AGENT_ID\n" +
			"  n8n agent get AGENT_ID --output json > agent.json",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateOutput(f.output); err != nil {
				return err
			}
			if err := validateAgentArg(args[0]); err != nil {
				return err
			}
			client, resolution, err := opts.apiClient(f.instance)
			if err != nil {
				return err
			}
			agent, err := client.GetAgent(cmd.Context(), args[0])
			if err != nil {
				return agentAPIError(err, resolution, "read", args[0])
			}
			return writeAgentResult(opts, resolution, "Agent", agent, f.output)
		},
	}
	f.register(cmd, "JSON is the full Agent, including config and tools")
	return cmd
}

type agentCreateFlags struct {
	agentReadFlags
	input          string
	name           string
	projectID      string
	availableInMCP bool
	yes            bool
	idempotency    idempotencyFlag
}

func newAgentCreateCommand(opts Options) *cobra.Command {
	var f agentCreateFlags
	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create an Agent",
		Long: "Create an Agent in your personal project or a team project you can access.\n" +
			"--name is enough for a draft; pass config (name, model, instructions) and\n" +
			"custom tools in --input JSON. An empty model creates an unconfigured draft.\n" +
			"Flags override the --input values. Each tools entry maps a tool name to\n" +
			"{\"code\": ..., \"descriptor\": {\"name\": ...}}.\n\n" +
			"--input accepts 'n8n agent get --output json' to copy an Agent: its\n" +
			"read-only fields (id, project, isArchived, versions, timestamps,\n" +
			"configHash, activeVersion) are dropped and reported on stderr; the copy is\n" +
			"an unpublished draft. --input - requires --yes. Requires agent:create and\n" +
			"n8n 2.49 or later.\n\n" + idempotencyHelp,
		Example: "  n8n agent create --name Support --project-id PROJECT_ID\n" +
			"  n8n agent create --input agent.json --name \"Support (copy)\"\n" +
			"  n8n agent get AGENT_ID --output json | n8n agent create --input - --project-id PROJECT_ID --yes",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runAgentCreate(cmd, opts, f)
		},
	}
	f.register(cmd, "JSON is the created Agent")
	cmd.Flags().StringVar(&f.input, "input", "", "create document JSON file path, or - for stdin, maximum 1 MiB (default: none; --name is then required)")
	cmd.Flags().StringVar(&f.name, "name", "", "Agent name; overrides name in --input (default: the --input value)")
	cmd.Flags().StringVar(&f.projectID, "project-id", "", "project to create the Agent in; overrides projectId in --input (default: the --input value, else your personal project)")
	cmd.Flags().BoolVar(&f.availableInMCP, "available-in-mcp", false, "make the Agent available to MCP clients; overrides availableInMCP in --input (default: the --input value, else the server default)")
	cmd.Flags().BoolVar(&f.yes, "yes", false, "confirm reading the document from stdin (required with --input -)")
	f.idempotency.register(cmd)
	return cmd
}

func runAgentCreate(cmd *cobra.Command, opts Options, f agentCreateFlags) error {
	if err := validateOutput(f.output); err != nil {
		return err
	}
	if f.input == "-" && !f.yes {
		return fmt.Errorf("--input - consumes stdin, which cannot also answer a confirmation: pass --yes after reviewing the document")
	}
	if err := f.idempotency.validate(); err != nil {
		return err
	}
	var request n8n.CreateAgentRequest
	if f.input != "" {
		var err error
		if request, err = readAgentCreateDocument(opts, f.input); err != nil {
			return err
		}
	}
	if cmd.Flags().Changed("name") {
		request.Name = f.name
	}
	if cmd.Flags().Changed("project-id") {
		if strings.TrimSpace(f.projectID) == "" {
			return fmt.Errorf("--project-id must not be empty: omit it to create the Agent in your personal project")
		}
		request.ProjectID = f.projectID
	}
	if cmd.Flags().Changed("available-in-mcp") {
		request.AvailableInMCP = &f.availableInMCP
	}
	if err := request.Validate(); err != nil {
		return err
	}
	client, resolution, err := opts.apiClient(f.instance)
	if err != nil {
		return err
	}
	agent, err := client.CreateAgent(cmd.Context(), request, f.idempotency.resolve(opts.Streams.Err)...)
	if err != nil {
		if refused := f.idempotency.refusal(err, resolution); refused != nil {
			return refused
		}
		return agentAPIError(err, resolution, "create", request.Name)
	}
	return writeAgentResult(opts, resolution, "Created", agent, f.output)
}

// readAgentCreateDocument decodes a create document strictly after dropping
// the read-only fields an 'agent get' response carries, each reported on
// stderr. Any other unknown field is an error: the API rejects it.
func readAgentCreateDocument(opts Options, path string) (n8n.CreateAgentRequest, error) {
	var request n8n.CreateAgentRequest
	reader := opts.Streams.In
	if path != "-" {
		file, err := os.Open(path)
		if err != nil {
			return request, fmt.Errorf("open agent input %q: %w", path, err)
		}
		defer file.Close()
		reader = file
	}
	raw, err := io.ReadAll(io.LimitReader(reader, maxAgentDocument+1))
	if err != nil {
		return request, fmt.Errorf("read agent JSON: %w", err)
	}
	if len(raw) > maxAgentDocument {
		return request, fmt.Errorf("agent JSON exceeds %d bytes", maxAgentDocument)
	}
	var fields map[string]json.RawMessage
	decoder := json.NewDecoder(bytes.NewReader(raw))
	if err := decoder.Decode(&fields); err != nil || fields == nil {
		if err == nil {
			err = fmt.Errorf("want a JSON object")
		}
		return request, fmt.Errorf("decode agent JSON: %w", err)
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return request, fmt.Errorf("decode agent JSON: multiple documents are not allowed")
		}
		return request, fmt.Errorf("decode agent JSON: %w", err)
	}
	for _, field := range n8n.AgentReadOnlyFields {
		if _, ok := fields[field]; ok {
			delete(fields, field)
			fmt.Fprintf(opts.Streams.Err, "Dropped read-only field %s.\n", field)
		}
	}
	// A get response sends config null for an Agent without one; null is not
	// a create value, and omitting it creates the same unconfigured draft.
	if value, ok := fields["config"]; ok && bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
		delete(fields, "config")
	}
	cleaned, err := json.Marshal(fields)
	if err != nil {
		return request, fmt.Errorf("decode agent JSON: %w", err)
	}
	strict := json.NewDecoder(bytes.NewReader(cleaned))
	strict.DisallowUnknownFields()
	if err := strict.Decode(&request); err != nil {
		return request, fmt.Errorf("decode agent JSON: %w (writable fields: name, projectId, config, tools, availableInMCP)", err)
	}
	return request, nil
}

type agentDeleteFlags struct {
	agentReadFlags
	yes bool
}

func newAgentDeleteCommand(opts Options) *cobra.Command {
	var f agentDeleteFlags
	cmd := &cobra.Command{
		Use:   "delete <agent-id>",
		Short: "Permanently delete an Agent",
		Long: "Permanently delete one Agent and its associated resources. It does not need\n" +
			"to be archived first: a published Agent is deleted too and its live channels\n" +
			"stop. This cannot be undone. Interactive runs ask for confirmation;\n" +
			"non-interactive runs require --yes. Requires agent:delete and n8n 2.49 or\n" +
			"later.",
		Example: "  n8n agent delete AGENT_ID\n" +
			"  n8n agent delete AGENT_ID --yes --output json",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateOutput(f.output); err != nil {
				return err
			}
			if err := validateAgentArg(args[0]); err != nil {
				return err
			}
			client, resolution, err := opts.apiClient(f.instance)
			if err != nil {
				return err
			}
			question := fmt.Sprintf("Permanently delete Agent %q from %s? A published Agent is deleted too and its channels stop; this cannot be undone.", args[0], resolution.URL)
			if err := opts.confirmer(f.yes).Confirm(question); err != nil {
				return err
			}
			agent, err := client.DeleteAgent(cmd.Context(), args[0])
			if err != nil {
				return agentAPIError(err, resolution, "delete", args[0])
			}
			return writeAgentResult(opts, resolution, "Deleted", agent, f.output)
		},
	}
	f.register(cmd, "JSON is the deleted Agent summary")
	cmd.Flags().BoolVar(&f.yes, "yes", false, "confirm permanent deletion without prompting (required when stdin is not interactive)")
	return cmd
}

func writeAgentResult(opts Options, resolution config.Resolution, heading string, agent *n8n.Agent, output string) error {
	if output == outputJSON {
		return writeJSON(opts.Streams.Out, agent)
	}
	tw := tabwriter.NewWriter(opts.Streams.Out, 0, 0, 2, ' ', 0)
	fmt.Fprintf(tw, "%s:\t%s\n", heading, agent.ID)
	fmt.Fprintf(tw, "Name:\t%s\n", agent.Name)
	project := agent.ProjectID
	if agent.Project != nil && agent.Project.Name != "" {
		project = fmt.Sprintf("%s (%s, %s)", agent.Project.ID, agent.Project.Name, agent.Project.Type)
	}
	fmt.Fprintf(tw, "Project:\t%s\n", emptyDash(project))
	fmt.Fprintf(tw, "Instance:\t%s\n", resolution.URL)
	fmt.Fprintf(tw, "Available in MCP:\t%s\n", yesNo(agent.AvailableInMCP))
	fmt.Fprintf(tw, "Archived:\t%s\n", yesNo(agent.IsArchived))
	fmt.Fprintf(tw, "Version:\t%s\n", emptyDash(derefString(agent.VersionID)))
	fmt.Fprintf(tw, "Published version:\t%s\n", emptyDash(derefString(agent.ActiveVersionID)))
	if agent.ConfigHash != nil {
		fmt.Fprintf(tw, "Config hash:\t%s\n", *agent.ConfigHash)
	}
	if tools := agentToolNames(agent.Tools); len(tools) > 0 {
		fmt.Fprintf(tw, "Tools:\t%s\n", strings.Join(tools, ", "))
	}
	if agent.ActiveVersion != nil {
		for _, integration := range agent.ActiveVersion.Integrations {
			fmt.Fprintf(tw, "Channel:\t%s (credential %s)\n", integration.Type, integration.CredentialID)
		}
	}
	fmt.Fprintf(tw, "Created:\t%s\n", emptyDash(agent.CreatedAt))
	fmt.Fprintf(tw, "Updated:\t%s\n", emptyDash(agent.UpdatedAt))
	return tw.Flush()
}

// agentToolNames lists the custom tool names in a tools document, sorted.
func agentToolNames(raw json.RawMessage) []string {
	var tools map[string]json.RawMessage
	if len(raw) == 0 || json.Unmarshal(raw, &tools) != nil {
		return nil
	}
	names := make([]string, 0, len(tools))
	for name := range tools {
		names = append(names, name)
	}
	slices.Sort(names)
	return names
}

func derefString(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

// agentAPIError names the agent scope a denied action needs and explains the
// 404 an instance before n8n 2.49 answers.
func agentAPIError(err error, resolution config.Resolution, action, subject string) error {
	switch {
	case n8n.IsForbidden(err):
		return forbiddenScopeError(err, resolution, "agent "+action, allOf(agentScope(action)), "", agentResource)
	case n8n.IsNotFound(err) && subject != "" && action != "create" && !promotionRouteMissing(err):
		return fmt.Errorf("%s has no Agent %q (404): check the ID with 'n8n agent list': %w", resolution.URL, subject, err)
	case n8n.IsNotFound(err) && action == "create" && !promotionRouteMissing(err):
		return fmt.Errorf("%s could not find the project for the new Agent (404): check --project-id with 'n8n project list', or omit it to use your personal project: %w", resolution.URL, err)
	case n8n.IsNotFound(err):
		return fmt.Errorf("%s does not serve Agents (404): they need n8n 2.49 or later with the Agents module enabled; run %s to see what this instance offers: %w", resolution.URL, discoverHint(agentResource), err)
	case action == "create" && (n8n.IsStatus(err, http.StatusBadRequest) || n8n.IsStatus(err, http.StatusUnprocessableEntity)):
		return fmt.Errorf("%s rejected the Agent create (%d), so nothing was created: check name, projectId, config (an object with name, model, and instructions), and that each tools entry has code and descriptor.name: %w", resolution.URL, n8n.StatusCodeOf(err), err)
	default:
		return apiError(err, resolution, agentResource)
	}
}

// agentScope is the scope an action needs: agent:list, agent:read,
// agent:create, or agent:delete.
func agentScope(action string) string { return "agent:" + action }

func validateAgentArg(id string) error {
	if strings.TrimSpace(id) == "" {
		return fmt.Errorf("agent ID is required: find it with 'n8n agent list'")
	}
	if strings.TrimSpace(id) != id {
		return fmt.Errorf("agent ID must not start or end with whitespace")
	}
	return nil
}
