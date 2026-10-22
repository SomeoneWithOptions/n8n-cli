## n8n credential-policy document

Manage reusable credential type policy documents

### Synopsis

Manage reusable rule lists. Start with 'list', then 'get' before 'set'. Create produces an unattached document; attach it with 'attachment set'. Updating a document changes every attached scope and bumps their versions. A document containing delegate cannot be attached to a project scope. Preview API: breaking changes may occur. Requires credentialTypePolicy:manage and the licensed type-availability-policies module. Use --output json for the full response; text prints a summary.

```
n8n credential-policy document [flags]
```

### Examples

```
  n8n credential-policy document list
  n8n credential-policy document get POLICY_ID --output json
```

### Options

```
  -h, --help   help for document
```

### SEE ALSO

* [n8n credential-policy](n8n_credential-policy.md)	 - Manage credential type availability policies
* [n8n credential-policy document create](n8n_credential-policy_document_create.md)	 - Create credential type policy documents
* [n8n credential-policy document delete](n8n_credential-policy_document_delete.md)	 - Delete credential type policy documents
* [n8n credential-policy document get](n8n_credential-policy_document_get.md)	 - Get credential type policy documents
* [n8n credential-policy document list](n8n_credential-policy_document_list.md)	 - List credential type policy documents
* [n8n credential-policy document set](n8n_credential-policy_document_set.md)	 - Replace credential type policy documents

