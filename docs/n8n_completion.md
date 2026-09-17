## n8n completion

Generate a shell completion script

### Synopsis

Generate a shell completion script for n8n.

The script is written to stdout; redirect or source it as your shell expects.
Completions cover commands, flags, and context names where known.

Examples:
  bash:       source <(n8n completion bash)
  zsh:        n8n completion zsh > "${fpath[1]}/_n8n"
  fish:       n8n completion fish > ~/.config/fish/completions/n8n.fish
  powershell: n8n completion powershell | Out-String | Invoke-Expression

```
n8n completion [bash|zsh|fish|powershell] [flags]
```

### Examples

```
  source <(n8n completion bash)
  n8n completion zsh > "${fpath[1]}/_n8n"
```

### Options

```
  -h, --help   help for completion
```

### SEE ALSO

* [n8n](n8n.md)	 - Command-line client for the n8n API

