## n8n source-control pull

Pull remote Git changes into this instance

### Synopsis

Fetch the connected Git branch into this instance, rewriting local instance
content from the remote. Preview first with
'n8n source-control status --direction pull' and check the conflict markers:
without --force a pull blocked by uncommitted local changes or merge conflicts
is rejected with 409 and nothing changes.

--force discards those local changes to complete the pull and cannot be undone.
--auto-publish controls imported workflows: none keeps every workflow in its
local published state, all publishes every imported workflow, and published
publishes only workflows that were published locally before the import.

This mutates the local instance. Interactive runs ask for confirmation and
non-interactive runs require --yes. Requires sourceControl:pull and a licensed,
connected Source Control feature.

```
n8n source-control pull [flags]
```

### Examples

```
  n8n source-control status --direction pull
  n8n source-control pull
  n8n source-control pull --auto-publish published --yes
  n8n source-control pull --force --yes --output json
```

### Options

```
      --auto-publish string   workflow publishing after import: none, all, or published (default: none) (default "none")
      --context string        saved context name to use (falls back to N8N_CONTEXT, then the saved current context; see 'n8n config context list')
      --force                 discard local changes and force the pull to complete (default: reject a conflicted pull with 409)
  -h, --help                  help for pull
      --output string         output format: text or json (JSON is the array of pulled files) (default "text")
      --url string            instance URL override, e.g. https://n8n.example.com (falls back to N8N_URL, then the selected context URL)
      --yes                   confirm rewriting local instance content without prompting (required when stdin is not interactive)
```

### SEE ALSO

* [n8n source-control](n8n_source-control.md)	 - Preview, push, and pull source-controlled instance changes

