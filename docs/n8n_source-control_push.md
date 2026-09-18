## n8n source-control push

Commit and push local changes to the Git remote

### Synopsis

Commit and push the selected local files to the connected Git repository.
Each entry in the selection is resolved against a fresh preview server-side,
so preview first with 'n8n source-control status --direction push' and pass the
file IDs and types it reports.

Pass --commit-message once and --file once per file as ID:TYPE, e.g.
--file abc:workflow, or supply the whole request document with --input FILE or
--input -. The two selection ways are mutually exclusive except that --force
may combine with either: without it a push containing unresolved conflicts is
rejected with 409 and nothing is pushed.

This mutates the remote Git branch. Interactive runs ask for confirmation and
non-interactive runs require --yes. Requires sourceControl:push and a licensed,
connected Source Control feature.

```
n8n source-control push [flags]
```

### Examples

```
  n8n source-control status --direction push
  n8n source-control push --commit-message "sync workflows" --file abc:workflow
  n8n source-control push --commit-message "sync" --file abc:workflow --file cred-1:credential --force --yes
  n8n source-control push --input push.json --yes --output json
```

### Options

```
      --commit-message string   commit message for the push, 1 to 1000 characters (required unless --input is used)
      --context string          saved context to use (default: the current context; see 'n8n config context list')
      --file stringArray        file to push as ID:TYPE, repeatable (cannot combine with --input; types: credential, workflow, tags, variables, file, folders, project, datatable)
      --force                   push despite unresolved conflicts (default: reject a conflicted push with 409)
  -h, --help                    help for push
      --input string            push JSON file path with commitMessage and fileNames, or - for stdin (cannot combine with --commit-message or --file; maximum 1 MiB)
      --output string           output format: text or json (JSON is the pushed files returned by the API) (default "text")
      --url string              instance URL override, e.g. https://n8n.example.com (default: the context URL, else N8N_URL)
      --yes                     confirm pushing to the remote Git branch without prompting (required when stdin is not interactive)
```

### SEE ALSO

* [n8n source-control](n8n_source-control.md)	 - Preview, push, and pull source-controlled instance changes

