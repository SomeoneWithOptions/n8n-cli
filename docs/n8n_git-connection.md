## n8n git-connection

Manage the Git connection, its projects, and push/pull sync

### Synopsis

Manage the Git connection this instance syncs team projects through.
Start with 'list' to find the connection ID (an instance holds at most
one), then 'get' to read it, 'create' to add it, or 'update' to change
it. Clone the repository with 'clone', link team projects under
'project', then 'push' local projects to the remote or 'pull' the
remote into the instance.

'push' commits and pushes every team project to the remote branch;
'pull' resets the local clone to the branch tip and imports projects
into the instance, overwriting to match. Both mutate versioned
content and ask for confirmation. 'disconnect' removes the local
checkout while keeping the connection; 'delete' removes the
connection and its checkout. This is the target-only GitConnections
generation: an instance that serves Promotions instead answers 404
or 503, which reads as the availability diagnostic.

```
n8n git-connection [flags]
```

### Examples

```
  n8n git-connection list
  n8n git-connection get CONNECTION_ID
  n8n git-connection create --input connection.json
  n8n git-connection clone CONNECTION_ID --branch main
  n8n git-connection project add CONNECTION_ID PROJECT_ID
  n8n git-connection push CONNECTION_ID --commit-message "sync" --yes
  n8n git-connection pull CONNECTION_ID --yes
```

### Options

```
  -h, --help   help for git-connection
```

### SEE ALSO

* [n8n](n8n.md)	 - Command-line client for the n8n API
* [n8n git-connection clone](n8n_git-connection_clone.md)	 - Clone the Git repository locally
* [n8n git-connection create](n8n_git-connection_create.md)	 - Create the Git connection
* [n8n git-connection delete](n8n_git-connection_delete.md)	 - Permanently delete a Git connection
* [n8n git-connection disconnect](n8n_git-connection_disconnect.md)	 - Remove the local Git checkout
* [n8n git-connection get](n8n_git-connection_get.md)	 - Show one Git connection
* [n8n git-connection list](n8n_git-connection_list.md)	 - List Git connections
* [n8n git-connection project](n8n_git-connection_project.md)	 - Manage projects linked to a Git connection
* [n8n git-connection pull](n8n_git-connection_pull.md)	 - Pull the Git remote into this instance
* [n8n git-connection push](n8n_git-connection_push.md)	 - Push all team projects to the Git remote
* [n8n git-connection update](n8n_git-connection_update.md)	 - Update a Git connection

