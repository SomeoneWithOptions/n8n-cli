# n8n CLI

Command-line client for the n8n public API.

## Install

macOS and Linux, into `~/.local/bin`:

```sh
curl -fsSL https://raw.githubusercontent.com/SomeoneWithOptions/n8n-cli/main/install.sh | sh
```

Windows (PowerShell), into `%LOCALAPPDATA%\n8n-cli\bin`:

```powershell
irm https://raw.githubusercontent.com/SomeoneWithOptions/n8n-cli/main/install.ps1 | iex
```

If PowerShell blocks the pipe, bypass the policy for that one command:

```powershell
powershell -ExecutionPolicy Bypass -Command "irm https://raw.githubusercontent.com/SomeoneWithOptions/n8n-cli/main/install.ps1 | iex"
```

Both scripts install the binary and nothing else. Neither creates or edits
contexts or credentials; `n8n auth login` owns that state. The Unix script warns
when the install directory is not on `PATH`; the Windows script appends it to
the user `PATH` and never touches the machine `PATH`.

Environment overrides, same names on both platforms:

| Variable | Effect |
|---|---|
| `N8N_CLI_VERSION` | Install this tag instead of the latest release |
| `N8N_CLI_INSTALL_DIR` | Install into this directory instead of the default |

With a Go toolchain:

```sh
go install github.com/SomeoneWithOptions/n8n-cli/cmd/n8n@latest
```

Or download a binary directly from the
[releases page](https://github.com/SomeoneWithOptions/n8n-cli/releases). Assets
are plain binaries named `n8n-<os>-<arch>` (`n8n-windows-amd64.exe` on Windows)
for `linux/amd64`, `linux/arm64`, `darwin/amd64`, `darwin/arm64` and
`windows/amd64`, alongside `checksums.txt`:

```sh
curl -fsSLO https://github.com/SomeoneWithOptions/n8n-cli/releases/latest/download/n8n-linux-amd64
curl -fsSLO https://github.com/SomeoneWithOptions/n8n-cli/releases/latest/download/checksums.txt
sha256sum --check --ignore-missing checksums.txt
chmod +x n8n-linux-amd64 && mv n8n-linux-amd64 ~/.local/bin/n8n
```

Verify the install with `n8n version`, which reports the release tag, commit and
build date.

## Shell completion

```sh
n8n completion bash > /etc/bash_completion.d/n8n     # bash
n8n completion zsh > "${fpath[1]}/_n8n"              # zsh
n8n completion fish > ~/.config/fish/completions/n8n.fish
n8n completion powershell | Out-String | Invoke-Expression
```

`n8n completion --help` documents per-shell placement and reloading.

## Getting started

```sh
n8n auth login --url https://n8n.example.com
n8n auth status --check
n8n discover
```

## Status

Early development, pre-1.0: minor versions may change command behavior. The CLI
version is independent of the n8n API version it talks to. Commands cover audit,
auth, community packages, config, credentials, data tables, discover,
evaluations, executions, folders, Git connections, insights, LDAP, log streams,
node policies, OIDC, OpenTelemetry, packages (beta), projects, promotions,
roles, role mappings, SAML, security policy, source control, tags, users,
variables, workflows and version.

## Command reference

[`docs/README.md`](docs/README.md) is generated from the command help.
Regenerate it with `make docs` after changing any command's help text.

## Development

`make check` is the definition of green: formatting, vet, staticcheck, tests
with and without the race detector, cross-compilation of every release target,
module tidiness and `govulncheck`. CI runs the same targets on Linux, macOS and
Windows; `make dist` builds the release artifacts the release workflow
publishes.

## License

MIT. See [LICENSE](LICENSE).
