package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"text/tabwriter"

	"github.com/SomeoneWithOptions/n8n-cli/internal/config"
	"github.com/SomeoneWithOptions/n8n-cli/internal/n8n"
	"github.com/spf13/cobra"
)

type policyGroup struct {
	name, resource, scope, noun string
	credential                  bool
}

func nodePolicyGroup() policyGroup {
	return policyGroup{"node-policy", "nodetypepolicy", "nodeTypePolicy:manage", "node type", false}
}
func credentialPolicyGroup() policyGroup {
	return policyGroup{"credential-policy", "credentialtypepolicy", "credentialTypePolicy:manage", "credential type", true}
}
func (g policyGroup) api(c *n8n.Client) n8n.PolicyClient {
	if g.credential {
		return c.CredentialTypePolicies()
	}
	return c.NodeTypePolicies()
}
func (g policyGroup) requirements() string {
	return "Preview API: breaking changes may occur. Requires " + g.scope + " and the licensed type-availability-policies module. Use --output json for the full response; text prints a summary."
}
func policyParent(use, short, long, example string) *cobra.Command {
	return &cobra.Command{Use: use, Short: short, Long: long, Example: example, Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error { return cmd.Help() }}
}

func newCredentialPolicyCommand(opts Options) *cobra.Command {
	g := credentialPolicyGroup()
	cmd := policyParent(g.name, "Manage credential type availability policies",
		"Manage instance and project credential type policies. Start with 'instance get', then inspect 'attachment get' before editing scopes with shared documents. GET returns composed rules; scope set replaces only the scope's own document and default. Documents are reusable rule lists. Floor attachments run first, then priority ascending within each group. The first matching rule wins. "+g.requirements(),
		"  n8n credential-policy instance get\n  n8n credential-policy document list")
	for _, scope := range []string{"instance", "project"} {
		parent := policyParent(scope, "Manage "+scope+" credential type policies", "Read or replace the "+scope+" policy. Start with 'get' to obtain scopeId and version; scopeId is null until the first write. Instance scope accepts delegate; project scope accepts allow or deny. "+g.requirements(), "  n8n credential-policy "+scope+" get"+policyExampleID(scope))
		parent.AddCommand(newCredentialScopeCommand(opts, g, scope, false), newCredentialScopeCommand(opts, g, scope, true))
		cmd.AddCommand(parent)
	}
	addPolicyDocuments(cmd, opts, g)
	return cmd
}
func policyExampleID(scope string) string {
	if scope == "project" {
		return " PROJECT_ID"
	}
	return ""
}
func newCredentialScopeCommand(opts Options, g policyGroup, scope string, set bool) *cobra.Command {
	verb := "get"
	if set {
		verb = "set"
	}
	use := verb
	if scope == "project" {
		use += " <project-id>"
	}
	var f policyFlags
	long := "Read the composed " + scope + " credential type policy, including every attached document in evaluation order. This does not combine project and instance evaluation. scopeId identifies attachments and is null until the scope is first written. Find project IDs with 'n8n project list'."
	if set {
		long = "Fully replace the " + scope + " defaultAction and rules of its own document using rules, defaultAction, and version last read (0 before first write). Omitted rules are deleted. GET includes shared rules: remove those before using its response as input, or they will be copied into the own document. Selectors accept name, package, or extends; extends matches a named credential type and every type extending it. Project scope rejects delegate. Stale versions or shared project documents return 409. Blocking credential types can stop workflows using them. Interactive runs confirm; non-interactive runs and --input - require --yes. scopeId and warnings from GET are dropped and reported on stderr."
	}
	cmd := &cobra.Command{Use: use, Short: map[string]string{"get": "Get", "set": "Replace", "list": "List", "create": "Create", "delete": "Delete"}[verb] + " the " + scope + " credential type policy", Long: long + " " + g.requirements(), Example: "  n8n " + g.name + " " + scope + " " + verb + policyExampleID(scope), Args: cobra.NoArgs}
	if scope == "project" {
		cmd.Args = cobra.ExactArgs(1)
	}
	if set {
		cmd.Example += " --input policy.json\n  n8n " + g.name + " " + scope + " set" + policyExampleID(scope) + " --input - --yes < policy.json"
	}
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		if err := validateOutput(f.output); err != nil {
			return err
		}
		id := ""
		if len(args) > 0 {
			id = args[0]
			if strings.TrimSpace(id) == "" {
				return fmt.Errorf("project ID is required")
			}
		}
		var request n8n.ReplaceNodeTypePolicyRequest
		if set {
			if err := policyStdinConsent(f); err != nil {
				return err
			}
			var d nodePolicyDocument
			raw, err := readPolicyInput(opts, f.input, &d)
			if err != nil {
				return err
			}
			if d.Rules == nil {
				return fmt.Errorf("rules is required: use [] for an empty policy")
			}
			request = n8n.ReplaceNodeTypePolicyRequest{Rules: d.Rules, DefaultAction: d.DefaultAction, Version: d.Version}
			// Validation needs only the policy kind, no configured HTTP client.
			if err := g.api(nil).ValidateScope(request, n8n.NodeTypePolicyScope(scope)); err != nil {
				return err
			}
			reportPolicyDropped(opts, raw, "scopeId", "warnings")
		}
		client, res, err := opts.apiClient(f.instance)
		if err != nil {
			return err
		}
		api := g.api(client)
		if !set {
			var result *n8n.NodeTypePolicy
			if scope == "project" {
				result, err = api.GetProject(cmd.Context(), id)
			} else {
				result, err = api.GetInstance(cmd.Context())
			}
			if err != nil {
				return policyAPIError(err, res, g, scope, verb)
			}
			return writePolicyScope(opts, res, result, nil, f.output)
		}
		if err := opts.confirmer(f.yes).Confirm(fmt.Sprintf("Replace %s credential type policy %q on %s? Omitted rules are deleted; blocked credential types can stop workflows using them.", scope, id, res.URL)); err != nil {
			return err
		}
		var result *n8n.NodeTypePolicyWriteResult
		if scope == "project" {
			result, err = api.ReplaceProject(cmd.Context(), id, request)
		} else {
			result, err = api.ReplaceInstance(cmd.Context(), request)
		}
		if err != nil {
			return policyAPIError(err, res, g, scope, verb)
		}
		if f.output == outputJSON {
			return writeJSON(opts.Streams.Out, result)
		}
		return writePolicyScope(opts, res, &result.NodeTypePolicy, result.Warnings, f.output)
	}
	f.register(cmd, set, set)
	return cmd
}

