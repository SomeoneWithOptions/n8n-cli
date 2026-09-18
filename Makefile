# These targets are the single definition of "green" for this project.
# CI in Phase 31 runs the same ones; keep them in sync by calling them, not
# by reimplementing them in a workflow file.

BINARY      := n8n
BIN_DIR     := bin
PKG         := github.com/SomeoneWithOptions/n8n-cli
VERSION_PKG := $(PKG)/internal/version

UPSTREAM_SPEC_URL ?= https://internal.users.n8n.cloud/api/v1/openapi.yml

# Analysers that are not module dependencies. They are pinned here rather than
# as tool directives so they cannot drift between runs and cannot drag their own
# dependency trees into go.mod.
GOPLS     := golang.org/x/tools/gopls@v0.23.0
DEADCODE  := golang.org/x/tools/cmd/deadcode@v0.44.0

# Every target the cross-compile gate must build. Pure Go, so no toolchain setup.
CROSS_TARGETS := linux/amd64 linux/arm64 darwin/amd64 darwin/arm64 windows/amd64

GO_FILES := $(shell find . -name '*.go' -not -path '*/.*')

VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
COMMIT  ?= $(shell git rev-parse HEAD 2>/dev/null || echo unknown)
DATE    ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)

LDFLAGS := -s -w \
	-X $(VERSION_PKG).version=$(VERSION) \
	-X $(VERSION_PKG).commit=$(COMMIT) \
	-X $(VERSION_PKG).date=$(DATE)

.PHONY: all check fmt fmt-check test test-race test-shuffle vet lint vuln analyze cross tidy-check deadcode build clean tidy docs spec spec-upstream integration

all: check build

## check: everything that must pass before a change lands.
check: fmt-check vet lint test test-race cross tidy-check vuln

## fmt: rewrite sources with gofmt.
fmt:
	gofmt -s -w .

## fmt-check: fail when sources are not gofmt-clean.
fmt-check:
	@files=$$(gofmt -s -l .); \
	if [ -n "$$files" ]; then \
		echo "not gofmt-clean:"; echo "$$files"; exit 1; \
	fi

## test: unit and component tests.
test:
	go test ./...

## test-race: same suite under the race detector.
test-race:
	go test -race ./...

## test-shuffle: same suite in randomised order, to catch order-dependent tests.
test-shuffle:
	go test -race -shuffle=on -count=1 ./...

## vet: standard toolchain checks.
vet:
	go vet ./...

## lint: staticcheck, pinned as a go tool directive. "all" adds the ST style
## and QF quickfix checks the default set omits.
lint:
	go tool staticcheck -checks="all" ./...

## analyze: gopls at both severities, deduped across build configurations.
## Deliberately not part of check: hint severity is advisory, and the tool is
## pinned in this file rather than in the build.
analyze:
	@echo "== gopls check (default severity)"
	@go run $(GOPLS) check $(GO_FILES) | grep -v '\[windows\]$$' || true
	@echo "== gopls check (hint severity)"
	@go run $(GOPLS) check -severity=hint $(GO_FILES) | grep -v '\[windows\]$$' | sort -u || true

## cross: every release target must compile, including the Windows-only
## keyring and ACL paths behind build tags.
cross:
	@for target in $(CROSS_TARGETS); do \
		os=$${target%/*}; arch=$${target#*/}; \
		echo "build $$os/$$arch"; \
		CGO_ENABLED=0 GOOS=$$os GOARCH=$$arch go build ./... || exit 1; \
	done

## tidy-check: fail when go.mod or go.sum is untidy.
tidy-check:
	go mod tidy -diff

## deadcode: cross-package unreachability from the real entry point.
deadcode:
	go run $(DEADCODE) ./cmd/n8n

## vuln: govulncheck, pinned as a go tool directive.
vuln:
	go tool govulncheck ./...

## docs: regenerate the command reference under docs/ from Cobra help.
docs:
	go test ./internal/docs -run TestDocsAreCurrent -update

## spec: fetch the instance OpenAPI document to ./openapi.yml (gitignored).
## Needs N8N_INTEGRATION_URL and N8N_INTEGRATION_API_KEY.
spec:
	@test -n "$(N8N_INTEGRATION_URL)" || { echo "set N8N_INTEGRATION_URL and N8N_INTEGRATION_API_KEY"; exit 1; }
	curl -fsS -H "X-N8N-API-KEY: $(N8N_INTEGRATION_API_KEY)" \
		"$(N8N_INTEGRATION_URL)/api/v1/openapi.yml" -o openapi.yml

## spec-upstream: fetch the n8n docs instance OpenAPI document to
## ./openapi.upstream.yml (gitignored). It declares the groups the target
## instance does not serve, so the union of both is the contract.
spec-upstream:
	curl -fsS "$(UPSTREAM_SPEC_URL)" -o openapi.upstream.yml

## integration: opt-in tests against a live instance. Skips without the env.
integration:
	go test -count=1 -v ./test/integration/...

## tidy: resolve and prune module requirements.
tidy:
	go mod tidy

## build: stamped binary in ./bin.
build:
	go build -trimpath -ldflags '$(LDFLAGS)' -o $(BIN_DIR)/$(BINARY) ./cmd/n8n

## clean: remove build output.
clean:
	rm -rf $(BIN_DIR)
