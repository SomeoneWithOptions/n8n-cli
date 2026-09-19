## n8n update

Update this CLI to the latest release

### Synopsis

Replace this binary with a release published on GitHub.

Use --check first: it reports the installed and the latest version and writes
nothing. A plain 'n8n update' asks before replacing the binary; --yes skips the
question for scripts. Nothing else is touched: contexts and credentials belong
to 'n8n auth login' and survive every update.

The asset for this platform is downloaded next to the current binary, checked
against the release checksums.txt, and run once to confirm it reports the
expected version before it is moved into place. A failure at any of those steps
leaves the installed binary untouched. Those checksums prove the download
arrived intact, not who built it; releases are not signed yet.

Refused unless --force is passed: a development build, because its version
cannot be compared with a release, and a binary that a package manager owns
(Homebrew, Nix, Snap, 'go install', a system package), because that manager
should do the update instead. A binary installed by install.sh, install.ps1 or
a manual download updates in place. --version installs an exact tag, including
an older one.

```
n8n update [flags]
```

### Examples

```
  n8n update --check
  n8n update --check --output json
  n8n update
  n8n update --yes
  n8n update --version v1.4.0 --yes
```

### Options

```
      --check            report the installed and latest versions without writing anything
      --force            update a development build or a package-manager-owned binary, and reinstall the current version
  -h, --help             help for update
      --output string    output format: text or json (default "text")
      --version string   install this exact release tag (for example v1.4.0) instead of the latest
  -y, --yes              do not ask before replacing the binary
```

### SEE ALSO

* [n8n](n8n.md)	 - Command-line client for the n8n API

