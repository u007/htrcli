.PHONY: build run test clean install

# HTRCLI_VERSION is stamped into commands.Version; "dev" when git cannot describe.
HTRCLI_VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
VERSION_LDFLAGS := -ldflags "-X github.com/u007/htrcli/internal/commands.Version=$(HTRCLI_VERSION)"

build:
	go build $(VERSION_LDFLAGS) -o bin/htrcli ./cmd/htrcli

run:
	go run ./cmd/htrcli

test:
	go test ./...

clean:
	rm -rf bin/

install:
	go install $(VERSION_LDFLAGS) ./cmd/htrcli
