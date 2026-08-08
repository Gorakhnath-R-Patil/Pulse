MODULE     := github.com/Gorakhnath-R-Patil/Pulse
BIN_DIR    := bin
VERSION    ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
COMMIT     ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo none)
BUILD_DATE ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)

LDFLAGS := -X '$(MODULE)/internal/version.Version=$(VERSION)' \
           -X '$(MODULE)/internal/version.Commit=$(COMMIT)' \
           -X '$(MODULE)/internal/version.BuildDate=$(BUILD_DATE)'

CMDS := pulse-agent pulse-collector pulse-cli

.PHONY: all build $(CMDS) test test-race cover vet fmt fmt-check tidy clean run-agent run-collector ci

all: build

## build: compile all three binaries into bin/, with version info embedded.
build: $(CMDS)

$(CMDS):
	go build -ldflags "$(LDFLAGS)" -o $(BIN_DIR)/$@ ./cmd/$@

## test: run the unit test suite.
test:
	go test ./...

## test-race: run the unit test suite with the race detector enabled.
test-race:
	go test -race ./...

## cover: run tests and report per-package coverage.
cover:
	go test ./... -coverprofile=coverage.out
	go tool cover -func=coverage.out

## vet: run go vet static analysis.
vet:
	go vet ./...

## fmt: reformat all Go source with gofmt.
fmt:
	gofmt -w .

## fmt-check: fail if any file is not gofmt-formatted, without modifying it.
fmt-check:
	@files="$$(gofmt -l .)"; \
	if [ -n "$$files" ]; then \
		echo "the following files are not gofmt-formatted:"; \
		echo "$$files"; \
		exit 1; \
	fi

## tidy: sync go.mod/go.sum with the current source.
tidy:
	go mod tidy

## clean: remove build artifacts.
clean:
	rm -rf $(BIN_DIR) coverage.out

## run-agent: build and run pulse-agent with the example config.
run-agent: pulse-agent
	./$(BIN_DIR)/pulse-agent --config examples/config/agent.example.yaml

## run-collector: build and run pulse-collector with the example config.
run-collector: pulse-collector
	./$(BIN_DIR)/pulse-collector --config examples/config/collector.example.yaml

## ci: the checks that must pass before a commit — mirrors .github/workflows/ci.yml.
ci: fmt-check vet test-race
