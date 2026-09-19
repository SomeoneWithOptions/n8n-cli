# Custom release notes

Optional per-release body text. Filename must match the tag exactly:

```sh
.github/release-notes/v0.2.0.md   # ships on tag v0.2.0
```

Flow:

```sh
git checkout main && git pull
make check
# optional custom body:
cat > .github/release-notes/vX.Y.Z.md <<'EOF'
## Highlights

- One line per change.
EOF
git add .github/release-notes/vX.Y.Z.md && git commit -m "chore(release): notes for vX.Y.Z"
git tag vX.Y.Z && git push origin main vX.Y.Z
```

Rules:

- File must exist on the tagged commit. Tag first, add file later -> too late.
- Contents ship verbatim above the GitHub auto-generated PR list (`generate_release_notes` stays on).
- No file for a tag -> release ships with generated notes only, no failure.
- Resolved locally via `make release-notes VERSION=vX.Y.Z` into `dist/release-notes.md`.
