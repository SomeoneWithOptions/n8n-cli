## n8n promotion provider

Manage promotion providers

### Synopsis

Manage promotion providers: the stored Git credentials one or more
connections sync through. Start with 'list' to find the provider ID,
then 'get' to read it, 'create' to add one, or 'update' to rename it
or replace its credential.

Replacing a provider credential affects every connection attached to
it, so 'update' asks for confirmation. Deleting a provider removes it
while attached connections keep their content but lose their credential.

```
n8n promotion provider [flags]
```

### Examples

```
  n8n promotion provider list
  n8n promotion provider get PROVIDER_ID
  n8n promotion provider create --input provider.json
  n8n promotion provider update PROVIDER_ID --input provider-update.json --yes
```

### Options

```
  -h, --help   help for provider
```

### SEE ALSO

* [n8n promotion](n8n_promotion.md)	 - Manage promotion providers, connections, and apply/promote sync
* [n8n promotion provider create](n8n_promotion_provider_create.md)	 - Create a promotion provider
* [n8n promotion provider delete](n8n_promotion_provider_delete.md)	 - Permanently delete a promotion provider
* [n8n promotion provider get](n8n_promotion_provider_get.md)	 - Show one promotion provider
* [n8n promotion provider list](n8n_promotion_provider_list.md)	 - List promotion providers
* [n8n promotion provider update](n8n_promotion_provider_update.md)	 - Update a promotion provider

