APP     := usbjieguo
VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -s -w

.PHONY: build build-all clean install

## build: compile for the current platform (output: ./usbjieguo or ./usbjieguo.exe)
build:
	go build -ldflags="$(LDFLAGS)" -o $(APP)$(shell go env GOEXE) .

## build-all: cross-compile for all supported platforms (output: dist/)
build-all: dist
	GOOS=darwin  GOARCH=amd64 go build -ldflags="$(LDFLAGS)" -o dist/$(APP)-darwin-amd64   .
	GOOS=darwin  GOARCH=arm64 go build -ldflags="$(LDFLAGS)" -o dist/$(APP)-darwin-arm64   .
	GOOS=linux   GOARCH=amd64 go build -ldflags="$(LDFLAGS)" -o dist/$(APP)-linux-amd64    .
	GOOS=linux   GOARCH=arm64 go build -ldflags="$(LDFLAGS)" -o dist/$(APP)-linux-arm64    .
	GOOS=windows GOARCH=amd64 go build -ldflags="$(LDFLAGS)" -o dist/$(APP)-windows-amd64.exe .
	@echo "\nBuilt all binaries in dist/:"
	@ls -lh dist/

dist:
	mkdir -p dist

## install: build and install to /usr/local/bin (macOS / Linux)
install: build
	install -m 755 $(APP) /usr/local/bin/$(APP)

## clean: remove compiled binaries and dist/
clean:
	rm -f $(APP) $(APP).exe
	rm -rf dist/
