# These targets are the single definition of "green" for this project.
# CI in Phase 29 runs the same ones; keep them in sync by calling them, not
# by reimplementing them in a workflow file.

BINARY      := n8n
BIN_DIR     := bin
PKG         := github.com/SomeoneWithOptions/n8n-cli
VERSION_PKG := $(PKG)/internal/version

VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
COMMIT  ?= $(shell git rev-parse HEAD 2>/dev/null || echo unknown)
DATE    ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)

LDFLAGS := -s -w \
	-X $(VERSION_PKG).version=$(VERSION) \
	-X $(VERSION_PKG).commit=$(COMMIT) \
	-X $(VERSION_PKG).date=$(DATE)

.PHONY: all check fmt fmt-check test test-race vet lint vuln build clean tidy docs spec integration

all: check build

## check: everything that must pass before a change lands.
check: fmt-check vet lint test test-race vuln

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

## vet: standard toolchain checks.
vet:
	go vet ./...

## lint: staticcheck, pinned as a go tool directive.
lint:
	go tool staticcheck ./...

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
