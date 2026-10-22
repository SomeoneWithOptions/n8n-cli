# AGENTS.md

- Green = `make check`. Run `make docs` after any help text change.
- Releases: follow `.github/release-notes/README.md`. Tag `main` HEAD only; tags are immutable.

## New commands

- Operation exists in `openapi.yml` or `openapi.upstream.yml` (match method + path): add client method + command, update `internal/coverage/manifest.json`. Never assume one spec is a superset of the other; never delete an op one instance stopped serving, mark it `retired`.
- No matching operation: CLI-native command with `Annotations: {"cliOnly": "true"}`, logic in its own `internal/<feature>/` package, no manifest change. Precedent: `workflow diff`.
- New upstream ops go into new `planned` phases, never delivered ones.

## Rules

- Validate locally before any request.
- Never send read-only fields back; filter and report dropped fields on stderr.
- stdout = result only; diagnostics, prompts, and hints go to stderr.
- Destructive commands: prompt when interactive, refuse without `--yes` otherwise, before any request. `--input -` always requires `--yes`.
- Error messages state the fix (which command to run, which scope is missing).
- Secrets never enter argv, logs, or error output.
- New dependencies require editing `internal/deps/deps.go:Allowed`. No test or mocking frameworks.
- Each command needs client + CLI tests. Integration tests are opt-in, skip without env, and never mutate resources they did not create.
