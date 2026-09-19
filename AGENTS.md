# AGENTS.md

Instructions for AI coding agents working in this repo. Single source of truth for Codex, Pi, Claude, Cursor, Copilot, opencode. Keep this file under 32 KiB so Codex loads it fully. Prefer editing this file over duplicating guidance elsewhere.

## Commands

```sh
make check   # single definition of green: fmt-check + vet + lint + test + test-race + cross + tidy-check + vuln
make build   # ./bin/n8n
make smoke   # version + help + completion sanity
make docs    # regenerate docs/README.md from Cobra help (required when help text changes)
go test ./internal/docs -run TestDocsAreCurrent -update  # same as make docs
go test -race -shuffle=on -count=1 ./...  # order-independence check
```

CI (`.github/workflows/ci.yml`) runs the same `make` targets on ubuntu/macos/windows. No logic lives in YAML that `make` cannot run locally. Integration tests stay off in CI (no secrets).

## Project shape

```text
cmd/n8n/              binary entry point only, thin
internal/cli/         Cobra commands; one file per group (tag.go, workflow.go) + *_test.go
internal/n8n/         typed API client; one file per resource + *_test.go
internal/config/      contexts, URL, credential resolution
internal/output/      JSON + human rendering (see internal/cli/output.go)
internal/confirm/     destructive-action guard (see internal/confirm/confirm.go)
internal/coverage/    endpoint manifest + contract tests (manifest.json is committed artifact)
internal/deps/        dependency allowlist (internal/deps/deps.go)
internal/docs/        command reference generated from Cobra help
internal/workflowdiff/ CLI-native diff engine (no spec operation behind it)
test/integration/     opt-in live tests, skip cleanly without env
docs/                 generated from help; never hand-edit prose that duplicates help
```

`cmd/n8n/main.go` stays thin. Business behavior belongs in testable packages. Pass dependencies via `internal/cli.Options` (streams, config dir, env, keyring, prompt, interactivity, HTTP client). No package-level globals, no `init()` command registration. See `internal/cli/cli.go`.

## Two tracks — pick one first

### Track A: spec-backed command (default when an OpenAPI operation exists)

Source of truth is the union contract: `openapi.yml` (target instance) + `openapi.upstream.yml` (docs instance). Both are gitignored. Committed artifact is `internal/coverage/manifest.json` with `availability: both | target-only | upstream-only`.

Rules:

- Confirm method, path, scope, params, body, response codes, schemas against whichever document declares the operation.
- Match operations by method + path with `{param}` normalized. Read `operationId` first, fall back to `x-eov-operation-id`. Never assume one server is a superset of the other. Never delete an implemented operation because one instance stopped serving it.
- Add one typed client method in `internal/n8n/<resource>.go` + one Cobra command in `internal/cli/<resource>.go`.
- Update `manifest.json` (`command`, `availability`, phase). Coverage test cross-checks manifest against each document that declares the operation and skips cleanly when a document is absent.
- Upstream-only / target-only commands ship unconditionally. An instance lacking the module answers 404/503, which the CLI reports as "not available to you" with a `discover` pointer. Integration tests for such groups run by default and skip on 404/503.

### Track B: CLI-native feature (when NO spec operation exists)

Spec is the default source, not a gate. If no operation matches, build Track B. Never force-fit `manifest.json`, never invent an `operationId`.

Rules:

- Mark the command `Annotations: map[string]string{"cliOnly": "true"}` (precedent: `internal/cli/workflow_diff.go`).
- Do not touch `internal/coverage/manifest.json`.
- Compose existing client methods; put new logic in a new package (`internal/<feature>/`), keep the Cobra wrapper thin.
- State in `Long` why this is CLI-side (what it composes, what it never mutates).
- Full shared contracts below still apply (help, output, confirm, secrets, tests, docs).
- Precedent: `n8n workflow diff` compares saved definitions across two contexts without any spec operation. See `internal/cli/workflow_diff.go`, `internal/workflowdiff/diff.go`.

Decision rule: matching method + path in either spec -> Track A. Otherwise -> Track B.

## Shared contracts (both tracks)

### 1. Client (`internal/n8n/`)

- Validate before transport: enums, limits, IDs, dates, strict JSON, 1 MiB input cap, unknown fields rejected. Never spend a round trip on what can fail locally.
- Escape every ID with `PathJoin` as one path segment (`internal/n8n/tag.go`). Encode queries with `url.Values.Encode`.
- Separate write documents from read models so read-only fields (`id`, timestamps, server-owned fields) are never sent back. Filter + report dropped fields on stderr (precedent: `WorkflowDocument.ForCreate/ForUpdate`).
- Pointers distinguish omitted vs `false` / `0` / `""` / `null`. Nil slice encodes as `[]`, never `null`. PATCH refuses an empty document; PUT documents full-replacement semantics.
- Keep open-ended payloads as `json.RawMessage` (workflow definitions, rows, metrics) + `Extra` map for unknown fields so `get --output json` round-trips losslessly. Decode polymorphic responses by shape, not by request.
- Redact secrets: redacted `String`/`GoString`/`MarshalJSON` + `slog.LogValuer` on credential types; strip server-echoed secrets from errors while keeping status/path/request ID; force non-conforming plaintext responses back to the sentinel (`**hidden**` family). Secrets never enter argv.

Canonical simple example: `internal/n8n/tag.go` (115 lines). Complex examples: `internal/n8n/datatable.go` (filters, polymorphic rows), `internal/n8n/workflow.go` (raw definition + read-only filtering).

### 2. CLI (`internal/cli/`)

