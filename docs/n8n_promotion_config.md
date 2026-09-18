## n8n promotion config

Manage promotion direction configs

### Synopsis

Manage the two direction configs of one promotion connection. Each
direction has its own checkout: 'apply' pulls the remote into the
instance, 'promote' pushes the instance to the remote.

Both 'set' actions are full replacements that create the config on
first write. Removing a config can remove its local checkout.

```
n8n promotion config [flags]
```

### Examples

```
  n8n promotion config apply set CONNECTION_ID --input apply.json
  n8n promotion config promote set CONNECTION_ID --input promote.json
  n8n promotion config delete CONNECTION_ID apply --yes
```

### Options

```
  -h, --help   help for config
```

### SEE ALSO

* [n8n promotion](n8n_promotion.md)	 - Manage promotion providers, connections, and apply/promote sync
* [n8n promotion config apply](n8n_promotion_config_apply.md)	 - Manage the apply direction config
* [n8n promotion config delete](n8n_promotion_config_delete.md)	 - Delete one direction config
* [n8n promotion config promote](n8n_promotion_config_promote.md)	 - Manage the promote direction config

