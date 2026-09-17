## n8n credential

Manage node credentials without exposing secrets

### Synopsis

Manage credentials stored by an n8n instance.

List and get return metadata only; n8n omits stored credential data. Create and
update accept secret-bearing JSON only from --file or explicit --stdin, never from
flags or arguments. --stdin must be piped so terminal input cannot echo secrets.

Listing is restricted by n8n to instance owners and admins. Updates, deletes,
tests and transfers also depend on credential ownership and granted scopes.

```
n8n credential [flags]
```

### Examples

```
  n8n credential list
  n8n credential get CREDENTIAL_ID
  n8n credential schema githubApi
  n8n credential create --file credential.json
  generate-json | n8n credential update CREDENTIAL_ID --stdin
```

### Options

```
  -h, --help   help for credential
```

### SEE ALSO

* [n8n](n8n.md)	 - Command-line client for the n8n API
* [n8n credential create](n8n_credential_create.md)	 - Create a credential from secret JSON
* [n8n credential delete](n8n_credential_delete.md)	 - Permanently delete a credential
* [n8n credential get](n8n_credential_get.md)	 - Get credential metadata
* [n8n credential list](n8n_credential_list.md)	 - List credential metadata
* [n8n credential schema](n8n_credential_schema.md)	 - Show credential data JSON Schema
* [n8n credential test](n8n_credential_test.md)	 - Test stored credential data
* [n8n credential transfer](n8n_credential_transfer.md)	 - Transfer a credential to another project
* [n8n credential update](n8n_credential_update.md)	 - Update a credential from secret JSON

