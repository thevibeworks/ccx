.PHONY: build build-all build-darwin-arm64 build-darwin-amd64 build-linux-amd64 build-linux-arm64
.PHONY: test clean install lint fmt deps run tools skill verify-pricing dev devweb

VERSION ?= dev
BUILD_TIME := $(shell date -u '+%Y-%m-%dT%H:%M:%SZ')
LDFLAGS := -ldflags "-X main.version=$(VERSION) -X main.buildTime=$(BUILD_TIME)"
GOBIN := $(or $(shell go env GOBIN),$(shell go env GOPATH)/bin)

# Default: build for current OS/arch
build:
	@go build $(LDFLAGS) -o bin/ccx ./cmd/ccx
	@echo ""
	@echo "  ccx built successfully"
	@echo "  ─────────────────────────────────────"
	@echo "  Binary:  bin/ccx ($$(go env GOOS)/$$(go env GOARCH))"
	@echo "  Version: $(VERSION)"
	@echo ""
	@echo "  Quick start:"
	@echo "    ./bin/ccx --help"
	@echo "    ./bin/ccx web"
	@echo ""
	@echo "  Docs: https://github.com/thevibeworks/ccx"
	@echo ""

# Cross-platform builds (pure Go, CGO disabled)
build-darwin-arm64:
	@CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 go build $(LDFLAGS) -o bin/ccx-darwin-arm64 ./cmd/ccx
	@echo "Built bin/ccx-darwin-arm64"

build-darwin-amd64:
	@CGO_ENABLED=0 GOOS=darwin GOARCH=amd64 go build $(LDFLAGS) -o bin/ccx-darwin-amd64 ./cmd/ccx
	@echo "Built bin/ccx-darwin-amd64"

build-linux-amd64:
	@CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build $(LDFLAGS) -o bin/ccx-linux-amd64 ./cmd/ccx
	@echo "Built bin/ccx-linux-amd64"

build-linux-arm64:
	@CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build $(LDFLAGS) -o bin/ccx-linux-arm64 ./cmd/ccx
	@echo "Built bin/ccx-linux-arm64"

build-all: build-darwin-arm64 build-darwin-amd64 build-linux-amd64 build-linux-arm64
	@echo "Built all platforms in bin/"

install:
	go install $(LDFLAGS) ./cmd/ccx

test:
	go test -v ./...

# Install dev tools (golangci-lint, goimports)
tools:
	@echo "Installing dev tools to $(GOBIN)..."
	go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest
	go install golang.org/x/tools/cmd/goimports@latest
	@echo "Done."

lint:
	@test -x $(GOBIN)/golangci-lint || { echo "golangci-lint not found. Run 'make tools' first."; exit 1; }
	$(GOBIN)/golangci-lint run

fmt:
	gofmt -s -w .
	@test -x $(GOBIN)/goimports && $(GOBIN)/goimports -w . || true

clean:
	rm -rf bin/

deps:
	go mod download
	go mod tidy

run: build
	./bin/ccx

run-projects: build
	./bin/ccx projects

run-doctor: build
	./bin/ccx doctor

# Dev loop: rebuild + run the web UI. Override PORT / HOST on the
# command line for non-default ports (`make devweb PORT=18080`), or
# set NO_OPEN=1 to skip auto-launching the browser (handy when
# iterating over SSH or in CI-adjacent tests).
#
# Kills any prior `ccx web` instance bound to the same PORT before
# starting so back-to-back invocations don't fail with "address
# already in use".
PORT   ?= 8080
HOST   ?= localhost
NO_OPEN ?=
devweb: build
	@-pkill -f "bin/ccx web.*--port $(PORT)" 2>/dev/null; sleep 0.2
	@echo ""
	@echo "  ccx web (dev)"
	@echo "  ─────────────────────────────────────"
	@echo "  URL:  http://$(HOST):$(PORT)"
	@echo "  Ctrl-C to stop."
	@echo ""
	@./bin/ccx web --host $(HOST) --port $(PORT) $(if $(NO_OPEN),--no-open,)

# Default dev alias — fastest "rebuild + run web" loop. Templates
# are embedded at compile time, so every edit to internal/web/
# requires a rebuild, which this target makes a single command.
dev: devweb

skill:
	cd skills && zip -r ../ccx.skill ccx/
	@echo "Packaged: ccx.skill"

# verify-pricing compares ccx's embedded pricing table against Claude
# Code's source of truth (modelCost.ts). Requires a local checkout of
# claude-code under ../../reference/claude-code-2188 by default; pass
# CLAUDE_SOURCE=<path> to override. Exits non-zero on drift.
CLAUDE_SOURCE ?= ../../reference/claude-code-2188
verify-pricing:
	@go run ./cmd/ccx-verify-pricing --claude-source $(CLAUDE_SOURCE)