type policyFlags struct {
	instance              instanceFlags
	input, output, cursor string
	yes, all              bool
	limit                 int
}

func (f *policyFlags) register(cmd *cobra.Command, input, confirm bool) {
	f.instance.register(cmd)
	cmd.Flags().StringVar(&f.output, "output", outputText, "output format: text or json (default: text; JSON contains the full response)")
	if input {
		cmd.Flags().StringVar(&f.input, "input", "", "JSON file path, or - for stdin (required; maximum 1 MiB; stdin requires --yes)")
	}
	// Create does not prompt, but stdin still requires explicit consent.
	if input || confirm {
		cmd.Flags().BoolVar(&f.yes, "yes", false, "confirm mutation without prompting (default: false; required for non-interactive replacement/deletion and --input -)")
	}
}
func addPolicyDocuments(cmd *cobra.Command, opts Options, g policyGroup) {
	doc := policyParent("document", "Manage reusable "+g.noun+" policy documents", "Manage reusable rule lists. Start with 'list', then 'get' before 'set'. Create produces an unattached document; attach it with 'attachment set'. Updating a document changes every attached scope and bumps their versions. A document containing delegate cannot be attached to a project scope. "+g.requirements(), "  n8n "+g.name+" document list\n  n8n "+g.name+" document get POLICY_ID --output json")
	for _, verb := range []string{"list", "get", "create", "set", "delete"} {
		doc.AddCommand(newPolicyResourceCommand(opts, g, "document", verb))
	}
	att := policyParent("attachment", "Manage attached "+g.noun+" policy documents", "Inspect or replace a scope's complete attachment list. Start with 'get'. Obtain SCOPE_ID from 'instance get' or 'project get PROJECT_ID' -> scopeId; it is null until that scope is first written with set. Requires instance-level access even for project scopes. Floor attachments evaluate first, then priority ascending within each group. "+g.requirements(), "  n8n "+g.name+" instance get --output json\n  n8n "+g.name+" attachment get SCOPE_ID")
	for _, verb := range []string{"get", "set"} {
		att.AddCommand(newPolicyResourceCommand(opts, g, "attachment", verb))
	}
	cmd.AddCommand(doc, att)
}
func newPolicyResourceCommand(opts Options, g policyGroup, resource, verb string) *cobra.Command {
	var f policyFlags
	use := verb
	args := cobra.NoArgs
	idName := "POLICY_ID"
	if resource == "attachment" {
		idName = "SCOPE_ID"
	}
	if verb != "list" && verb != "create" {
		use += " <" + strings.ReplaceAll(strings.ToLower(idName), "_", "-") + ">"
		args = cobra.ExactArgs(1)
	}
	descriptions := map[string]string{
		"document list":   "List reusable documents in cursor pages (default 100, maximum 250). Use --cursor for another page, or --all to collect up to 10,000 documents. Start here to find POLICY_ID for get, set, and delete. " + cursorCollectionHelp,
		"document get":    "Read a reusable document's ID, kind, ordered rules, version, updater and timestamps. Find POLICY_ID with 'document list'. Edit rules and use the last-read version with 'document set'; read-only fields are dropped with a stderr diagnostic.",
		"document create": "Create an unattached reusable document from JSON {\"rules\":[]}. Rules are required; each rule has a unique id, action allow|deny|delegate, and selector {kind,value}. Creation returns {policy,warnings}; warnings do not fail the write. Attach the returned policy.id using 'attachment set'. --input - requires --yes.",
		"document set":    "Fully replace a document's rules using JSON {\"rules\":[],\"version\":1}. Read the document first. This changes every attached scope and bumps each scope's version; omitted rules are deleted. The server rejects delegate when any attached scope is a project. Stale versions return 409: read again before retrying. Returns {policy,warnings}. Interactive runs confirm; non-interactive runs and --input - require --yes.",
		"document delete": "Permanently delete an unattached document found with 'document list'. Attached documents return 409: inspect 'attachment get' and use 'attachment set' to detach first. Interactive runs confirm; non-interactive runs require --yes. Success returns the deleted ID; deletion cannot be undone.",
		"attachment get":  "Read a scope's attachments and version in evaluation order: floor first, then priority ascending within each group. Get SCOPE_ID from 'instance get' or 'project get PROJECT_ID' -> scopeId; it is null until the scope is first written. Requires instance-level access even for project scopes. Use the returned version with 'attachment set'.",
		"attachment set":  "Fully replace all attachments using JSON {\"attachments\":[{\"policyId\":\"POLICY_ID\",\"priority\":0,\"isFloor\":false}],\"version\":1}. Each policyId and (isFloor,priority) pair must be unique. Use [] to detach every document. Get SCOPE_ID from 'instance get' or 'project get PROJECT_ID' -> scopeId; null means write that scope first. Requires instance-level access even for projects. Delegate documents cannot attach to projects. Use the last-read scope version: stale writes return 409. Replacing attachments changes policy evaluation and bumps the scope version. Interactive runs confirm; non-interactive runs and --input - require --yes. Returns attachments and the new version; read-only rules in GET input are dropped with a stderr diagnostic.",
	}
	long := descriptions[resource+" "+verb]
	if resource == "document" && (verb == "create" || verb == "set") {
		long += " Selectors accept name or package."
		if g.credential {
			long += " Credential policies also accept extends, matching the named type and every type extending it."
		}
	}
	example := "  n8n " + g.name + " " + resource + " " + verb
	if verb != "list" && verb != "create" {
		example += " " + idName
	}
	input := verb == "create" || verb == "set"
	confirm := verb == "set" || verb == "delete"
	if input {
		example += " --input policy.json\n  n8n " + g.name + " " + resource + " " + verb
		if verb == "set" {
			example += " " + idName
		}
		example += " --input - --yes < policy.json"
	} else if confirm {
		example += "\n  n8n " + g.name + " " + resource + " " + verb + " " + idName + " --yes"
	}
	cmd := &cobra.Command{Use: use, Short: map[string]string{"get": "Get", "set": "Replace", "list": "List", "create": "Create", "delete": "Delete"}[verb] + " " + g.noun + " policy " + resource + "s", Long: long + " " + g.requirements(), Example: example, Args: args}
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		return runPolicyResource(cmd, opts, g, resource, verb, args, f)
	}
	f.register(cmd, input, confirm)
	if verb == "list" {
		cmd.Flags().IntVar(&f.limit, "limit", 100, "documents per page, 1 to 250 (default: 100)")
		cmd.Flags().StringVar(&f.cursor, "cursor", "", "cursor returned by a previous list (default: first page)")
		cmd.Flags().BoolVar(&f.all, "all", false, "follow cursor pages up to 10,000 documents (default: false)")
	}
	return cmd
}

