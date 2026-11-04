## n8n promotion environment

Manage promotion environments (n8n 2.46 and later)

### Synopsis

Manage promotion environments: one remote, one direction (apply or
promote), its settings, and its local checkout. n8n 2.46 replaced
connections with environments; on older instances use 'n8n promotion
connection' instead.

Start with 'list' to find the environment ID, then 'get' to read it,
'create' to add one on a provider, or 'update' to change its name,
provider, remote, or settings. 'connect' prepares the local checkout
and 'disconnect' removes it. Link team projects to a projects-scope
environment under 'project'. The instance can have one environment for
each direction.

Sync the whole instance through the connected instance-scope
environment: 'promote' pushes every team project to a promote
environment; 'apply' imports an apply environment's branch, and
'apply-continue' finishes an apply that preflight blocked. For selected
workflows of one project use 'n8n promotion project changes', 'apply',
and 'promote'.

Deleting an environment removes its project links and local checkout;
its provider is kept and nothing in Git changes.

```
n8n promotion environment [flags]
```

### Examples

```
  n8n promotion environment list
  n8n promotion environment get ENVIRONMENT_ID
  n8n promotion environment create --input environment.json
  n8n promotion environment connect ENVIRONMENT_ID
  n8n promotion environment apply ENVIRONMENT_ID --yes
  n8n promotion environment promote ENVIRONMENT_ID --commit-message "sync" --yes
  n8n promotion environment project add ENVIRONMENT_ID PROJECT_ID
```

### Options

```
  -h, --help   help for environment
```

### SEE ALSO

* [n8n promotion](n8n_promotion.md)	 - Manage promotion providers, environments, connections, and apply/promote sync
* [n8n promotion environment apply](n8n_promotion_environment_apply.md)	 - Apply an environment's Git branch into this instance
* [n8n promotion environment apply-continue](n8n_promotion_environment_apply-continue.md)	 - Continue a reviewed environment apply after binding setup
* [n8n promotion environment connect](n8n_promotion_environment_connect.md)	 - Prepare the local checkout of a promotion environment
* [n8n promotion environment create](n8n_promotion_environment_create.md)	 - Create a promotion environment
* [n8n promotion environment delete](n8n_promotion_environment_delete.md)	 - Permanently delete a promotion environment
* [n8n promotion environment disconnect](n8n_promotion_environment_disconnect.md)	 - Remove the local checkout of a promotion environment
* [n8n promotion environment get](n8n_promotion_environment_get.md)	 - Show one promotion environment
* [n8n promotion environment list](n8n_promotion_environment_list.md)	 - List promotion environments
* [n8n promotion environment project](n8n_promotion_environment_project.md)	 - Manage projects linked to a promotion environment
* [n8n promotion environment promote](n8n_promotion_environment_promote.md)	 - Promote team projects to an environment's Git remote
* [n8n promotion environment update](n8n_promotion_environment_update.md)	 - Update a promotion environment

