## n8n promotion connection

Manage promotion connections

### Synopsis

Manage promotion connections: one remote plus its two direction
configs. Start with 'list' to find the connection ID, then 'get' to
read it, 'create' to add one on a provider, or 'update' to change its
name, target, or provider.

Deleting a connection can remove its local checkouts. Linked projects
stay on the instance but lose their remote.

```
n8n promotion connection [flags]
```

### Examples

```
  n8n promotion connection list
  n8n promotion connection get CONNECTION_ID
  n8n promotion connection create --input connection.json
  n8n promotion connection update CONNECTION_ID --input connection-update.json
```

### Options

```
  -h, --help   help for connection
```

### SEE ALSO

* [n8n promotion](n8n_promotion.md)	 - Manage promotion providers, connections, and apply/promote sync
* [n8n promotion connection create](n8n_promotion_connection_create.md)	 - Create a promotion connection
* [n8n promotion connection delete](n8n_promotion_connection_delete.md)	 - Permanently delete a promotion connection
* [n8n promotion connection get](n8n_promotion_connection_get.md)	 - Show one promotion connection
* [n8n promotion connection list](n8n_promotion_connection_list.md)	 - List promotion connections
* [n8n promotion connection update](n8n_promotion_connection_update.md)	 - Update a promotion connection

