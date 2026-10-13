SHELL := /bin/sh

GO ?= go
GOFMT ?= gofmt
GO_BUILD_FLAGS ?= -buildvcs=false
BINARY ?= bin/equus
PACKAGES := ./...
GO_FILES := $(shell find cmd internal -type f -name '*.go' -print)

# Directory for the global `equus` command. Override with
# `make build INSTALL_DIR=/somewhere` or `make build INSTALL_DIR=` to skip.
INSTALL_DIR ?= $(HOME)/.local/bin
INSTALLED := $(INSTALL_DIR)/equus

.PHONY: all fmt fmt-check vet test build install check clean

all: check

fmt:
	$(GOFMT) -w $(GO_FILES)

fmt-check:
	@unformatted="$$($(GOFMT) -l $(GO_FILES))"; \
	if [ -n "$$unformatted" ]; then \
		echo "Go files need formatting:"; \
		echo "$$unformatted"; \
		exit 1; \
	fi

vet:
	$(GO) vet $(PACKAGES)

test:
	$(GO) test $(PACKAGES)

# build compiles the binary and installs it globally so `equus` is on PATH.
build: install

# install builds first, then copies the binary into INSTALL_DIR. It is a
# no-op when INSTALL_DIR is empty (e.g. `make build INSTALL_DIR=`).
install:
	mkdir -p $(dir $(BINARY))
	$(GO) build $(GO_BUILD_FLAGS) -o $(BINARY) ./cmd/equus
	@if [ -z "$(INSTALL_DIR)" ]; then \
		echo "install: INSTALL_DIR is empty, skipping global install"; \
		exit 0; \
	fi; \
	mkdir -p "$(abspath $(INSTALL_DIR))"; \
	cp -f "$(abspath $(BINARY))" "$(abspath $(INSTALLED))"; \
	echo "installed $(INSTALLED)"

check: fmt-check vet test install

clean:
	rm -rf bin
