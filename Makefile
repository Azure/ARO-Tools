test:
	go test -cover -race ./...
.PHONY: test

test-compile:
	go test -c -o /dev/null ./...
.PHONY: test-compile

lint:
	go tool golangci-lint run ./...
.PHONY: lint

lint-fix:
	go tool golangci-lint run --fix ./...
.PHONY: lint
