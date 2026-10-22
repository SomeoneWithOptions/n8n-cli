## n8n node-policy document

Manage reusable node type policy documents

### Synopsis

Manage reusable rule lists. Start with 'list', then 'get' before 'set'. Create produces an unattached document; attach it with 'attachment set'. Updating a document changes every attached scope and bumps their versions. A document containing delegate cannot be attached to a project scope. Preview API: breaking changes may occur. Requires nodeTypePolicy:manage and the licensed type-availability-policies module. Use --output json for the full response; text prints a summary.

```
n8n node-policy document [flags]
```

### Examples

```
  n8n node-policy document list
  n8n node-policy document get POLICY_ID --output json
```

### Options

```
  -h, --help   help for document
```

### SEE ALSO

* [n8n node-policy](n8n_node-policy.md)	 - Manage node type availability policies
* [n8n node-policy document create](n8n_node-policy_document_create.md)	 - Create node type policy documents
* [n8n node-policy document delete](n8n_node-policy_document_delete.md)	 - Delete node type policy documents
* [n8n node-policy document get](n8n_node-policy_document_get.md)	 - Get node type policy documents
* [n8n node-policy document list](n8n_node-policy_document_list.md)	 - List node type policy documents
* [n8n node-policy document set](n8n_node-policy_document_set.md)	 - Replace node type policy documents

