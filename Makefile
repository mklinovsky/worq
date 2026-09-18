BINARY := worq
PREFIX ?= $(HOME)/.local
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -s -w -X github.com/mklinovsky/worq/internal/cli.Version=$(VERSION)

.PHONY: build install test check fmt vet clean

build:
	go build -ldflags '$(LDFLAGS)' -o bin/$(BINARY) .

install:
	go build -ldflags '$(LDFLAGS)' -o $(PREFIX)/bin/$(BINARY) .
	@echo "installed $(PREFIX)/bin/$(BINARY) ($(VERSION))"

test:
	go test ./...

fmt:
	gofmt -w .

vet:
	go vet ./...

check: vet test
	@test -z "$$(gofmt -l . )" || (echo "gofmt needed:"; gofmt -l .; exit 1)

clean:
	rm -rf bin