- Every command sets `Short` (imperative, one line), `Long` (what, when vs siblings, prereqs, side effects, output shape, next step), `Example` (copy-pasteable; two when interactive/non-interactive differ). Parent groups document workflow order (`list` first) and name the child to start with.
- Every flag usage states format + default + fallback (e.g. `--context`, `--url`, `--output`, `--yes`, `--input`). Never leave an empty usage string. Enforced by `TestHelpIsDescriptive` and `TestHelpSucceedsAtEveryLevel` in `internal/cli/help_test.go`.
- Shared flags: `instanceFlags` (`--context`, `--url`) via `Options.apiClient` (`internal/cli/instance.go`); `--output text|json` validated by `validateOutput` (`internal/cli/output.go`).
- stdout = machine-readable result only. stderr = diagnostics, prompts, confirmations, next-step hints. `--output json` is stable for scripts; text uses `text/tabwriter`. Never color redirected output. Empty success exits 0 with a stderr hint, not an error.
- Complex bodies: `--input FILE` + `--input -`, capped at 1 MiB, strict decode (reject unknown fields + trailing documents). Flag-based and JSON-based inputs are mutually exclusive; one is required where applicable. `--input -` (stdin) always requires `--yes` on mutating commands because stdin cannot answer a prompt.
- Errors restate the fix because agents act on stderr: 401 names `n8n auth login` (or `n8n auth status` when the credential comes from env); 403 names the scope + license + `n8n discover --resource <key>` with the plural key the instance filters on; 404/503 share one availability diagnostic; 409 names the conflict cause + remediation. See `apiError` in `internal/cli/instance.go` plus per-resource wrappers (e.g. `tagAPIError`).

### 3. Destructive commands

- Rule: interactive run asks, non-interactive refuses unless `--yes`. Missing terminal is never consent. Confirm before transport so refusal sends no request. See `internal/confirm/confirm.go`, usage in `internal/cli/tag.go:runTagDelete`.
- Prompt names the exact blast radius (tag removed from all workflows vs workflows deleted; remote-branch rewrite vs local overwrite; identities deleted; checkout loss). Stronger variant for `--force` / hard-delete paths.
- No client-side dry-run unless the API supports it. Server dry-run (e.g. data-table row `--dry-run`) prints "dry run, nothing was changed" and skips confirmation because nothing is deleted.

### 4. Pagination

- List commands expose `--limit`, `--cursor`, `--all`; offset-style resources use `--skip`/`--take`. Cap `--all` at 10,000 (`n8n.DefaultCollectLimit`). Preserve filters across pages. Stop on short page / reported total / cap so a server ignoring the cursor cannot loop. Offset + cursor are mutually exclusive. Reject `--all` with unbounded detailed data (e.g. execution `--include-data` must page explicitly).

### 5. Dependencies (closed list)

Allowed direct requires live in `internal/deps/deps.go:Allowed`: `cobra`, `pflag`, `go-keyring`, `x/sys`, `x/term` (runtime); `go-cmp`, `yaml.v3` (tests). Adding a module is a deliberate edit to that allowlist, not an implementation detail. Stdlib covers HTTP, JSON, `log/slog`, `text/tabwriter`, `mime/multipart`, `os.Root`, testing + `httptest`. No assertion / mocking / CLI-testing framework. `staticcheck` + `govulncheck` are `go tool` directives, never binary imports.

## Tests (required per command)

- Client: `httptest.Server` covering method, escaped path, query encoding, auth header, body, success decode, non-2xx, empty 204/201, malformed JSON, cancellation/timeout.
- CLI in-process (`Options` with injected streams + fake client/server): arg validation, exit code, stdout, stderr, JSON output, confirmation refusal before transport, `--input -` requiring `--yes`.
- Help: new paths added to `TestHelpSucceedsAtEveryLevel`; `TestHelpIsDescriptive` must pass (Short/Long/Example + flag usage).
- Pagination where applicable: two-page collection at both layers, filters preserved.
- Docs: run `make docs`; `TestDocsAreCurrent` fails when stale.
- Integration (`test/integration/`): opt-in, skip cleanly without env. Destructive paths require explicit env (e.g. `N8N_INTEGRATION_DESTRUCTIVE=1`) and use `n8n-cli-test-` prefix + timestamp with cleanup registered before the first assertion. Read-only where the resource is identity- or instance-wide (roles, users, LDAP/SSO settings, source-control push/pull, package import). Never read-modify-write a resource the test did not create. Union-contract groups skip on 404/503, never fail.

## Common traps (from shipped phases)

- Audit returns `[]` when clean (not `{}`) and has no `{"data": ...}` envelope; accept empty array, reject populated array rather than dropping it.
- Tag names cap at ~24 chars and report overlong as 409 "already exists" — help + 409 message must name both causes.
- Workflow create answers 200 (not 201); sending a read `id` back is `400 request/body/id is read-only`; `description: null` on read cannot be replayed — drop nulls the schema does not type as nullable.
- Data-table insert `returnType=count|id` answers `{"success":true,"insertedRows":n}` / `[{"id":n}]`, not the documented shapes; upsert with `returnData` answers a one-row array. Column type `json` is documented but refused by some instances — still accept, warn in help.
- Folder list is offset (`skip`/`take`, `{count, data}`), not cursor; `sortBy` must be percent-encoded; only `create` accepts the literal project ID `personal`; `select: project|tags|parentFolder` 500s on some instances.
- `discoverHint` keys are the lowercase tag names the instance reports (`tags`, `datatable`, `sourcecontrol`, ...), not the CLI group spellings.
- `Promotions` and `GitConnections` share the `gitConnection:*` scope namespace — distinguish by served paths in `/discover`, not by scopes.