func policyStdinConsent(f policyFlags) error {
	if f.input == "-" && !f.yes {
		return fmt.Errorf("--input - consumes stdin: pass --yes after reviewing the document")
	}
	return nil
}
func readPolicyInput(opts Options, path string, dest any) ([]byte, error) {
	if strings.TrimSpace(path) == "" {
		return nil, fmt.Errorf("--input is required: pass a JSON file path or - for stdin")
	}
	reader := opts.Streams.In
	if path != "-" {
		file, err := os.Open(path)
		if err != nil {
			return nil, err
		}
		defer file.Close()
		reader = file
	}
	raw, err := io.ReadAll(io.LimitReader(reader, maxNodePolicyDocument+1))
	if err != nil {
		return nil, err
	}
	if len(raw) > maxNodePolicyDocument {
		return nil, fmt.Errorf("policy JSON exceeds %d bytes", maxNodePolicyDocument)
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(dest); err != nil {
		return nil, fmt.Errorf("decode policy JSON: %w", err)
	}
	var extra any
	if err := dec.Decode(&extra); err != io.EOF {
		return nil, fmt.Errorf("decode policy JSON: expected one document")
	}
	return raw, nil
}
func reportPolicyDropped(opts Options, raw []byte, fields ...string) {
	var m map[string]json.RawMessage
	if json.Unmarshal(raw, &m) != nil {
		return
	}
	for _, field := range fields {
		if _, ok := m[field]; ok {
			fmt.Fprintf(opts.Streams.Err, "Dropped read-only field %s.\n", field)
		}
	}
}

type reusablePolicyInput struct {
	Rules     []n8n.NodeTypePolicyRule `json:"rules"`
	Version   *int                     `json:"version"`
	ID        json.RawMessage          `json:"id"`
	Kind      json.RawMessage          `json:"kind"`
	UpdatedBy json.RawMessage          `json:"updatedBy"`
	CreatedAt json.RawMessage          `json:"createdAt"`
	UpdatedAt json.RawMessage          `json:"updatedAt"`
}
type attachmentPolicyInput struct {
	Attachments []struct {
		PolicyID string          `json:"policyId"`
		Priority *int            `json:"priority"`
		IsFloor  *bool           `json:"isFloor"`
		Rules    json.RawMessage `json:"rules"`
	} `json:"attachments"`
	Version *int `json:"version"`
}

func runPolicyResource(cmd *cobra.Command, opts Options, g policyGroup, resource, verb string, args []string, f policyFlags) error {
	if err := validateOutput(f.output); err != nil {
		return err
	}
	id := ""
	if len(args) > 0 {
		id = args[0]
		if strings.TrimSpace(id) == "" {
			return fmt.Errorf("%s ID is required", resource)
		}
	}
	if verb == "list" && (f.limit < 1 || f.limit > 250) {
		return fmt.Errorf("--limit must be between 1 and 250")
	}
	var doc reusablePolicyInput
	var attachments n8n.ReplacePolicyAttachmentsRequest
	if verb == "create" || verb == "set" {
		if err := policyStdinConsent(f); err != nil {
			return err
		}
		if resource == "document" {
			if verb == "create" {
				var d n8n.CreatePolicyDocumentRequest
				if _, err := readPolicyInput(opts, f.input, &d); err != nil {
					return err
				}
				doc.Rules = d.Rules
			} else {
				raw, err := readPolicyInput(opts, f.input, &doc)
				if err != nil {
					return err
				}
				reportPolicyDropped(opts, raw, "id", "kind", "updatedBy", "createdAt", "updatedAt")
			}
			if doc.Rules == nil {
				return fmt.Errorf("rules is required: use [] for an empty document")
			}
			if err := g.api(nil).ValidateRules(doc.Rules, n8n.NodeTypePolicyScopeInstance); err != nil {
				return err
			}
			if verb == "set" && (doc.Version == nil || *doc.Version < 0) {
				return fmt.Errorf("version is required and must be 0 or greater: send the version last read")
			}
		} else {
			var d attachmentPolicyInput
			if _, err := readPolicyInput(opts, f.input, &d); err != nil {
				return err
			}
			if d.Attachments == nil {
				return fmt.Errorf("attachments is required: use [] to detach every document")
			}
			attachments = n8n.ReplacePolicyAttachmentsRequest{Version: d.Version, Attachments: []n8n.PolicyAttachmentInput{}}
			for i, a := range d.Attachments {
				attachments.Attachments = append(attachments.Attachments, n8n.PolicyAttachmentInput{PolicyID: a.PolicyID, Priority: a.Priority, IsFloor: a.IsFloor})
				if len(a.Rules) > 0 {
					fmt.Fprintf(opts.Streams.Err, "Dropped read-only field attachments[%d].rules.\n", i)
				}
			}
			if err := attachments.Validate(); err != nil {
				return err
			}
		}
	}
	client, res, err := opts.apiClient(f.instance)
	if err != nil {
		return err
	}
	api := g.api(client)
	if verb == "set" || verb == "delete" {
		question := fmt.Sprintf("Permanently delete %s policy document %q on %s? Deletion cannot be undone.", g.noun, id, res.URL)
		if verb == "set" && resource == "document" {
			question = fmt.Sprintf("Replace every rule in %s policy document %q on %s? This changes every attached scope, bumps their versions, and may stop workflows using blocked types.", g.noun, id, res.URL)
		}
		if resource == "attachment" {
			question = fmt.Sprintf("Replace all %s policy attachments of scope %q on %s? Omitted attachments are removed; changed policy evaluation may stop workflows.", g.noun, id, res.URL)
		}
		if err := opts.confirmer(f.yes).Confirm(question); err != nil {
			return err
		}
	}
	var result any
	if resource == "attachment" {
		if verb == "get" {
			result, err = api.GetAttachments(cmd.Context(), id)
		} else {
			result, err = api.ReplaceAttachments(cmd.Context(), id, attachments)
		}
	} else {
		switch verb {
		case "list":
			var page n8n.Page[n8n.PolicyDocument]
			var collection *listCollection
			o := n8n.ListOptions{Limit: f.limit, Cursor: f.cursor}
			if f.all {
				page, collection, err = collectPage(cmd.Context(), api.ListDocuments, o)
			} else {
				page, err = api.ListDocuments(cmd.Context(), o)
			}
			if err != nil {
				return policyAPIError(err, res, g, resource, verb)
			}
			if len(page.Data) == 0 {
				fmt.Fprintf(opts.Streams.Err, "No policy documents returned. Create one with 'n8n %s document create --input policy.json'.\n", g.name)
			}
			if f.output == outputJSON {
				err = writePageJSON(opts.Streams.Out, page, collection)
			} else {
				tw := tabwriter.NewWriter(opts.Streams.Out, 0, 0, 2, ' ', 0)
				fmt.Fprintln(tw, "ID\tKIND\tVERSION\tRULES\tUPDATED")
				for _, d := range page.Data {
					fmt.Fprintf(tw, "%s\t%s\t%d\t%d\t%s\n", d.ID, d.Kind, d.Version, len(d.Rules), d.UpdatedAt)
				}
				if page.NextCursor != "" {
					fmt.Fprintf(tw, "Next cursor:\t%s\n", page.NextCursor)
				}
				err = tw.Flush()
			}
			return finishCollection(opts.Streams.Err, collection, err)
		case "get":
			result, err = api.GetDocument(cmd.Context(), id)
		case "create":
			result, err = api.CreateDocument(cmd.Context(), n8n.CreatePolicyDocumentRequest{Rules: doc.Rules})
		case "set":
			result, err = api.UpdateDocument(cmd.Context(), id, n8n.UpdatePolicyDocumentRequest{Rules: doc.Rules, Version: doc.Version})
		case "delete":
			err = api.DeleteDocument(cmd.Context(), id)
			result = struct {
				ID string `json:"id"`
			}{id}
		}
	}
	if err != nil {
		return policyAPIError(err, res, g, resource, verb)
	}
	if f.output == outputJSON {
		return writeJSON(opts.Streams.Out, result)
	}
	return writePolicyResourceText(opts, result)
}
func writePolicyResourceText(opts Options, result any) error {
	tw := tabwriter.NewWriter(opts.Streams.Out, 0, 0, 2, ' ', 0)
	document := func(d n8n.PolicyDocument) {
		fmt.Fprintf(tw, "ID:\t%s\nKind:\t%s\nVersion:\t%d\nUpdated by:\t%s\nCreated:\t%s\nUpdated:\t%s\n", d.ID, d.Kind, d.Version, d.UpdatedBy, d.CreatedAt, d.UpdatedAt)
		writePolicyRules(tw, d.Rules)
	}
	switch r := result.(type) {
	case *n8n.PolicyDocument:
		document(*r)
	case *n8n.PolicyDocumentWriteResult:
		document(r.Policy)
		writePolicyWarnings(opts, r.Warnings)
	case *n8n.PolicyAttachments:
		fmt.Fprintf(tw, "Version:\t%d\nPOLICY ID\tPRIORITY\tFLOOR\tRULES\n", r.Version)
		for _, a := range r.Attachments {
			fmt.Fprintf(tw, "%s\t%d\t%t\t%d\n", a.PolicyID, a.Priority, a.IsFloor, len(a.Rules))
		}
		if len(r.Attachments) == 0 {
			fmt.Fprintln(opts.Streams.Err, "No policy documents attached. Use attachment set to attach documents.")
		}
	default:
		fmt.Fprintln(opts.Streams.Err, "Policy document deleted.")
		return nil
	}
	return tw.Flush()
}
func writePolicyRules(w io.Writer, rules []n8n.NodeTypePolicyRule) {
	fmt.Fprintln(w, "ORDER\tID\tACTION\tSELECTOR")
	for i, r := range rules {
		fmt.Fprintf(w, "%d\t%s\t%s\t%s=%s\n", i+1, r.ID, r.Action, r.Selector.Kind, r.Selector.Value)
	}
}
func writePolicyWarnings(opts Options, warnings []n8n.NodeTypePolicyWarning) {
	for _, w := range warnings {
		fmt.Fprintf(opts.Streams.Err, "Warning: rule %s: %s", w.RuleID, w.Kind)
		if w.ShadowedByRuleID != "" {
			fmt.Fprintf(opts.Streams.Err, " (shadowed by %s)", w.ShadowedByRuleID)
		}
		fmt.Fprintln(opts.Streams.Err)
	}
}
func writePolicyScope(opts Options, res config.Resolution, p *n8n.NodeTypePolicy, warnings []n8n.NodeTypePolicyWarning, output string) error {
	if output == outputJSON {
		return writeJSON(opts.Streams.Out, p)
	}
	tw := tabwriter.NewWriter(opts.Streams.Out, 0, 0, 2, ' ', 0)
	fmt.Fprintf(tw, "Instance:\t%s\nDefault action:\t%s\nVersion:\t%d\n", res.URL, p.DefaultAction, p.Version)
	if p.ScopeID == nil {
		fmt.Fprintln(tw, "Scope ID:\tnone (never configured)")
	} else {
		fmt.Fprintf(tw, "Scope ID:\t%s\n", *p.ScopeID)
	}
	writePolicyRules(tw, p.Rules)
	writePolicyWarnings(opts, warnings)
	return tw.Flush()
}
func policyAPIError(err error, res config.Resolution, g policyGroup, resource, verb string) error {
	switch {
	case n8n.IsForbidden(err):
		detail := ""
		if resource == "attachment" {
			detail = "attachment endpoints require instance-level access even for project scopes"
		}
		return forbiddenScopeError(err, res, g.noun+" policy "+verb, allOf(g.scope), detail, g.resource)
	case n8n.IsNotFound(err), n8n.IsStatus(err, http.StatusServiceUnavailable):
		return fmt.Errorf("%s: %s policies are not available to you (%d); verify the policy/scope ID and licensed type-availability-policies module; run %s", res.URL, g.noun, n8n.StatusCodeOf(err), discoverHint(g.resource))
	case n8n.IsConflict(err):
		if verb == "delete" {
			return fmt.Errorf("policy document is still attached (409): run 'n8n %s attachment get SCOPE_ID', then 'n8n %s attachment set SCOPE_ID' to detach it before retrying", g.name, g.name)
		}
		return fmt.Errorf("%s policy %s rejected (409): re-read 'n8n %s %s get' and send the current version; attachment writes and attached-document updates bump scope versions. Project scopes reject delegate, and shared project documents must be edited with document set", g.noun, verb, g.name, resource)
	case n8n.IsStatus(err, http.StatusBadRequest):
		return fmt.Errorf("%s policy input rejected (400): send required rules/attachments and version for replacements, use unique rule IDs, policyIds and (isFloor, priority) pairs; project scopes reject delegate; inspect 'n8n %s %s %s --help'", g.noun, g.name, resource, verb)
	case n8n.IsStatus(err, http.StatusUnsupportedMediaType):
		return fmt.Errorf("policy input rejected (415): send a JSON document with --input FILE or --input -")
	default:
		return apiError(err, res, g.resource)
	}
}
