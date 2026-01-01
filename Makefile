.PHONY: build build-all build-darwin-arm64 build-darwin-amd64 build-linux-amd64 build-linux-arm64
.PHONY: test clean install lint fmt deps run tools

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
