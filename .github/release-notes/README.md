# Release runbook

Default and standard flow: tag `main` HEAD directly, let CI publish the release, then curate the release notes via `gh release edit`. No extra PR or commit needed.

Flow (maintainer only):

```sh
# 1. Pre-flight (ensure main is up to date and all checks pass)
git checkout main && git pull
make check

# 2. Verify HEAD is the commit to release
git log --oneline --decorate -3

# 3. Tag main HEAD and push (triggers release.yml)
git tag vX.Y.Z && git push origin vX.Y.Z

# 4. Monitor release run
gh run list --workflow=Release --limit 1
# check status:
gh run view <run-id> --json status,conclusion

# 5. Curate release notes on GitHub
gh release view vX.Y.Z --json body --jq .body
# Edit /tmp/notes.md: keep user-facing feats/fixes and compare link, strip chore PRs
gh release edit vX.Y.Z --notes-file /tmp/notes.md
gh release view vX.Y.Z --json body --jq .body  # verify
```

Rules:

- Tag `main` HEAD directly, never an unmerged branch head. Release workflow fails off-branch tags fast.
- Tags cannot be deleted or updated (enforced by `release-tags` ruleset).
- Curate notes post-publish: GitHub auto-generates notes from all merged PRs (including chore PRs). Curate the release body with `gh release edit` so it highlights only user-facing features/fixes and keeps the compare link.
- Optional legacy path: If a `.github/release-notes/vX.Y.Z.md` file exists on the tagged commit, its text ships above the auto-generated notes. If absent, release ships with auto-generated notes only (which you curate post-publish).

