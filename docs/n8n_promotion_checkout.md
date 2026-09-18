## n8n promotion checkout

Manage promotion checkouts

### Synopsis

Manage the local checkouts of one promotion connection, one direction
at a time. 'clone' prepares the checkout that 'project', 'promote',
and 'apply' work against; 'disconnect' removes it while keeping the
connection, its configs, and its credentials.

The direction argument is apply or promote.

```
n8n promotion checkout [flags]
```

### Examples

```
  n8n promotion checkout clone CONNECTION_ID apply
  n8n promotion checkout clone CONNECTION_ID promote
  n8n promotion checkout disconnect CONNECTION_ID apply --yes
```

### Options

```
  -h, --help   help for checkout
```

### SEE ALSO

* [n8n promotion](n8n_promotion.md)	 - Manage promotion providers, connections, and apply/promote sync
* [n8n promotion checkout clone](n8n_promotion_checkout_clone.md)	 - Clone one promotion checkout locally
* [n8n promotion checkout disconnect](n8n_promotion_checkout_disconnect.md)	 - Remove one promotion checkout

