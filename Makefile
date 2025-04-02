all: test lint
.PHONY: all

test:
	go test -timeout 1200s  -cover ./...
.PHONY: test

lint:
	go tool golangci-lint run -v ./...
.PHONY: lint

fmt:
	go tool goimports -w -local github.com/Azure/ARO-Tools .
.PHONY: fmt

tidy: fmt
	go mod tidy
.PHONY: tidy
