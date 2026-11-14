# Development tasks. CGO is required because the SQL parser is libpg_query.
GO        ?= go
BIN       := bin/prodshadow
PKGS      := ./...
VERSION   ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS   := -s -w -X github.com/BhuwanJoshi-01/prodshadow/internal/version.Version=$(VERSION)

export CGO_ENABLED = 1

.PHONY: all build test race integration examples lint fmt vet tidy fuzz clean

all: lint test build

build:
	$(GO) build -trimpath -ldflags '$(LDFLAGS)' -o $(BIN) ./cmd/prodshadow

test:
	$(GO) test $(PKGS)

race:
	$(GO) test -race $(PKGS)

# Requires Docker (or Podman with PRODSHADOW_IT_PROVIDER=podman).
integration:
	$(GO) test -tags integration -timeout 30m $(PKGS)

# Runs every example end to end and checks its documented outcome.
examples: build
	$(GO) test -tags integration -timeout 60m -run TestExamples ./tests/integration/

fuzz:
	$(GO) test -run XXX -fuzz FuzzSplit -fuzztime 60s ./internal/sqlsplit/

fmt:
	gofmt -w cmd internal schemas tests

vet:
	$(GO) vet $(PKGS)

lint: vet
	@test -z "$$(gofmt -l cmd internal schemas)" || (echo "run make fmt" && gofmt -l cmd internal schemas && exit 1)
	@command -v golangci-lint >/dev/null && golangci-lint run || echo "golangci-lint not installed; skipped (CI uses v2.15.0)"

tidy:
	$(GO) mod tidy

clean:
	$(GO) clean
	rm -rf bin dist prodshadow-results
