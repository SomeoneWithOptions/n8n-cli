# Contributing to n8n-cli

Thanks for input. Contribute via fork + pull request, or via issues.

## Ownership model

- Only `@SomeoneWithOptions` can push/merge to `main` and push `v*` tags.
- Releases run only on `v*` tags via `.github/workflows/release.yml`.
- No direct pushes to `main`, including maintainer — use PRs so CI gates everything.
- There are no additional collaborators with write access. If collaborators are added later, branch/tag rulesets still enforce this.

## Ways to contribute

1. **Report bug / request feature:** open issue with repro steps, version (`n8n version`), OS/arch.
2. **Fix / feature:** fork repo, branch off `main`, open PR against `main`.
3. **Docs:** same PR flow. Regenerate command ref with `make docs` if help text changed.

## Local dev

Prereqs: Go toolchain per `go.mod`.

```sh
make check   # fmt-check + vet + lint + test + test-race + cross + tidy-check + vuln
make build   # ./bin/n8n
make smoke   # version + help + completion sanity
make docs    # regenerate docs/README.md from Cobra help
```

CI (`.github/workflows/ci.yml`) runs same targets on ubuntu/macos/windows plus `install.sh` / `install.ps1` checks. If CI fails, repro with one `make` command above.

## PR rules

- One logical change per PR. Keep diff small.
- Fork branch name: `fix/<short>` or `feat/<short>`.
- Must pass CI. Must update `docs/README.md` via `make docs` if CLI help changed.
- Maintainer squashes/merges. Branches from forks do not need to be up to date — maintainer handles conflicts.
- No release commits in PRs. No `v*` tags in PRs. Maintainer tags separately.
- Be civil. No secrets/keys/tokens in issues/PRs/logs.

## Releases (maintainer only)

```sh
git checkout main && git pull
make check
git tag vX.Y.Z && git push origin vX.Y.Z
```

Tag triggers `release.yml`: full CI matrix → `make dist` → GitHub Release → install-script self-test.

Custom body (optional): commit `.github/release-notes/vX.Y.Z.md` (name matches tag) before tagging. It ships above auto-generated notes; missing file means generated notes only. See `.github/release-notes/README.md`. Preview with `make release-notes VERSION=vX.Y.Z`.
