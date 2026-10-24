GO ?= go
BIN := bin/wpstatic
VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -s -w -X github.com/tekmanic/wpstatic/cmd.Version=$(VERSION)

.PHONY: all build test vet fmt lint clean install

all: build

build:
	$(GO) build -trimpath -ldflags "$(LDFLAGS)" -o $(BIN) .

test:
	$(GO) test -race -count=1 ./...

vet:
	$(GO) vet ./...

fmt:
	gofmt -w .

lint: vet
	$(GO) test -race -count=1 ./...

clean:
	rm -rf bin

install: build
	install -m 0755 $(BIN) /usr/local/bin/wpstatic
