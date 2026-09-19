# Security Policy

## Supported versions

Latest `v*` release only. Older tags get no patches.

## Release integrity

Releases are not signed. `install.sh`, `install.ps1` and `n8n update` fetch
assets over HTTPS and verify them against the `checksums.txt` published with the
same release, which proves the download arrived intact, not who produced it. A
compromised release or repository would therefore still install. `n8n update`
additionally runs the downloaded binary once and refuses it unless it reports
the expected release tag.

## Reporting

Do **not** open public issues for vulnerabilities.

Use **Security → Advisories → New draft advisory** (private):
https://github.com/SomeoneWithOptions/n8n-cli/security/advisories/new

Include: version (`n8n version`), platform, repro, impact. Redact secrets.

Maintainer (`@SomeoneWithOptions`) triages and releases fix as new `v*` tag. Credit on request.
