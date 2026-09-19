# Custom release notes

Optional per-release body text. Filename must match the tag exactly:

```sh
.github/release-notes/v0.2.0.md   # ships on tag v0.2.0
```

Flow (maintainer only):

```sh
git checkout main && git pull
make check
# optional custom body, via PR so CI gates it:
git checkout -b chore/release-notes-vX.Y.Z
cat > .github/release-notes/vX.Y.Z.md <<'EOF'
## Highlights

- One line per change.
EOF
make release-notes VERSION=vX.Y.Z && cat dist/release-notes.md  # preview
git add .github/release-notes/vX.Y.Z.md && git commit -m "chore(release): notes for vX.Y.Z"
git push origin chore/release-notes-vX.Y.Z
# open PR, wait for CI, squash-merge. Then tag the merge result:
git checkout main && git pull
git tag vX.Y.Z && git push origin vX.Y.Z
```

Rules:

- Tag after merge, never before. The tag must point at a commit on `main`.
  Tagging a pre-merge commit runs the full CI matrix twice on the same tree
  (once for the PR, once for the release) and leaves the tag off-branch.
- Verify before tagging:
  `git checkout main && git pull && git log --oneline --decorate -5`
  must show the target SHA as `main` HEAD. Never tag a PR branch head.
  Precedent: v0.2.0 tagged its branch head, not the `main` squash-merge,
  so v0.3.0 auto notes fell back to v0.1.0 and listed #1-11.
- Auto list (`generate_release_notes`) uses nearest ancestor tag as previous.
  Off-branch tag -> fallback to older tag -> full list instead of last-version
  delta. Release workflow fails off-branch tags fast; keep that guard.
- Auto list includes every merged PR, including `chore(release)` notes/docs.
  Highlights file stays curated; `What's Changed` stays auto. To ship a
  feat-only body (e.g. v0.3.0 = only #10), curate after publish and keep the
  compare link to the last version:

```sh
gh release view vX.Y.Z --json body --jq .body  # inspect
# trim What's Changed to feat PRs only, drop New Contributors when none,
# set Full Changelog to .../compare/vPrev...vX.Y.Z
gh release edit vX.Y.Z --notes-file /tmp/notes.md
gh release view vX.Y.Z --json body --jq .body  # verify
```
- Never push `main` directly (branch rules reject it). Push the notes branch
  and the tag only.
- File must exist on the tagged commit. Tag first, add file later -> too late.
- Contents ship verbatim above the GitHub auto-generated PR list (`generate_release_notes` stays on).
- No file for a tag -> release ships with generated notes only, no failure.
- Resolved locally via `make release-notes VERSION=vX.Y.Z` into `dist/release-notes.md`.
