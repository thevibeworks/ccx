.PHONY: build test clean install lint fmt

VERSION ?= dev
BUILD_TIME := $(shell date -u '+%Y-%m-%dT%H:%M:%SZ')
LDFLAGS := -ldflags "-X main.version=$(VERSION) -X main.buildTime=$(BUILD_TIME)"

build:
	go build $(LDFLAGS) -o bin/ccx ./cmd/ccx

install:
	go install $(LDFLAGS) ./cmd/ccx

test:
	go test -v ./...

lint:
	golangci-lint run

fmt:
	gofmt -s -w .
	goimports -w .

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
