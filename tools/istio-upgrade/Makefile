SHELL = /bin/bash

# Source files
SOURCES = $(shell find . -name '*.go' -o -name 'go.mod' -o -name 'go.sum')

# Default target
.DEFAULT_GOAL := build

# Build all packages (no main.go — binary entry point is in sdp-pipelines)
build: $(SOURCES)
	go build ./...
.PHONY: build

# Run tests
test:
	go test -v -timeout 300s -cover ./...
.PHONY: test

# Clean build artifacts
clean:
	rm -f istio-upgrade
	rm -f *.exe
	rm -f coverage.out coverage.html
.PHONY: clean

# Help target
help:
	@echo "Available targets:"
	@echo "  build  - Build all packages (default)"
	@echo "  test   - Run unit tests"
	@echo "  clean  - Remove build artifacts"
	@echo "  help   - Show this help message"
.PHONY: help
